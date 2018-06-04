package chain

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/script"
	"github.com/ParallelCoinTeam/duod/lib/utxo"
)

// TrustedTxChecker is meant to speed up verifying transactions that had
// been verified already by the client while being taken to its memory pool
var TrustedTxChecker func(*btc.Tx) bool

// ProcessBlockTransactions -
func (ch *Chain) ProcessBlockTransactions(bl *btc.Block, height, lknown uint32) (changes *utxo.BlockChanges, sigopscost uint32, e error) {
	changes = new(utxo.BlockChanges)
	changes.Height = height
	changes.LastKnownHeight = lknown
	changes.DeledTxs = make(map[[32]byte][]bool, bl.TotalInputs)
	sigopscost, e = ch.commitTxs(bl, changes)
	return
}

// AcceptBlock This function either appends a new block at the end of the existing chain in which case it also applies all the transactions to the unspent database.
// If the block does is not the heighest, it is added to the chain, but maked as an orphan - its transaction will be verified only if the chain would swap to its branch later on.
func (ch *Chain) AcceptBlock(bl *btc.Block) (e error) {
	ch.BlockIndexAccess.Lock()
	cur := ch.AcceptHeader(bl)
	ch.BlockIndexAccess.Unlock()
	return ch.CommitBlock(bl, cur)
}

// AcceptHeader - Make sure to call this function with ch.BlockIndexAccess locked
func (ch *Chain) AcceptHeader(bl *btc.Block) (cur *BlockTreeNode) {
	prevblk, ok := ch.BlockIndex[btc.NewUint256(bl.ParentHash()).BIdx()]
	if !ok {
		panic("This should not happen")
	}

	// create new BlockTreeNode
	cur = new(BlockTreeNode)
	cur.BlockHash = bl.Hash
	cur.Parent = prevblk
	cur.Height = prevblk.Height + 1
	copy(cur.BlockHeader[:], bl.Raw[:80])

	// Add this block to the block index
	prevblk.addChild(cur)
	ch.BlockIndex[cur.BlockHash.BIdx()] = cur

	return
}

// CommitBlock -
func (ch *Chain) CommitBlock(bl *btc.Block, cur *BlockTreeNode) (e error) {
	cur.BlockSize = uint32(len(bl.Raw))
	cur.TxCount = uint32(bl.TxCount)
	if ch.LastBlock() == cur.Parent {
		// The head of out chain - apply the transactions
		var changes *utxo.BlockChanges
		var sigopscost uint32
		changes, sigopscost, e = ch.ProcessBlockTransactions(bl, cur.Height, bl.LastKnownHeight)
		if e != nil {
			// ProcessBlockTransactions failed, so trash the block.
			//println("ProcessBlockTransactionsA", cur.BlockHash.String(), cur.Height, e.Error())
			ch.BlockIndexAccess.Lock()
			cur.Parent.delChild(cur)
			delete(ch.BlockIndex, cur.BlockHash.BIdx())
			ch.BlockIndexAccess.Unlock()
		} else {
			cur.SigopsCost = sigopscost
			// ProcessBlockTransactions succeeded, so save the block as "trusted".
			bl.Trusted = true
			ch.Blocks.BlockAdd(cur.Height, bl)
			// Apply the block's trabnsactions to the unspent database:
			ch.Unspent.CommitBlockTxs(changes, bl.Hash.Hash[:])
			ch.SetLast(cur) // Advance the head
			if ch.CB.BlockMinedCB != nil {
				ch.CB.BlockMinedCB(bl)
			}
		}
	} else {
		// The block's parent is not the current head of the chain...

		// Save the block, though do not mark it as "trusted" just yet
		ch.Blocks.BlockAdd(cur.Height, bl)

		// If it has more POW than the current head, move the head to it
		if cur.MorePOW(ch.LastBlock()) {
			ch.MoveToBlock(cur)
			if ch.LastBlock() != cur {
				e = errors.New("CommitBlock: MoveToBlock failed")
			}
		} else {
			println("Orphaned block", bl.Hash.String(), cur.Height)
		}
	}

	return
}

