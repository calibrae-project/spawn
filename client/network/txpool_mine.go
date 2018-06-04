// Package network -
package network

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/L"
)

// IIdx -
func (tx *OneTxToSend) IIdx(key uint64) int {
	for i, o := range tx.TxIn {
		if o.Input.UIdx() == key {
			return i
		}
	}
	return -1
}

// UnMarkChildrenForMem - Clear MemInput flag of all the children (used when a tx is mined)
func (tx *OneTxToSend) UnMarkChildrenForMem() {
	// Go through all the tx's outputs and unmark MemInputs in txs that have been spending it
	var po btc.TxPrevOut
	po.Hash = tx.Hash.Hash
	for po.Vout = 0; po.Vout < uint32(len(tx.TxOut)); po.Vout++ {
		uidx := po.UIdx()
		if val, ok := SpentOutputs[uidx]; ok {
			if rec, _ := TransactionsToSend[val]; rec != nil {
				if rec.MemInputs == nil {
					common.CountSafe("TxMinedMeminER1")
					L.Debug("WTF?", po.String(), "just mined in", rec.Hash.String(), "- not marked as mem")
					continue
				}
				idx := rec.IIdx(uidx)
				if idx < 0 {
					common.CountSafe("TxMinedMeminER2")
					L.Debug("WTF?", po.String(), " just mined. Was in SpentOutputs & mempool, but DUPA")
					continue
				}
				rec.MemInputs[idx] = false
				rec.MemInputCnt--
				common.CountSafe("TxMinedMeminOut")
				if rec.MemInputCnt == 0 {
					common.CountSafe("TxMinedMeminTx")
					rec.MemInputs = nil
				}
			} else {
				common.CountSafe("TxMinedMeminERR")
				L.Debug("WTF?", po.String(), " in SpentOutputs, but not in mempool")
			}
		}
	}
}

// This function is called for each tx mined in a new block
func txMined(tx *btc.Tx) (wtg *OneWaitingList) {
	h := tx.Hash
	if rec, ok := TransactionsToSend[h.BIdx()]; ok {
		common.CountSafe("TxMinedToSend")
		rec.UnMarkChildrenForMem()
		rec.Delete(false, 0)
	}
	if mr, ok := TransactionsRejected[h.BIdx()]; ok {
		if mr.Tx != nil {
			common.CountSafe(fmt.Sprint("TxMinedROK-", mr.Reason))
		} else {
			common.CountSafe(fmt.Sprint("TxMinedRNO-", mr.Reason))
		}
		deleteRejected(h.BIdx())
	}
	if _, ok := TransactionsPending[h.BIdx()]; ok {
		common.CountSafe("TxMinedPending")
		delete(TransactionsPending, h.BIdx())
	}

	// Go through all the inputs and make sure we are not leaving them in SpentOutputs
	for i := range tx.TxIn {
		idx := tx.TxIn[i].Input.UIdx()
		if val, ok := SpentOutputs[idx]; ok {
			if rec, _ := TransactionsToSend[val]; rec != nil {
				// if we got here, the txs has been Malleabled
				if rec.Local {
					common.CountSafe("TxMinedMalleabled")
					L.Debug("Input from own ", rec.Tx.Hash.String(), " mined in ", tx.Hash.String())
				} else {
					common.CountSafe("TxMinedOtherSpend")
				}
				rec.Delete(true, 0)
			} else {
				common.CountSafe("TxMinedSpentERROR")
				L.Debug("WTF? Input from ", rec.Tx.Hash.String(), " in mem-spent, but tx not in the mem-pool")
			}
			delete(SpentOutputs, idx)
		}
	}

	wtg = WaitingForInputs[h.BIdx()]
	return
}

// BlockMined - Removes all the block's tx from the mempool
func BlockMined(bl *btc.Block) {
	wtgs := make([]*OneWaitingList, len(bl.Txs)-1)
	var wtgCount int
	TxMutex.Lock()
	for i := 1; i < len(bl.Txs); i++ {
		wtg := txMined(bl.Txs[i])
		if wtg != nil {
			wtgs[wtgCount] = wtg
			wtgCount++
		}
	}
	TxMutex.Unlock()

	// Try to redo waiting txs
	if wtgCount > 0 {
		common.CountSafeAdd("TxMinedGotInput", uint64(wtgCount))
		for _, wtg := range wtgs[:wtgCount] {
			RetryWaitingForInput(wtg)
		}
	}

	expireTxsNow = true
}

// SendGetMP -
func (c *OneConnection) SendGetMP() error {
	TxMutex.Lock()
	tcnt := len(TransactionsToSend) + len(TransactionsRejected)
	if tcnt > MaxGetmpTxs {
		L.Debug("Too many transactions in the current pool")
		TxMutex.Unlock()
		return errors.New("Too many transactions in the current pool")
	}
	b := new(bytes.Buffer)
	btc.WriteVlen(b, uint64(tcnt))
	for k := range TransactionsToSend {
		b.Write(k[:])
	}
	for k := range TransactionsRejected {
		b.Write(k[:])
	}
	TxMutex.Unlock()
	return c.SendRawMsg("getmp", b.Bytes())
}

// ProcessGetMP -
func (c *OneConnection) ProcessGetMP(pl []byte) {
	br := bytes.NewBuffer(pl)

	cnt, er := btc.ReadVLen(br)
	if er != nil {
		L.Debug("getmp message does not have the length field")
		c.DoS("GetMPError1")
		return
	}

	hasThisOne := make(map[BIDX]bool, cnt)
	for i := 0; i < int(cnt); i++ {
		var idx BIDX
		if n, _ := br.Read(idx[:]); n != len(idx) {
			L.Debug("getmp message too short")
			c.DoS("GetMPError2")
			return
		}
		hasThisOne[idx] = true
	}

	var dataSentSoFar int
	var redo [1]byte

	TxMutex.Lock()
	for k, v := range TransactionsToSend {
		if c.BytesToSent() > SendBufSize/4 {
			redo[0] = 1
			break
		}
		if !hasThisOne[k] {
			c.SendRawMsg("tx", v.Raw)
			dataSentSoFar += 24 + len(v.Raw)
		}
	}
	TxMutex.Unlock()

	c.SendRawMsg("getmpdone", redo[:])
}
