// Package network -
package network

import (
	"sort"
	"time"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/L"
)

var (
	expireTxsNow  = true
	lastTxsExpire time.Time
)

// GetSortedMempool - Return txs sorted by SPB, but with parents first
func GetSortedMempool() (result []*OneTxToSend) {
	allTxs := make([]BIDX, len(TransactionsToSend))
	var idx int
	const MinPkb = 200
	for k := range TransactionsToSend {
		allTxs[idx] = k
		idx++
	}
	sort.Slice(allTxs, func(i, j int) bool {
		recI := TransactionsToSend[allTxs[i]]
		recJ := TransactionsToSend[allTxs[j]]
		rateI := recI.Fee * uint64(recJ.Weight())
		rateJ := recJ.Fee * uint64(recI.Weight())
		if rateI != rateJ {
			return rateI > rateJ
		}
		if recI.MemInputCnt != recJ.MemInputCnt {
			return recI.MemInputCnt < recJ.MemInputCnt
		}
		for x := 0; x < 32; x++ {
			if recI.Hash.Hash[x] != recI.Hash.Hash[x] {
				return recI.Hash.Hash[x] < recI.Hash.Hash[x]
			}
		}
		return false
	})

	// now put the childrer after the parents
	result = make([]*OneTxToSend, len(allTxs))
	alreadyIn := make(map[BIDX]bool, len(allTxs))
	parentOf := make(map[BIDX][]BIDX)

	idx = 0

	var missingParents = func(txkey BIDX, is_any bool) (res []BIDX, yes bool) {
		tx := TransactionsToSend[txkey]
		if tx.MemInputs == nil {
			return
		}
		var countOK int
		for idx, inp := range tx.TxIn {
			if tx.MemInputs[idx] {
				txk := btc.BIdx(inp.Input.Hash[:])
				if _, ok := alreadyIn[txk]; ok {
				} else {
					yes = true
					if is_any {
						return
					}
					res = append(res, txk)
				}

				countOK++
				if countOK == tx.MemInputCnt {
					return
				}
			}
		}
		return
	}

	var appendTxs func(txkey BIDX)
	appendTxs = func(txkey BIDX) {
		result[idx] = TransactionsToSend[txkey]
		idx++
		alreadyIn[txkey] = true

		if toretry, ok := parentOf[txkey]; ok {
			for _, kv := range toretry {
				if _, in := alreadyIn[kv]; in {
					continue
				}
				if _, yes := missingParents(kv, true); !yes {
					appendTxs(kv)
				}
			}
			delete(parentOf, txkey)
		}
	}

	for _, txkey := range allTxs {
		if missing, yes := missingParents(txkey, false); yes {
			for _, kv := range missing {
				parentOf[kv] = append(parentOf[kv], txkey)
			}
			continue
		}
		appendTxs(txkey)
	}

	if idx != len(result) || idx != len(alreadyIn) || len(parentOf) != 0 {
		L.Debug("Get sorted mempool idx:", idx, " result:", len(result), " alreadyin:", len(alreadyIn), " parents:", len(parentOf))
		L.Debug("DUPA!!!!!!!!!!")
		result = result[:idx]
	}

	return
}

// LimitPoolSize - This must be called with TxMutex locked
func LimitPoolSize(maxlen uint64) {
	ticklen := maxlen >> 5 // 1/32th of the max size = X

	if TransactionsToSendSize < maxlen {
		if TransactionsToSendSize < maxlen-2*ticklen {
			if common.SetMinFeePerKB(0) {
				var cnt uint64
				for k, v := range TransactionsRejected {
					if v.Reason == TxRejectedLowFee {
						deleteRejected(k)
						cnt++
					}
				}
				common.CounterMutex.Lock()
				common.Counter["TxPoolSizeLow"]++
				common.Counter["TxRejectedFeeUndone"] += cnt
				common.CounterMutex.Unlock()
				L.Debug("Mempool size low:", TransactionsToSendSize, maxlen, maxlen-2*ticklen, "-", cnt, "rejected purged")
			}
		} else {
			common.CountSafe("TxPoolSizeOK")
			L.Debug("Mempool size OK:", TransactionsToSendSize, maxlen, maxlen-2*ticklen)
		}
		return
	}

	//sta := time.Now()

	sorted := GetSortedMempoolNew()
	idx := len(sorted)

	oldSize := TransactionsToSendSize

	maxlen -= ticklen

	for idx > 0 && TransactionsToSendSize > maxlen {
		idx--
		tx := sorted[idx]
		if _, ok := TransactionsToSend[tx.Hash.BIdx()]; !ok {
			// this has already been rmoved
			continue
		}
		tx.Delete(true, TxRejectedLowFee)
	}

	if cnt := len(sorted) - idx; cnt > 0 {
		newspkb := uint64(float64(1000*sorted[idx].Fee) / float64(sorted[idx].VSize()))
		common.SetMinFeePerKB(newspkb)

		/*fmt.Println("Mempool purged in", time.Now().Sub(sta).String(), "-",
		oldSize-TransactionsToSendSize, "/", oldSize, "bytes and", cnt, "/", len(sorted), "txs removed. SPKB:", newspkb)*/
		common.CounterMutex.Lock()
		common.Counter["TxPoolSizeHigh"]++
		common.Counter["TxPurgedSizCnt"] += uint64(cnt)
		common.Counter["TxPurgedSizBts"] += oldSize - TransactionsToSendSize
		common.CounterMutex.Unlock()
	}
}