// This isusually the most time consuming process when applying a new block
func (ch *Chain) commitTxs(bl *btc.Block, changes *utxo.BlockChanges) (sigopscost uint32, e error) {
	sumblockin := btc.GetBlockReward(changes.Height)
	var txoutsum, txinsum, sumblockout uint64

	if changes.Height+ch.Unspent.UnwindBufLen >= changes.LastKnownHeight {
		changes.UndoData = make(map[[32]byte]*utxo.Rec)
	}

	blUnsp := make(map[[32]byte][]*btc.TxOut, len(bl.Txs))

	var wg sync.WaitGroup
	var verErrCount uint32

	for i := range bl.Txs {
		txoutsum, txinsum = 0, 0

		sigopscost += uint32(btc.WitnessScaleFactor * bl.Txs[i].GetLegacySigOpCount())

		// Check each tx for a valid input, except from the first one
		if i > 0 {
			txTrusted := bl.Trusted
			if !txTrusted && TrustedTxChecker != nil && TrustedTxChecker(bl.Txs[i]) {
				txTrusted = true
			}

			for j := 0; j < len(bl.Txs[i].TxIn); j++ {
				inp := &bl.Txs[i].TxIn[j].Input
				spentMap, wasSpent := changes.DeledTxs[inp.Hash]
				if wasSpent {
					if int(inp.Vout) >= len(spentMap) {
						println("txin", inp.String(), "did not have vout", inp.Vout)
						e = errors.New("Tx VOut too big")
						return
					}

					if spentMap[inp.Vout] {
						println("txin", inp.String(), "already spent in this block")
						e = errors.New("Double spend inside the block")
						return
					}
				}
				tout := ch.Unspent.UnspentGet(inp)
				if tout == nil {
					t, ok := blUnsp[inp.Hash]
					if !ok {
						e = errors.New("Unknown input TxID: " + btc.NewUint256(inp.Hash[:]).String())
						return
					}

					if inp.Vout >= uint32(len(t)) {
						println("Vout too big", len(t), inp.String())
						e = errors.New("Vout too big")
						return
					}

					if t[inp.Vout] == nil {
						println("Vout already spent", inp.String())
						e = errors.New("Vout already spent")
						return
					}

					if t[inp.Vout].WasCoinbase {
						e = errors.New("Cannot spend block's own coinbase in TxID: " + btc.NewUint256(inp.Hash[:]).String())
						return
					}

					tout = t[inp.Vout]
					t[inp.Vout] = nil // and now mark it as spent:
				} else {
					if tout.WasCoinbase && changes.Height-tout.BlockHeight < CoinbaseMaturity {
						e = errors.New("Trying to spend prematured coinbase: " + btc.NewUint256(inp.Hash[:]).String())
						return
					}
					// it is confirmed already so delete it later
					if !wasSpent {
						spentMap = make([]bool, tout.VoutCount)
						changes.DeledTxs[inp.Hash] = spentMap
					}
					spentMap[inp.Vout] = true

					if changes.UndoData != nil {
						var urec *utxo.Rec
						urec = changes.UndoData[inp.Hash]
						if urec == nil {
							urec = new(utxo.Rec)
							urec.TxID = inp.Hash
							urec.Coinbase = tout.WasCoinbase
							urec.InBlock = tout.BlockHeight
							urec.Outs = make([]*utxo.TxOut, tout.VoutCount)
							changes.UndoData[inp.Hash] = urec
						}
						tmp := new(utxo.TxOut)
						tmp.Value = tout.Value
						tmp.PKScr = make([]byte, len(tout.PkScript))
						copy(tmp.PKScr, tout.PkScript)
						urec.Outs[inp.Vout] = tmp
					}
				}

				if !txTrusted { // run VerifyTxScript() in a parallel task
					wg.Add(1)
					go func(prv []byte, amount uint64, i int, tx *btc.Tx) {
						if !script.VerifyTxScript(prv, amount, i, tx, bl.VerifyFlags) {
							atomic.AddUint32(&verErrCount, 1)
						}
						wg.Done()
					}(tout.PkScript, tout.Value, j, bl.Txs[i])
				}

				if btc.IsP2SH(tout.PkScript) {
					sigopscost += uint32(btc.WitnessScaleFactor * btc.GetP2SHSigOpCount(bl.Txs[i].TxIn[j].ScriptSig))
				}

				sigopscost += uint32(bl.Txs[i].CountWitnessSigOps(j, tout.PkScript))

				txinsum += tout.Value
			}

			if !txTrusted {
				wg.Wait()
				if verErrCount > 0 {
					println("VerifyScript failed", verErrCount, "time (s)")
					e = errors.New(fmt.Sprint("VerifyScripts failed ", verErrCount, "time (s)"))
					return
				}
			}
		} else {
			// For coinbase tx we need to check (like satoshi) whether the script size is between 2 and 100 bytes
			// (Previously we made sure in CheckBlock() that this was a coinbase type tx)
			if len(bl.Txs[0].TxIn[0].ScriptSig) < 2 || len(bl.Txs[0].TxIn[0].ScriptSig) > 100 {
				e = errors.New(fmt.Sprint("Coinbase script has a wrong length ", len(bl.Txs[0].TxIn[0].ScriptSig)))
				return
			}
		}
		sumblockin += txinsum

		for j := range bl.Txs[i].TxOut {
			txoutsum += bl.Txs[i].TxOut[j].Value
		}
		sumblockout += txoutsum

		if e != nil {
			return // If any input fails, do not continue
		}
		if i > 0 {
			bl.Txs[i].Fee = txinsum - txoutsum
			if txoutsum > txinsum {
				e = errors.New(fmt.Sprintf("More spent (%.8f) than at the input (%.8f) in TX %s",
					float64(txoutsum)/1e8, float64(txinsum)/1e8, bl.Txs[i].Hash.String()))
				return
			}
		}

		// Add each tx outs from the currently executed TX to the temporary pool
		outs := make([]*btc.TxOut, len(bl.Txs[i].TxOut))
		copy(outs, bl.Txs[i].TxOut)
		blUnsp[bl.Txs[i].Hash.Hash] = outs
	}

	if sumblockin < sumblockout {
		e = errors.New(fmt.Sprintf("Out:%d > In:%d", sumblockout, sumblockin))
		return
	}

	if sigopscost > ch.MaxBlockSigopsCost(bl.Height) {
		e = errors.New("commitTxs(): too many sigops - RPC_Result:bad-blk-sigops")
		return
	}

	var rec *utxo.Rec
	changes.AddList = make([]*utxo.Rec, 0, len(blUnsp))
	for k, v := range blUnsp {
		for i := range v {
			if v[i] != nil {
				if rec == nil {
					rec = new(utxo.Rec)
					rec.TxID = k
					rec.Coinbase = v[i].WasCoinbase
					rec.InBlock = changes.Height
					rec.Outs = make([]*utxo.TxOut, len(v))
				}
				rec.Outs[i] = &utxo.TxOut{Value: v[i].Value, PKScr: v[i].PkScript}
			}
		}
		if rec != nil {
			changes.AddList = append(changes.AddList, rec)
			rec = nil
		}
	}

	return
}

// CheckTransactions - Check transactions for consistency and finality. Return nil if OK, otherwise a descripive error
func CheckTransactions(txs []*btc.Tx, height, btime uint32) (res error) {
	var wg sync.WaitGroup

	resChan := make(chan error, 1)

	for i := 0; len(resChan) == 0 && i < len(txs); i++ {
		wg.Add(1)

		go func(tx *btc.Tx) {
			defer wg.Done() // call wg.Done() before returning from this goroutine

			if len(resChan) > 0 {
				return // abort checking if a parallel error has already been reported
			}

			er := tx.CheckTransaction()

			if len(resChan) > 0 {
				return // abort checking if a parallel error has already been reported
			}

			if er == nil && !tx.IsFinal(height, btime) {
				er = errors.New("CheckTransactions() : not-final transaction - RPC_Result:bad-txns-nonfinal")
			}

			if er != nil {
				select { // this is a non-blocking write to channel
				case resChan <- er:
				default:
				}
			}
		}(txs[i])
	}

	wg.Wait() // wait for all the goroutines to complete

	if len(resChan) > 0 {
		res = <-resChan
	}

	return
}
