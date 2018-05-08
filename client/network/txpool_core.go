// Package network -
package network

import (
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/calibrae-project/spawn/client/common"
	"github.com/calibrae-project/spawn/lib/btc"
	"github.com/calibrae-project/spawn/lib/chain"
	"github.com/calibrae-project/spawn/lib/script"
)

const (
	// TxRejectedDisabled -
	TxRejectedDisabled = 1
	// TxRejectedTooBig -
	TxRejectedTooBig = 101
	// TxRejectedFormat -
	TxRejectedFormat = 102
	// TxRejectedLenMismatch -
	TxRejectedLenMismatch = 103
	// TxRejectedEmptyInput -
	TxRejectedEmptyInput = 104
	// TxRejectedOverspend -
	TxRejectedOverspend = 154
	// TxRejectedBadInput -
	TxRejectedBadInput = 157
	// Anything from the list below might eventually get mined

	// TxRejectedNoTxOU -
	TxRejectedNoTxOU = 202
	// TxRejectedLowFee -
	TxRejectedLowFee = 205
	// TxRejectedNotMined -
	TxRejectedNotMined = 208
	// TxRejectedCoinbaseImmature -
	TxRejectedCoinbaseImmature = 209
	// TxRejectedRBFLowFee -
	TxRejectedRBFLowFee = 210
	// TxRejectedRBFFinal -
	TxRejectedRBFFinal = 211
	// TxRejectedRBF100 -
	TxRejectedRBF100 = 212
	// TxRejectedReplaced -
	TxRejectedReplaced = 213
)

var (
	// TxMutex -
	TxMutex sync.Mutex

	// The actual memory pool:

	// TransactionsToSend -
	TransactionsToSend = make(map[BIDX]*OneTxToSend)
	// TransactionsToSendSize -
	TransactionsToSendSize uint64
	// TransactionsToSendWeight -
	TransactionsToSendWeight uint64

	// All the outputs that are currently spent in TransactionsToSend:

	// SpentOutputs -
	SpentOutputs = make(map[uint64]BIDX)

	// Transactions that we downloaded, but rejected:

	// TransactionsRejected -
	TransactionsRejected = make(map[BIDX]*OneTxRejected)
	// TransactionsRejectedSize -
	TransactionsRejectedSize uint64 // only include those that have *Tx pointer set

	// Transactions that are received from network (via "tx"), but not yet processed:

	// TransactionsPending -
	TransactionsPending = make(map[BIDX]bool)

	// Transactions that are waiting for inputs:

	// WaitingForInputs -
	WaitingForInputs = make(map[BIDX]*OneWaitingList)
	// WaitingForInputsSize -
	WaitingForInputsSize uint64
)

// OneTxToSend -
type OneTxToSend struct {
	Invsentcnt, SentCnt uint32
	Firstseen, Lastsent time.Time
	Local               bool
	Spent               []uint64 // Which records in SpentOutputs this TX added
	Volume, Fee         uint64
	*btc.Tx
	Blocked     byte   // if non-zero, it gives you the reason why this tx nas not been routed
	MemInputs   []bool // transaction is spending inputs from other unconfirmed tx(s)
	MemInputCnt int
	SigopsCost  uint64
	Final       bool // if true RFB will not work on it
	VerifyTime  time.Duration
}

// OneTxRejected -
type OneTxRejected struct {
	ID *btc.Uint256
	time.Time
	Size     uint32
	Reason   byte
	Waiting4 *btc.Uint256
	*btc.Tx
}

// OneWaitingList -
type OneWaitingList struct {
	TxID  *btc.Uint256
	TxLen uint32
	Ids   map[BIDX]time.Time // List of pending tx ids
}