// GetSortedRejected -
func GetSortedRejected() (sorted []*OneTxRejected) {
	var idx int
	sorted = make([]*OneTxRejected, len(TransactionsRejected))
	for _, t := range TransactionsRejected {
		sorted[idx] = t
		idx++
	}
	var now = time.Now()
	sort.Slice(sorted, func(i, j int) bool {
		return int64(sorted[i].Size)*int64(now.Sub(sorted[i].Time)) < int64(sorted[j].Size)*int64(now.Sub(sorted[j].Time))
	})
	return
}

// LimitRejectedSize - This must be called with TxMutex locked
func LimitRejectedSize() {
	//ticklen := maxlen >> 5 // 1/32th of the max size = X
	var idx int
	var sorted []*OneTxRejected

	oldCount := len(TransactionsRejected)
	oldSize := TransactionsRejectedSize

	maxlen, maxcnt := common.RejectedTxsLimits()

	if maxcnt > 0 && len(TransactionsRejected) > maxcnt {
		common.CountSafe("TxRejectedCntHigh")
		sorted = GetSortedRejected()
		maxcnt -= maxcnt >> 5
		for idx = maxcnt; idx < len(sorted); idx++ {
			deleteRejected(sorted[idx].Hash.BIdx())
		}
		sorted = sorted[:maxcnt]
	}

	if maxlen > 0 && TransactionsRejectedSize > maxlen {
		common.CountSafe("TxRejectedBtsHigh")
		if sorted == nil {
			sorted = GetSortedRejected()
		}
		maxlen -= maxlen >> 5
		for idx = len(sorted) - 1; idx >= 0; idx-- {
			deleteRejected(sorted[idx].Hash.BIdx())
			if TransactionsRejectedSize <= maxlen {
				break
			}
		}
	}

	if oldCount > len(TransactionsRejected) {
		common.CounterMutex.Lock()
		common.Counter["TxRejectedSizCnt"] += uint64(oldCount - len(TransactionsRejected))
		common.Counter["TxRejectedSizBts"] += oldSize - TransactionsRejectedSize
		if common.GetBool(&common.CFG.TXPool.Debug) {
			L.Debug("Removed", uint64(oldCount-len(TransactionsRejected)), "txs and", oldSize-TransactionsRejectedSize,
				"bytes from the rejected poool")
		}
		common.CounterMutex.Unlock()
	}
}

/* --== Let's keep it here for now as it sometimes comes handy for debuging

var first_ = true

// call this one when TxMutex is locked
func MPC_locked() bool {
	if first_ && MempoolCheck() {
		first_ = false
		_, file, line, _ := runtime.Caller(1)
		println("=====================================================")
		println("Mempool first iime seen broken from", file, line)
		return true
	}
	return false
}

func MPC() (res bool) {
	TxMutex.Lock()
	res = MPC_locked()
	TxMutex.Unlock()
	return
}
*/

