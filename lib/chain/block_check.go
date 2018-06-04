package chain

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/script"
)

// PreCheckBlock -
// Make sure to call this function with ch.BlockIndexAccess locked
func (ch *Chain) PreCheckBlock(bl *btc.Block) (dos bool, maybelater bool, err error) {
	// Size limits
	if len(bl.Raw) < 81 {
		err = errors.New("CheckBlock() : size limits failed - RPC_Result:bad-blk-length")
		dos = true
		return
	}

	ver := bl.Version()
	if ver == 0 {
		err = errors.New("CheckBlock() : Block version 0 not allowed - RPC_Result:bad-version")
		dos = true
		return
	}

	// Check proof-of-work
	if !btc.CheckProofOfWork(bl.Hash, bl.Bits()) {
		err = errors.New("CheckBlock() : proof of work failed - RPC_Result:high-hash")
		dos = true
		return
	}

	// Check timestamp (must not be higher than now +2 hours)
	if int64(bl.BlockTime()) > time.Now().Unix()+2*60*60 {
		err = errors.New("CheckBlock() : block timestamp too far in the future - RPC_Result:time-too-new")
		dos = true
		return
	}

	if prv, pres := ch.BlockIndex[bl.Hash.BIdx()]; pres {
		if prv.Parent == nil {
			// This is genesis block
			err = errors.New("Genesis")
			return
		}
		err = errors.New("CheckBlock: " + bl.Hash.String() + " already in - RPC_Result:duplicate")
		return
	}

	prevblk, ok := ch.BlockIndex[btc.NewUint256(bl.ParentHash()).BIdx()]
	if !ok {
		err = errors.New("CheckBlock: " + bl.Hash.String() + " parent not found - RPC_Result:bad-prevblk")
		maybelater = true
		return
	}

	bl.Height = prevblk.Height + 1

	// Reject the block if it reaches into the chain deeper than our unwind buffer
	lstNow := ch.LastBlock()
	if prevblk != lstNow && int(lstNow.Height)-int(bl.Height) >= MovingCheckopintDepth {
		err = errors.New(fmt.Sprint("CheckBlock: btc.Block ", bl.Hash.String(),
			" hooks too deep into the chain: ", bl.Height, "/", lstNow.Height, " ",
			btc.NewUint256(bl.ParentHash()).String(), " - RPC_Result:bad-prevblk"))
		return
	}

	// Check proof of work
	gnwr := ch.GetNextWorkRequired(prevblk, bl.BlockTime())
	if bl.Bits() != gnwr {
		err = errors.New("CheckBlock: incorrect proof of work - RPC_Result:bad-diffbits")
		dos = true
		return
	}

	// Check timestamp against prev
	bl.MedianPastTime = prevblk.GetMedianTimePast()
	if bl.BlockTime() <= bl.MedianPastTime {
		err = errors.New("CheckBlock: block's timestamp is too early - RPC_Result:time-too-old")
		dos = true
		return
	}

	if ver < 2 && bl.Height >= ch.Consensus.BIP34Height ||
		ver < 3 && bl.Height >= ch.Consensus.BIP66Height ||
		ver < 4 && bl.Height >= ch.Consensus.BIP65Height {
		// bad block version
		erstr := fmt.Sprintf("0x%08x", ver)
		err = errors.New("CheckBlock() : Rejected Version=" + erstr + " block - RPC_Result:bad-version(" + erstr + ")")
		dos = true
		return
	}

	if ch.Consensus.BIP91Height != 0 && ch.Consensus.EnforceSegwit != 0 {
		if bl.Height >= ch.Consensus.BIP91Height && bl.Height < ch.Consensus.EnforceSegwit-2016 {
			if (ver&0xE0000000) != 0x20000000 || (ver&2) == 0 {
				err = errors.New("CheckBlock() : relayed block must signal for segwit - RPC_Result:bad-no-segwit")
			}
		}
	}

	return
}

// ApplyBlockFlags -
func (ch *Chain) ApplyBlockFlags(bl *btc.Block) {
	if bl.BlockTime() >= BIP16SwitchTime {
		bl.VerifyFlags = script.VerP2sh
	} else {
		bl.VerifyFlags = 0
	}

	if bl.Height >= ch.Consensus.BIP66Height {
		bl.VerifyFlags |= script.VerDerSig
	}

	if bl.Height >= ch.Consensus.BIP65Height {
		bl.VerifyFlags |= script.VerCLTV
	}

	if ch.Consensus.EnforceCSV != 0 && bl.Height >= ch.Consensus.EnforceCSV {
		bl.VerifyFlags |= script.VerCSV
	}

	if ch.Consensus.EnforceSegwit != 0 && bl.Height >= ch.Consensus.EnforceSegwit {
		bl.VerifyFlags |= script.VerWitness | script.VerNullDummy
	}

}