// ReasonToString -
func ReasonToString(reason byte) string {
	switch reason {
	case 0:
		return ""
	case TxRejectedDisabled:
		return "RELAY_OFF"
	case TxRejectedTooBig:
		return "TOO_BIG"
	case TxRejectedFormat:
		return "FORMAT"
	case TxRejectedLenMismatch:
		return "LEN_MISMATCH"
	case TxRejectedEmptyInput:
		return "EMPTY_INPUT"
	case TxRejectedOverspend:
		return "OVERSPEND"
	case TxRejectedBadInput:
		return "BAD_INPUT"
	case TxRejectedNoTxOU:
		return "NO_TXOU"
	case TxRejectedLowFee:
		return "LOW_FEE"
	case TxRejectedNotMined:
		return "NOT_MINED"
	case TxRejectedCoinbaseImmature:
		return "CB_INMATURE"
	case TxRejectedRBFLowFee:
		return "RBF_LOWFEE"
	case TxRejectedRBFFinal:
		return "RBF_FINAL"
	case TxRejectedRBF100:
		return "RBF_100"
	case TxRejectedReplaced:
		return "REPLACED"
	}
	return fmt.Sprint("UNKNOWN_", reason)
}

// NeedThisTx -
func NeedThisTx(id *btc.Uint256, cb func()) (res bool) {
	return NeedThisTxExt(id, cb) == 0
}

// NeedThisTxExt - Return false if we do not want to receive a data for this tx
func NeedThisTxExt(id *btc.Uint256, cb func()) (whyNot int) {
	TxMutex.Lock()
	if _, present := TransactionsToSend[id.BIdx()]; present {
		whyNot = 1
	} else if _, present := TransactionsRejected[id.BIdx()]; present {
		whyNot = 2
	} else if _, present := TransactionsPending[id.BIdx()]; present {
		whyNot = 3
	} else if common.BlockChain.Unspent.TxPresent(id) {
		whyNot = 4
		// This assumes that tx's out #0 has not been spent yet, which may not always be the case, but well...
		common.CountSafe("TxAlreadyMined")
	} else {
		// whyNot = 0
		if cb != nil {
			cb()
		}
	}
	TxMutex.Unlock()
	return
}

// TxInvNotify - Handle tx-inv notifications
func (c *OneConnection) TxInvNotify(hash []byte) {
	if NeedThisTx(btc.NewUint256(hash), nil) {
		var b [1 + 4 + 32]byte
		b[0] = 1 // One inv
		if (c.Node.Services & ServiceSegwit) != 0 {
			binary.LittleEndian.PutUint32(b[1:5], MsgWitnessTx) // SegWit Tx
			//println(c.ConnID, "getdata", btc.NewUint256(hash).String())
		} else {
			b[1] = MsgTx // Tx
		}
		copy(b[5:37], hash)
		c.SendRawMsg("getdata", b[:])
	}
}

// RejectTx - Adds a transaction to the rejected list or not, it it has been mined already
// Make sure to call it with locked TxMutex.
// Returns the OneTxRejected or nil if it has not been added.
func RejectTx(tx *btc.Tx, why byte) *OneTxRejected {
	rec := new(OneTxRejected)
	rec.Time = time.Now()
	rec.Size = uint32(len(tx.Raw))
	rec.Reason = why

	// TODO: only store tx for selected reasons
	if why >= 200 {
		rec.Tx = tx
		rec.ID = &tx.Hash
		TransactionsRejectedSize += uint64(rec.Size)
	} else {
		rec.ID = new(btc.Uint256)
		rec.ID.Hash = tx.Hash.Hash
	}

	bidx := tx.Hash.BIdx()
	TransactionsRejected[bidx] = rec

	LimitRejectedSize()

	// try to re-fetch the record from the map, in case it has been removed by LimitRejectedSize()
	return TransactionsRejected[bidx]
}