// MempoolCheck - Verifies Mempool for consistency
// Make sure to call it with TxMutex Locked
func MempoolCheck() (dupa bool) {
	var spentCount int
	var totsize uint64

	// First check if t2s.MemInputs fields are properly set
	for _, t2s := range TransactionsToSend {
		var micnt int

		totsize += uint64(len(t2s.Raw))

		for i, inp := range t2s.TxIn {
			spentCount++

			outk, ok := SpentOutputs[inp.Input.UIdx()]
			if ok {
				if outk != t2s.Hash.BIdx() {
					L.Debug("Tx", t2s.Hash.String(), "input", i, "has a mismatch in SpentOutputs record", outk)
					dupa = true
				}
			} else {
				L.Debug("Tx", t2s.Hash.String(), "input", i, "is not in SpentOutputs")
				dupa = true
			}

			_, ok = TransactionsToSend[btc.BIdx(inp.Input.Hash[:])]

			if t2s.MemInputs == nil {
				if ok {
					L.Debug("Tx", t2s.Hash.String(), "MemInputs==nil but input", i, "is in mempool", inp.Input.String())
					dupa = true
				}
			} else {
				if t2s.MemInputs[i] {
					micnt++
					if !ok {
						L.Debug("Tx", t2s.Hash.String(), "MemInput set but input", i, "NOT in mempool", inp.Input.String())
						dupa = true
					}
				} else {
					if ok {
						L.Debug("Tx", t2s.Hash.String(), "MemInput NOT set but input", i, "IS in mempool", inp.Input.String())
						dupa = true
					}
				}
			}

			if _, ok := TransactionsToSend[btc.BIdx(inp.Input.Hash[:])]; !ok {
				if unsp := common.BlockChain.Unspent.UnspentGet(&inp.Input); unsp == nil {
					L.Debug("Mempool tx", t2s.Hash.String(), "has no input", i)
					dupa = true
				}
			}
		}
		if t2s.MemInputs != nil && micnt == 0 {
			L.Debug("Tx", t2s.Hash.String(), "has MemInputs array with all false values")
			dupa = true
		}
		if t2s.MemInputCnt != micnt {
			L.Debug("Tx", t2s.Hash.String(), "has incorrect MemInputCnt", t2s.MemInputCnt, micnt)
			dupa = true
		}
	}

	if spentCount != len(SpentOutputs) {
		L.Debug("SpentOutputs length mismatch", spentCount, len(SpentOutputs))
		dupa = true
	}

	if totsize != TransactionsToSendSize {
		L.Debug("TransactionsToSendSize mismatch", totsize, TransactionsToSendSize)
		dupa = true
	}

	totsize = 0
	for _, tr := range TransactionsRejected {
		totsize += uint64(tr.Size)
	}
	if totsize != TransactionsRejectedSize {
		L.Debug("TransactionsRejectedSize mismatch", totsize, TransactionsRejectedSize)
		dupa = true
	}

	return
}

// GetChildren - Get all first level children of the tx
func (tx *OneTxToSend) GetChildren() (result []*OneTxToSend) {
	var po btc.TxPrevOut
	po.Hash = tx.Hash.Hash

	res := make(map[*OneTxToSend]bool)

	for po.Vout = 0; po.Vout < uint32(len(tx.TxOut)); po.Vout++ {
		uidx := po.UIdx()
		if val, ok := SpentOutputs[uidx]; ok {
			res[TransactionsToSend[val]] = true
		}
	}

	result = make([]*OneTxToSend, len(res))
	var idx int
	for ttx := range res {
		result[idx] = ttx
		idx++
	}
	return
}

// GetAllChildren - Get all the children (and all of their children...) of the tx
// The result is sorted by the oldest parent
func (tx *OneTxToSend) GetAllChildren() (result []*OneTxToSend) {
	alreadyIncluded := make(map[*OneTxToSend]bool)
	var idx int
	par := tx
	for {
		chlds := par.GetChildren()
		for _, ch := range chlds {
			if _, ok := alreadyIncluded[ch]; !ok {
				result = append(result, ch)
			}
		}
		if idx == len(result) {
			break
		}

		par = result[idx]
		alreadyIncluded[par] = true
		idx++
	}
	return
}

// GetAllParents - Get all the parents of the given tx
// The result is sorted by the oldest parent
func (tx *OneTxToSend) GetAllParents() (result []*OneTxToSend) {
	alreadyIn := make(map[*OneTxToSend]bool)
	alreadyIn[tx] = true
	var doOne func(*OneTxToSend)
	doOne = func(tx *OneTxToSend) {
		if tx.MemInputCnt > 0 {
			for idx := range tx.TxIn {
				if tx.MemInputs[idx] {
					doOne(TransactionsToSend[btc.BIdx(tx.TxIn[idx].Input.Hash[:])])
				}
			}
		}
		if _, ok := alreadyIn[tx]; !ok {
			result = append(result, tx)
			alreadyIn[tx] = true
		}
	}
	doOne(tx)
	return
}