// PostCheckBlock -
func (ch *Chain) PostCheckBlock(bl *btc.Block) (err error) {
	// Size limits
	if len(bl.Raw) < 81 {
		err = errors.New("CheckBlock() : size limits failed low - RPC_Result:bad-blk-length")
		return
	}

	if bl.Txs == nil {
		err = bl.BuildTxList()
		if err != nil {
			return
		}
		if bl.BlockWeight > ch.MaxBlockWeight(bl.Height) {
			err = errors.New("CheckBlock() : weight limits failed - RPC_Result:bad-blk-weight")
			return
		}
		//fmt.Println("New block", bl.Height, " Weight:", bl.BlockWeight, " Raw:", len(bl.Raw))
	}

	if !bl.Trusted {
		// We need to be satoshi compatible
		if len(bl.Txs) == 0 || !bl.Txs[0].IsCoinBase() {
			err = errors.New("CheckBlock() : first tx is not coinbase: " + bl.Hash.String() + " - RPC_Result:bad-cb-missing")
			return
		}

		// Enforce rule that the coinbase starts with serialized block height
		if bl.Height >= ch.Consensus.BIP34Height {
			var exp [6]byte
			var expLen int
			binary.LittleEndian.PutUint32(exp[1:5], bl.Height)
			for expLen = 5; expLen > 1; expLen-- {
				if exp[expLen] != 0 || exp[expLen-1] >= 0x80 {
					break
				}
			}
			exp[0] = byte(expLen)
			expLen++

			if !bytes.HasPrefix(bl.Txs[0].TxIn[0].ScriptSig, exp[:expLen]) {
				err = errors.New("CheckBlock() : Unexpected block number in coinbase: " + bl.Hash.String() + " - RPC_Result:bad-cb-height")
				return
			}
		}

		// And again...
		for i := 1; i < len(bl.Txs); i++ {
			if bl.Txs[i].IsCoinBase() {
				err = errors.New("CheckBlock() : more than one coinbase: " + bl.Hash.String() + " - RPC_Result:bad-cb-multiple")
				return
			}
		}
	}

	// Check Merkle Root, even for trusted blocks - that's important, as they may come from untrasted peers
	merkle, mutated := bl.GetMerkle()
	if mutated {
		err = errors.New("CheckBlock(): duplicate transaction - RPC_Result:bad-txns-duplicate")
		return
	}

	if !bytes.Equal(merkle, bl.MerkleRoot()) {
		err = errors.New("CheckBlock() : Merkle Root mismatch - RPC_Result:bad-txnmrklroot")
		return
	}

	ch.ApplyBlockFlags(bl)

	if !bl.Trusted {
		var blockTime uint32
		var hadWitness bool

		if (bl.VerifyFlags & script.VerCSV) != 0 {
			blockTime = bl.MedianPastTime
		} else {
			blockTime = bl.BlockTime()
		}

		// Verify merkle root of witness data
		if (bl.VerifyFlags & script.VerWitness) != 0 {
			var i int
			for i = len(bl.Txs[0].TxOut) - 1; i >= 0; i-- {
				o := bl.Txs[0].TxOut[i]
				if len(o.PkScript) >= 38 && bytes.Equal(o.PkScript[:6], []byte{0x6a, 0x24, 0xaa, 0x21, 0xa9, 0xed}) {
					if len(bl.Txs[0].SegWit) != 1 || len(bl.Txs[0].SegWit[0]) != 1 || len(bl.Txs[0].SegWit[0][0]) != 32 {
						err = errors.New("CheckBlock() : invalid witness nonce size - RPC_Result:bad-witness-nonce-size")
						println(err.Error())
						println(bl.Hash.String(), len(bl.Txs[0].SegWit))
						return
					}

					// The malleation check is ignored; as the transaction tree itself
					// already does not permit it, it is impossible to trigger in the
					// witness tree.
					merkle, _ := btc.GetWitnessMerkle(bl.Txs)
					withNonce := btc.Sha2Sum(append(merkle, bl.Txs[0].SegWit[0][0]...))

					if !bytes.Equal(withNonce[:], o.PkScript[6:38]) {
						err = errors.New("CheckBlock(): Witness Merkle mismatch - RPC_Result:bad-witness-merkle-match")
						return
					}

					hadWitness = true
					break
				}
			}
		}

		if !hadWitness {
			for _, t := range bl.Txs {
				if t.SegWit != nil {
					err = errors.New("CheckBlock(): unexpected witness data found - RPC_Result:unexpected-witness")
					return
				}
			}
		}

		// Check transactions - this is the most time consuming task
		err = CheckTransactions(bl.Txs, bl.Height, blockTime)
	}
	return
}

// CheckBlock -
func (ch *Chain) CheckBlock(bl *btc.Block) (dos bool, maybelater bool, err error) {
	dos, maybelater, err = ch.PreCheckBlock(bl)
	if err == nil {
		err = ch.PostCheckBlock(bl)
		if err != nil { // all post-check errors are DoS kind
			dos = true
		}
	}
	return
}