// ParseTxNet - Handle incoming "tx" msg
func (c *OneConnection) ParseTxNet(pl []byte) {
	tx, le := btc.NewTx(pl)
	if tx == nil {
		c.DoS("TxRejectedBroken")
		return
	}
	if le != len(pl) {
		c.DoS("TxRejectedLenMismatch")
		return
	}
	if len(tx.TxIn) < 1 {
		c.Misbehave("TxRejectedNoInputs", 100)
		return
	}

	tx.SetHash(pl)

	if tx.Weight() > 4*int(common.GetUint32(&common.CFG.TXPool.MaxTxSize)) {
		TxMutex.Lock()
		RejectTx(tx, TxRejectedTooBig)
		TxMutex.Unlock()
		common.CountSafe("TxRejectedBig")
		return
	}

	NeedThisTx(&tx.Hash, func() {
		// This body is called with a locked TxMutex
		tx.Raw = pl
		select {
		case NetTxs <- &TxRcvd{conn: c, Tx: tx, trusted: c.X.Authorized}:
			TransactionsPending[tx.Hash.BIdx()] = true
		default:
			common.CountSafe("TxRejectedFullQ")
			//println("NetTxsFULL")
		}
	})
}

// HandleNetTx - Must be called from the chain's thread
func HandleNetTx(ntx *TxRcvd, retry bool) (accepted bool) {
	common.CountSafe("HandleNetTx")

	tx := ntx.Tx
	startTime := time.Now()
	var final bool // set to true if any of the inpits has a final sequence

	var totinp, totout uint64
	var frommem []bool
	var frommemcnt int

	TxMutex.Lock()

	if !retry {
		if _, present := TransactionsPending[tx.Hash.BIdx()]; !present {
			// It had to be mined in the meantime, so just drop it now
			TxMutex.Unlock()
			common.CountSafe("TxNotPending")
			return
		}
		delete(TransactionsPending, ntx.Hash.BIdx())
	} else {
		// In case case of retry, it is on the rejected list,
		// ... so remove it now to free any tied WaitingForInputs
		deleteRejected(tx.Hash.BIdx())
	}

	pos := make([]*btc.TxOut, len(tx.TxIn))
	spent := make([]uint64, len(tx.TxIn))

	var rbfTxList map[*OneTxToSend]bool

	// Check if all the inputs exist in the chain
	for i := range tx.TxIn {
		if !final && tx.TxIn[i].Sequence >= 0xfffffffe {
			final = true
		}

		spent[i] = tx.TxIn[i].Input.UIdx()

		if so, ok := SpentOutputs[spent[i]]; ok {
			// Can only be accepted as RBF...

			if rbfTxList == nil {
				rbfTxList = make(map[*OneTxToSend]bool)
			}

			ctx := TransactionsToSend[so]

			if !ntx.trusted && ctx.Final {
				RejectTx(ntx.Tx, TxRejectedRBFFinal)
				TxMutex.Unlock()
				common.CountSafe("TxRejectedRBFFinal")
				return
			}

			rbfTxList[ctx] = true
			if !ntx.trusted && len(rbfTxList) > 100 {
				RejectTx(ntx.Tx, TxRejectedRBF100)
				TxMutex.Unlock()
				common.CountSafe("TxRejectedRBF100+")
				return
			}

			chlds := ctx.GetAllChildren()
			for _, ctx = range chlds {
				if !ntx.trusted && ctx.Final {
					RejectTx(ntx.Tx, TxRejectedRBFFinal)
					TxMutex.Unlock()
					common.CountSafe("TxRejectedRBF_Final")
					return
				}

				rbfTxList[ctx] = true

				if !ntx.trusted && len(rbfTxList) > 100 {
					RejectTx(ntx.Tx, TxRejectedRBF100)
					TxMutex.Unlock()
					common.CountSafe("TxRejectedRBF100+")
					return
				}
			}
		}

		if txinmem, ok := TransactionsToSend[btc.BIdx(tx.TxIn[i].Input.Hash[:])]; ok {
			if int(tx.TxIn[i].Input.Vout) >= len(txinmem.TxOut) {
				RejectTx(ntx.Tx, TxRejectedBadInput)
				TxMutex.Unlock()
				common.CountSafe("TxRejectedBadInput")
				return
			}

			if !ntx.trusted && !common.CFG.TXPool.AllowMemInputs {
				RejectTx(ntx.Tx, TxRejectedNotMined)
				TxMutex.Unlock()
				common.CountSafe("TxRejectedMemInput1")
				return
			}

			pos[i] = txinmem.TxOut[tx.TxIn[i].Input.Vout]
			common.CountSafe("TxInputInMemory")
			if frommem == nil {
				frommem = make([]bool, len(tx.TxIn))
			}
			frommem[i] = true
			frommemcnt++
		} else {
			pos[i] = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if pos[i] == nil {
				var newone bool

				if !common.CFG.TXPool.AllowMemInputs {
					RejectTx(ntx.Tx, TxRejectedNotMined)
					TxMutex.Unlock()
					common.CountSafe("TxRejectedMemInput2")
					return
				}

				if rej, ok := TransactionsRejected[btc.BIdx(tx.TxIn[i].Input.Hash[:])]; ok {
					if rej.Reason != TxRejectedNoTxOU || rej.Waiting4 == nil {
						RejectTx(ntx.Tx, TxRejectedNoTxOU)
						TxMutex.Unlock()
						common.CountSafe("TxRejectedParentRej")
						return
					}
					common.CountSafe("TxWait4ParentsParent")
				}

				// In this case, let's "save" it for later...
				missingid := btc.NewUint256(tx.TxIn[i].Input.Hash[:])
				nrtx := RejectTx(ntx.Tx, TxRejectedNoTxOU)

				if nrtx != nil && nrtx.Tx != nil {
					nrtx.Waiting4 = missingid
					//nrtx.Tx = ntx.Tx

					// Add to waiting list:
					var rec *OneWaitingList
					if rec, _ = WaitingForInputs[missingid.BIdx()]; rec == nil {
						rec = new(OneWaitingList)
						rec.TxID = missingid
						rec.TxLen = uint32(len(ntx.Raw))
						rec.Ids = make(map[BIDX]time.Time)
						newone = true
						WaitingForInputsSize += uint64(rec.TxLen)
					}
					rec.Ids[tx.Hash.BIdx()] = time.Now()
					WaitingForInputs[missingid.BIdx()] = rec
				}

				TxMutex.Unlock()
				if newone {
					common.CountSafe("TxRejectedNoInpNew")
				} else {
					common.CountSafe("TxRejectedNoInpOld")
				}
				return
			}
			if pos[i].WasCoinbase {
				if common.Last.BlockHeight()+1-pos[i].BlockHeight < chain.COINBASE_MATURITY {
					RejectTx(ntx.Tx, TxRejectedCoinbaseImmature)
					TxMutex.Unlock()
					common.CountSafe("TxRejectedCBInmature")
					fmt.Println(tx.Hash.String(), "trying to spend inmature coinbase block", pos[i].BlockHeight, "at", common.Last.BlockHeight())
					return
				}
			}
		}
		totinp += pos[i].Value
	}

	// Check if total output value does not exceed total input
	for i := range tx.TxOut {
		totout += tx.TxOut[i].Value
	}

	if totout > totinp {
		RejectTx(ntx.Tx, TxRejectedOverspend)
		TxMutex.Unlock()
		if ntx.conn != nil {
			ntx.conn.DoS("TxOverspend")
		}
		return
	}

	// Check for a proper fee
	fee := totinp - totout
	if !ntx.local && fee < (uint64(tx.VSize())*common.MinFeePerKB()/1000) { // do not check minimum fee for locally loaded txs
		RejectTx(ntx.Tx, TxRejectedLowFee)
		TxMutex.Unlock()
		common.CountSafe("TxRejectedLowFee")
		return
	}

	if rbfTxList != nil {
		var totweight int
		var totfees uint64

		for ctx := range rbfTxList {
			totweight += ctx.Weight()
			totfees += ctx.Fee
		}

		if !ntx.local && totfees*uint64(tx.Weight()) >= fee*uint64(totweight) {
			RejectTx(ntx.Tx, TxRejectedRBFLowFee)
			TxMutex.Unlock()
			common.CountSafe("TxRejectedRBFLowFee")
			return
		}
	}

	sigops := btc.WitnessScaleFactor * tx.GetLegacySigOpCount()

	if !ntx.trusted { // Verify scripts
		var wg sync.WaitGroup
		var verErrCount uint32

		prevDebugError := script.DebugError
		script.DebugError = false // keep quiet for incorrect txs
		for i := range tx.TxIn {
			wg.Add(1)
			go func(prv []byte, amount uint64, i int, tx *btc.Tx) {
				if !script.VerifyTxScript(prv, amount, i, tx, script.STANDARD_VERIFY_FLAGS) {
					atomic.AddUint32(&verErrCount, 1)
				}
				wg.Done()
			}(pos[i].Pk_script, pos[i].Value, i, tx)
		}

		wg.Wait()
		script.DebugError = prevDebugError

		if verErrCount > 0 {
			// not moving it to rejected, but baning the peer
			TxMutex.Unlock()
			if ntx.conn != nil {
				ntx.conn.DoS("TxScriptFail")
			}
			if len(rbfTxList) > 0 {
				fmt.Println("RBF try", verErrCount, "script(s) failed!")
				fmt.Print("> ")
			}
			return
		}
	}

	for i := range tx.TxIn {
		if btc.IsP2SH(pos[i].Pk_script) {
			sigops += btc.WitnessScaleFactor * btc.GetP2SHSigOpCount(tx.TxIn[i].ScriptSig)
		}
		sigops += uint(tx.CountWitnessSigOps(i, pos[i].Pk_script))
	}

	if rbfTxList != nil {
		for ctx := range rbfTxList {
			// we dont remove with children because we have all of them on the list
			ctx.Delete(false, TxRejectedReplaced)
			common.CountSafe("TxRemovedByRBF")
		}
	}

	rec := &OneTxToSend{Spent: spent, Volume: totinp, Local: ntx.local,
		Fee: fee, Firstseen: time.Now(), Tx: tx, MemInputs: frommem, MemInputCnt: frommemcnt,
		SigopsCost: uint64(sigops), Final: final, VerifyTime: time.Now().Sub(startTime)}

	TransactionsToSend[tx.Hash.BIdx()] = rec

	if maxpoolsize := common.MaxMempoolSize(); maxpoolsize != 0 {
		newsize := TransactionsToSendSize + uint64(len(rec.Raw))
		if TransactionsToSendSize < maxpoolsize && newsize >= maxpoolsize {
			expireTxsNow = true
		}
		TransactionsToSendSize = newsize
	} else {
		TransactionsToSendSize += uint64(len(rec.Raw))
	}
	TransactionsToSendWeight += uint64(rec.Tx.Weight())

	for i := range spent {
		SpentOutputs[spent[i]] = tx.Hash.BIdx()
	}

	wtg := WaitingForInputs[tx.Hash.BIdx()]
	if wtg != nil {
		defer RetryWaitingForInput(wtg) // Redo waiting txs when leaving this function
	}

	TxMutex.Unlock()
	common.CountSafe("TxAccepted")

	if frommem != nil && !common.GetBool(&common.CFG.TXRoute.MemInputs) {
		// By default Spawn does not route txs that spend unconfirmed inputs
		rec.Blocked = TxRejectedNotMined
		common.CountSafe("TxRouteNotMined")
	} else if !ntx.trusted && rec.isRoutable() {
		// do not automatically route loacally loaded txs
		rec.Invsentcnt += NetRouteInvExt(1, &tx.Hash, ntx.conn, 1000*fee/uint64(len(ntx.Raw)))
		common.CountSafe("TxRouteOK")
	}

	if ntx.conn != nil {
		ntx.conn.Mutex.Lock()
		ntx.conn.txsCur++
		ntx.conn.X.TxsReceived++
		ntx.conn.Mutex.Unlock()
	}

	accepted = true
	return
}