// SPW -
func (tx *OneTxToSend) SPW() float64 {
	return float64(tx.Fee) / float64(tx.Weight())
}

// SPB -
func (tx *OneTxToSend) SPB() float64 {
	return tx.SPW() * 4.0
}

// OneTxsPackage -
type OneTxsPackage struct {
	Txs    []*OneTxToSend
	Weight int
	Fee    uint64
}

// AnyIn -
func (pk *OneTxsPackage) AnyIn(list map[*OneTxToSend]bool) (ok bool) {
	for _, par := range pk.Txs {
		if _, ok = list[par]; ok {
			return
		}
	}
	return
}

// LookForPackages -
func LookForPackages(txs []*OneTxToSend) (result []*OneTxsPackage) {
	for _, tx := range txs {
		var pkg OneTxsPackage
		parents := tx.GetAllParents()
		if len(parents) > 0 {
			pkg.Txs = append(parents, tx)
			for _, t := range pkg.Txs {
				pkg.Weight += t.Weight()
				pkg.Fee += t.Fee
			}
			result = append(result, &pkg)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Fee*uint64(result[j].Weight) > result[j].Fee*uint64(result[i].Weight)
	})
	return
}

// GetSortedMempoolNew - It is like GetSortedMempool(), but one uses Child-Pays-For-Parent algo
func GetSortedMempoolNew() (result []*OneTxToSend) {
	txs := GetSortedMempool()
	pkgs := LookForPackages(txs)

	result = make([]*OneTxToSend, len(txs))
	var txsIndex, pksIndex, resIndex int
	alreadyIn := make(map[*OneTxToSend]bool, len(txs))
	for txsIndex < len(txs) {
		tx := txs[txsIndex]

		if pksIndex < len(pkgs) {
			pk := pkgs[pksIndex]
			if pk.Fee*uint64(tx.Weight()) > tx.Fee*uint64(pk.Weight) {
				pksIndex++
				if pk.AnyIn(alreadyIn) {
					continue
				}
				// all package's txs new: incude them all
				copy(result[resIndex:], pk.Txs)
				resIndex += len(pk.Txs)
				for _, _t := range pk.Txs {
					alreadyIn[_t] = true
				}
				continue
			}
		}

		txsIndex++
		if _, ok := alreadyIn[tx]; ok {
			continue
		}
		result[resIndex] = tx
		alreadyIn[tx] = true
		resIndex++
	}
	L.Debug("All sorted.  resIndex:", resIndex, "  txs:", len(txs))
	return
}

// GetMempoolFees - Only take tx/package weight and the fee
func GetMempoolFees(maxweight uint64) (result [][2]uint64) {
	txs := GetSortedMempool()
	pkgs := LookForPackages(txs)

	var txsIndex, pksIndex, resIndex int
	var weightsofar uint64
	result = make([][2]uint64, len(txs))
	alreadyIn := make(map[*OneTxToSend]bool, len(txs))
	for txsIndex < len(txs) && weightsofar < maxweight {
		tx := txs[txsIndex]

		if pksIndex < len(pkgs) {
			pk := pkgs[pksIndex]
			if pk.Fee*uint64(tx.Weight()) > tx.Fee*uint64(pk.Weight) {
				pksIndex++
				if pk.AnyIn(alreadyIn) {
					continue
				}

				result[resIndex] = [2]uint64{uint64(pk.Weight), pk.Fee}
				resIndex++
				weightsofar += uint64(pk.Weight)

				for _, _t := range pk.Txs {
					alreadyIn[_t] = true
				}
				continue
			}
		}

		txsIndex++
		if _, ok := alreadyIn[tx]; ok {
			continue
		}
		result[resIndex] = [2]uint64{uint64(tx.Weight()), tx.Fee}
		resIndex++
		weightsofar += uint64(tx.Weight())

		alreadyIn[tx] = true
	}
	result = result[:resIndex]
	return
}

// ExpireTxs -
func ExpireTxs() {
	lastTxsExpire = time.Now()
	expireTxsNow = false

	TxMutex.Lock()

	if maxpoolsize := common.MaxMempoolSize(); maxpoolsize != 0 {
		LimitPoolSize(maxpoolsize)
	}

	LimitRejectedSize()

	TxMutex.Unlock()

	common.CountSafe("TxPurgedTicks")
}