func (tx *OneTxToSend) isRoutable() bool {
	if !common.CFG.TXRoute.Enabled {
		common.CountSafe("TxRouteDisabled")
		tx.Blocked = TxRejectedDisabled
		return false
	}
	if tx.Weight() > 4*int(common.GetUint32(&common.CFG.TXRoute.MaxTxSize)) {
		common.CountSafe("TxRouteTooBig")
		tx.Blocked = TxRejectedTooBig
		return false
	}
	if tx.Fee < (uint64(tx.VSize()) * common.RouteMinFeePerKB() / 1000) {
		common.CountSafe("TxRouteLowFee")
		tx.Blocked = TxRejectedLowFee
		return false
	}
	return true
}

// RetryWaitingForInput -
func RetryWaitingForInput(wtg *OneWaitingList) {
	for k := range wtg.Ids {
		pendtxrcv := &TxRcvd{Tx: TransactionsRejected[k].Tx}
		if HandleNetTx(pendtxrcv, true) {
			common.CountSafe("TxRetryAccepted")
		} else {
			common.CountSafe("TxRetryRejected")
		}
	}
}

// Delete -
// Make sure to call it with locked TxMutex
// Detele the tx fomr mempool.
// Delete all the children as well if withChildren is true
// If reason is not zero, add the deleted txs to the rejected list
func (tx *OneTxToSend) Delete(withChildren bool, reason byte) {
	if withChildren {
		// remove all the children that are spending from tx
		var po btc.TxPrevOut
		po.Hash = tx.Hash.Hash
		for po.Vout = 0; po.Vout < uint32(len(tx.TxOut)); po.Vout++ {
			if so, ok := SpentOutputs[po.UIdx()]; ok {
				if child, ok := TransactionsToSend[so]; ok {
					child.Delete(true, reason)
				}
			}
		}
	}

	for i := range tx.Spent {
		delete(SpentOutputs, tx.Spent[i])
	}

	TransactionsToSendSize -= uint64(len(tx.Raw))
	TransactionsToSendWeight -= uint64(tx.Weight())
	delete(TransactionsToSend, tx.Hash.BIdx())
	if reason != 0 {
		RejectTx(tx.Tx, reason)
	}
}

func txChecker(tx *btc.Tx) bool {
	TxMutex.Lock()
	rec, ok := TransactionsToSend[tx.Hash.BIdx()]
	TxMutex.Unlock()
	if ok && rec.Local {
		common.CountSafe("TxScrOwn")
		return false // Assume own txs as non-trusted
	}
	if ok {
		ok = tx.WTxID().Equal(rec.WTxID())
		if !ok {
			println("wTXID mismatch at", tx.Hash.String(), tx.WTxID().String(), rec.WTxID().String())
			common.CountSafe("TxScrSWErr")
		}
	}
	if ok {
		common.CountSafe("TxScrBoosted")
	} else {
		common.CountSafe("TxScrMissed")
	}
	return ok
}

// Make sure to call it with locked TxMutex
func deleteRejected(bidx BIDX) {
	if tr, ok := TransactionsRejected[bidx]; ok {
		if tr.Waiting4 != nil {
			w4i, _ := WaitingForInputs[tr.Waiting4.BIdx()]
			delete(w4i.Ids, bidx)
			if len(w4i.Ids) == 0 {
				WaitingForInputsSize -= uint64(w4i.TxLen)
				delete(WaitingForInputs, tr.Waiting4.BIdx())
			}
		}
		if tr.Tx != nil {
			TransactionsRejectedSize -= uint64(TransactionsRejected[bidx].Size)
		}
		delete(TransactionsRejected, bidx)
	}
}

// RemoveFromRejected -
func RemoveFromRejected(hash *btc.Uint256) {
	TxMutex.Lock()
	deleteRejected(hash.BIdx())
	TxMutex.Unlock()
}

// SubmitLocalTx -
func SubmitLocalTx(tx *btc.Tx, rawtx []byte) bool {
	return HandleNetTx(&TxRcvd{Tx: tx, trusted: true, local: true}, true)
}

func init() {
	chain.TrustedTxChecker = txChecker
}
