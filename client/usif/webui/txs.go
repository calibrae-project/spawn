package webui

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/calibrae-project/spawn/client/common"
	"github.com/calibrae-project/spawn/client/network"
	"github.com/calibrae-project/spawn/client/usif"
	"github.com/calibrae-project/spawn/lib/btc"
	"github.com/calibrae-project/spawn/lib/script"
)

func pTxs(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var txloadresult string
	var wg sync.WaitGroup
	var tx2in []byte

	// Check if there is a tx upload request
	r.ParseMultipartForm(2e6)
	fil, _, _ := r.FormFile("txfile")
	if fil != nil {
		tx2in, _ = ioutil.ReadAll(fil)
	} else if len(r.Form["rawtx"]) == 1 {
		tx2in, _ = hex.DecodeString(r.Form["rawtx"][0])
	}

	if len(tx2in) > 0 {
		wg.Add(1)
		req := &usif.OneUiReq{Param: string(tx2in)}
		req.Done.Add(1)
		req.Handler = func(dat string) {
			txloadresult = usif.LoadRawTx([]byte(dat))
			wg.Done()
		}
		usif.UiChannel <- req
	}

	s := loadTemplate("txs.html")

	wg.Wait()
	if txloadresult != "" {
		ld := loadTemplate("txs_load.html")
		ld = strings.Replace(ld, "{TX_RAW_DATA}", txloadresult, 1)
		s = strings.Replace(s, "<!--TX_LOAD-->", ld, 1)
	}

	if common.CFG.TXPool.Enabled {
		s = strings.Replace(s, "<!--MEM_POOL_ENABLED-->", "Enabled", 1)
	} else {
		s = strings.Replace(s, "<!--MEM_POOL_ENABLED-->", "Disabled", 1)
	}

	if common.CFG.TXRoute.Enabled {
		s = strings.Replace(s, "<!--TX_ROUTE_ENABLED-->", "Enabled", 1)
	} else {
		s = strings.Replace(s, "<!--TX_ROUTE_ENABLED-->", "Disabled", 1)
	}

	writeHTMLHead(w, r)
	w.Write([]byte(s))
	writeHTMLTail(w)
}

// outputTxXML -
func outputTxXML(w http.ResponseWriter, tx *btc.Tx) {
	w.Write([]byte("<input_list>"))
	for i := range tx.TxIn {
		w.Write([]byte("<input>"))
		w.Write([]byte("<script_sig>"))
		w.Write([]byte(hex.EncodeToString(tx.TxIn[i].ScriptSig)))
		w.Write([]byte("</script_sig>"))
		fmt.Fprint(w, "<txid-vout>", tx.TxIn[i].Input.String(), "</txid-vout>")
		var po *btc.TxOut
		inpid := btc.NewUint256(tx.TxIn[i].Input.Hash[:])
		if txinmem, ok := network.TransactionsToSend[inpid.BIdx()]; ok {
			if int(tx.TxIn[i].Input.Vout) < len(txinmem.TxOut) {
				po = txinmem.TxOut[tx.TxIn[i].Input.Vout]
			}
		} else {
			po = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
		}
		if po != nil {
			ok := script.VerifyTxScript(po.Pk_script, po.Value, i, tx, script.STANDARD_VERIFY_FLAGS)
			if !ok {
				w.Write([]byte("<status>Script FAILED</status>"))
			} else {
				w.Write([]byte("<status>OK</status>"))
			}
			fmt.Fprint(w, "<value>", po.Value, "</value>")
			fmt.Fprint(w, "<pkscript>", hex.EncodeToString(po.Pk_script), "</pkscript>")
			if ad := btc.NewAddrFromPkScript(po.Pk_script, common.Testnet); ad != nil {
				fmt.Fprint(w, "<addr>", ad.String(), "</addr>")
			}
			fmt.Fprint(w, "<block>", po.BlockHeight, "</block>")

			if btc.IsP2SH(po.Pk_script) {
				fmt.Fprint(w, "<input_sigops>", btc.WITNESS_SCALE_FACTOR*btc.GetP2SHSigOpCount(tx.TxIn[i].ScriptSig), "</input_sigops>")
			}
			fmt.Fprint(w, "<witness_sigops>", tx.CountWitnessSigOps(i, po.Pk_script), "</witness_sigops>")
		} else {
			w.Write([]byte("<status>Unknown input</status>"))
		}
		fmt.Fprint(w, "<sequence>", tx.TxIn[i].Sequence, "</sequence>")

		if tx.SegWit != nil {
			w.Write([]byte("<segwit>"))
			for _, wit := range tx.SegWit[i] {
				w.Write([]byte("<witness>"))
				w.Write([]byte(hex.EncodeToString(wit)))
				w.Write([]byte("</witness>"))
			}
			w.Write([]byte("</segwit>"))
		}
		w.Write([]byte("</input>"))
	}
	w.Write([]byte("</input_list>"))

	w.Write([]byte("<output_list>"))
	for i := range tx.TxOut {
		w.Write([]byte("<output>"))
		fmt.Fprint(w, "<value>", tx.TxOut[i].Value, "</value>")
		adr := btc.NewAddrFromPkScript(tx.TxOut[i].Pk_script, common.Testnet)
		if adr != nil {
			fmt.Fprint(w, "<addr>", adr.String(), "</addr>")
		} else {
			fmt.Fprint(w, "<addr>", "scr:"+hex.EncodeToString(tx.TxOut[i].Pk_script), "</addr>")
		}
		w.Write([]byte("</output>"))
	}
	w.Write([]byte("</output_list>"))
}

func txXML(w http.ResponseWriter, v *network.OneTxToSend, verbose bool) {
	w.Write([]byte("<tx><status>OK</status>"))
	fmt.Fprint(w, "<id>", v.Tx.Hash.String(), "</id>")
	fmt.Fprint(w, "<version>", v.Tx.Version, "</version>")
	fmt.Fprint(w, "<time>", v.Firstseen.Unix(), "</time>")
	if int(v.Size) != len(v.Raw) {
		panic("TX size does not match data length")
	}

	fmt.Fprint(w, "<size>", v.Size, "</size>")
	fmt.Fprint(w, "<nwsize>", v.NoWitSize, "</nwsize>")
	fmt.Fprint(w, "<weight>", v.Weight(), "</weight>")
	fmt.Fprint(w, "<sw_compress>", 1000*(int(v.Size)-int(v.NoWitSize))/int(v.Size), "</sw_compress>")
	fmt.Fprint(w, "<inputs>", len(v.TxIn), "</inputs>")
	fmt.Fprint(w, "<outputs>", len(v.TxOut), "</outputs>")
	fmt.Fprint(w, "<lock_time>", v.Lock_time, "</lock_time>")
	fmt.Fprint(w, "<witness_cnt>", len(v.SegWit), "</witness_cnt>")
	if verbose {
		outputTxXML(w, v.Tx)
	}
	fmt.Fprint(w, "<own>", v.Local, "</own>")
	fmt.Fprint(w, "<firstseen>", v.Firstseen.Unix(), "</firstseen>")
	fmt.Fprint(w, "<invsentcnt>", v.Invsentcnt, "</invsentcnt>")
	fmt.Fprint(w, "<sigops>", v.SigopsCost, "</sigops>")
	fmt.Fprint(w, "<sentcnt>", v.SentCnt, "</sentcnt>")
	fmt.Fprint(w, "<sentlast>", v.Lastsent.Unix(), "</sentlast>")
	fmt.Fprint(w, "<volume>", v.Volume, "</volume>")
	fmt.Fprint(w, "<fee>", v.Fee, "</fee>")
	fmt.Fprint(w, "<blocked>", network.ReasonToString(v.Blocked), "</blocked>")
	fmt.Fprint(w, "<final>", v.Final, "</final>")
	fmt.Fprint(w, "<verify_us>", uint(v.VerifyTime/time.Microsecond), "</verify_us>")
	w.Write([]byte("</tx>"))
}

func outputUXTOTxXML(w http.ResponseWriter, minedid, minedat string) {
	txid := btc.NewUint256FromString(minedid)
	if txid == nil {
		return
	}

	blockNumber, er := strconv.ParseUint(minedat, 10, 32)
	if er != nil {
		return
	}

	lck := new(usif.OneLock)
	lck.In.Add(1)
	lck.Out.Add(1)
	usif.LocksChan <- lck
	lck.In.Wait()

	w.Write([]byte("<tx>"))
	fmt.Fprint(w, "<id>", minedid, "</id>")
	if dat, er := common.GetRawTx(uint32(blockNumber), txid); er == nil {
		w.Write([]byte("<status>OK</status>"))
		w.Write([]byte(fmt.Sprint("<size>", len(dat), "</size>")))
		tx, _ := btc.NewTx(dat)
		outputTxXML(w, tx)
	} else {
		w.Write([]byte("<status>Not found</status>"))
	}
	w.Write([]byte("</tx>"))

	lck.Out.Done()

}

/* memory pool transaction sorting stuff */
type sortedTxList []*network.OneTxToSend

func (tl sortedTxList) Len() int      { return len(tl) }
func (tl sortedTxList) Swap(i, j int) { tl[i], tl[j] = tl[j], tl[i] }
func (tl sortedTxList) Less(i, j int) bool {
	var res bool
	switch tx2sSort {
	case "age":
		res = tl[j].Firstseen.UnixNano() > tl[i].Firstseen.UnixNano()
	case "siz":
		res = tl[j].Size < tl[i].Size
	case "nws":
		res = tl[j].NoWitSize < tl[i].NoWitSize
	case "wgh":
		res = tl[j].Weight() < tl[i].Weight()
	case "inp":
		res = len(tl[j].TxIn) < len(tl[i].TxIn)
	case "out":
		res = len(tl[j].TxOut) < len(tl[i].TxOut)
	case "btc":
		res = tl[j].Volume < tl[i].Volume
	case "fee":
		res = tl[j].Fee < tl[i].Fee
	case "ops":
		res = tl[j].SigopsCost < tl[i].SigopsCost
	case "rbf":
		res = !tl[j].Final && tl[i].Final
	case "ver":
		res = int(tl[j].VerifyTime) < int(tl[i].VerifyTime)
	case "swc":
		swComprI := float64(int(tl[i].Size)-int(tl[i].NoWitSize)) / float64(tl[i].Size)
		swComprJ := float64(int(tl[j].Size)-int(tl[j].NoWitSize)) / float64(tl[j].Size)
		res = swComprI > swComprJ
	default: /*spb*/
		spbI := float64(tl[i].Fee) / float64(tl[i].Weight())
		spbJ := float64(tl[j].Fee) / float64(tl[j].Weight())
		res = spbJ < spbI
	}
	if tx2sSortDesc {
		return res
	}
	return !res
}

var tx2sCount = 1000
var tx2sSort = "spb"
var tx2sSortDesc = true

func xmlTxs2s(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}

	if len(r.Form["minedid"]) > 0 && len(r.Form["minedat"]) > 0 {
		outputUXTOTxXML(w, r.Form["minedid"][0], r.Form["minedat"][0])
		return
	}

	if len(r.Form["id"]) > 0 {
		txid := btc.NewUint256FromString(r.Form["id"][0])
		if txid == nil {
			return
		}
		network.TxMutex.Lock()
		defer network.TxMutex.Unlock()
		if t2s, ok := network.TransactionsToSend[txid.BIdx()]; ok {
			txXML(w, t2s, true)
		} else {
			w.Write([]byte("<tx>"))
			fmt.Fprint(w, "<id>", txid.String(), "</id>")
			w.Write([]byte("<status>Not found</status>"))
			w.Write([]byte("</tx>"))
		}
		return
	}

	if checksid(r) {
		if len(r.Form["del"]) > 0 {
			tid := btc.NewUint256FromString(r.Form["del"][0])
			if tid != nil {
				network.TxMutex.Lock()
				if tts, ok := network.TransactionsToSend[tid.BIdx()]; ok {
					tts.Delete(true, 0)
				}
				network.TxMutex.Unlock()
			}
		}

		if len(r.Form["send"]) > 0 {
			tid := btc.NewUint256FromString(r.Form["send"][0])
			if tid != nil {
				network.TxMutex.Lock()
				if ptx, ok := network.TransactionsToSend[tid.BIdx()]; ok {
					network.TxMutex.Unlock()
					cnt := network.NetRouteInv(1, tid, nil)
					if cnt == 0 {
						usif.SendInvToRandomPeer(1, tid)
					} else {
						ptx.Invsentcnt += cnt
					}
				} else {
					network.TxMutex.Unlock()
				}
			}
		}

		if len(r.Form["sendone"]) > 0 {
			tid := btc.NewUint256FromString(r.Form["sendone"][0])
			if tid != nil {
				network.TxMutex.Lock()
				if ptx, ok := network.TransactionsToSend[tid.BIdx()]; ok {
					network.TxMutex.Unlock()
					usif.SendInvToRandomPeer(1, tid)
					ptx.Invsentcnt++
				} else {
					network.TxMutex.Unlock()
				}
			}
		}

		if len(r.Form["quiet"]) > 0 {
			return
		}

		if len(r.Form["cnt"]) > 0 {
			u, e := strconv.ParseUint(r.Form["cnt"][0], 10, 32)
			if e == nil && u > 0 && u < 10e3 {
				tx2sCount = int(u)
			}
		}

		if len(r.Form["sort"]) > 0 && len(r.Form["sort"][0]) == 3 {
			tx2sSort = r.Form["sort"][0]
		}

		tx2sSortDesc = len(r.Form["descending"]) > 0
	}

	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	sorted := make(sortedTxList, len(network.TransactionsToSend))
	var cnt int
	for _, v := range network.TransactionsToSend {
		if len(r.Form["ownonly"]) > 0 && !v.Local {
			continue
		}
		sorted[cnt] = v
		cnt++
	}
	sorted = sorted[:cnt]
	sort.Sort(sorted)

	w.Write([]byte("<txpool>"))
	for cnt = 0; cnt < len(sorted) && cnt < tx2sCount; cnt++ {
		v := sorted[cnt]
		txXML(w, v, false)
	}
	w.Write([]byte("</txpool>"))
}

func xmlTxSre(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<txbanned>"))
	network.TxMutex.Lock()
	for _, v := range network.TransactionsRejected {
		w.Write([]byte("<tx>"))
		fmt.Fprint(w, "<id>", v.ID.String(), "</id>")
		fmt.Fprint(w, "<time>", v.Time.Unix(), "</time>")
		fmt.Fprint(w, "<size>", v.Size, "</size>")
		fmt.Fprint(w, "<reason>", network.ReasonToString(v.Reason), "</reason>")
		w.Write([]byte("</tx>"))
	}
	network.TxMutex.Unlock()
	w.Write([]byte("</txbanned>"))
}

func xmlTxW4i(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"text/xml"}
	w.Write([]byte("<pending>"))
	network.TxMutex.Lock()
	for _, v := range network.WaitingForInputs {
		w.Write([]byte("<wait4>"))
		fmt.Fprint(w, "<id>", v.TxID.String(), "</id>")
		for x, t := range v.Ids {
			w.Write([]byte("<tx>"))
			if v, ok := network.TransactionsRejected[x]; ok {
				fmt.Fprint(w, "<id>", v.ID.String(), "</id>")
				fmt.Fprint(w, "<time>", t.Unix(), "</time>")
			} else {
				fmt.Fprint(w, "<id>FATAL ERROR!!! This should not happen! Please report</id>")
				fmt.Fprint(w, "<time>", time.Now().Unix(), "</time>")
			}
			w.Write([]byte("</tx>"))
		}
		w.Write([]byte("</wait4>"))
	}
	network.TxMutex.Unlock()
	w.Write([]byte("</pending>"))
}

func rawTx(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(w, "Error")
			if err, ok := r.(error); ok {
				fmt.Fprintln(w, err.Error())
			}
		}
	}()

	if len(r.Form["id"]) == 0 {
		fmt.Println("No id given")
		return
	}

	txid := btc.NewUint256FromString(r.Form["id"][0])
	fmt.Fprintln(w, "TxID:", txid.String())
	if tx, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		s, _, _, _, _ := usif.DecodeTx(tx.Tx)
		w.Write([]byte(s))
	} else {
		fmt.Fprintln(w, "Not found")
	}
}

func jsonTxStat(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}
	w.Header()["Content-Type"] = []string{"application/json"}
	w.Write([]byte("{"))

	network.TxMutex.Lock()

	w.Write([]byte(fmt.Sprint("\"t2s_cnt\":", len(network.TransactionsToSend), ",")))
	w.Write([]byte(fmt.Sprint("\"t2s_size\":", network.TransactionsToSendSize, ",")))
	w.Write([]byte(fmt.Sprint("\"tre_cnt\":", len(network.TransactionsRejected), ",")))
	w.Write([]byte(fmt.Sprint("\"tre_size\":", network.TransactionsRejectedSize, ",")))
	w.Write([]byte(fmt.Sprint("\"ptr1_cnt\":", len(network.TransactionsPending), ",")))
	w.Write([]byte(fmt.Sprint("\"ptr2_cnt\":", len(network.NetTxs), ",")))
	w.Write([]byte(fmt.Sprint("\"spent_outs_cnt\":", len(network.SpentOutputs), ",")))
	w.Write([]byte(fmt.Sprint("\"awaiting_inputs\":", len(network.WaitingForInputs), ",")))
	w.Write([]byte(fmt.Sprint("\"awaiting_inputs_size\":", network.WaitingForInputsSize, ",")))
	w.Write([]byte(fmt.Sprint("\"min_fee_per_kb\":", common.MinFeePerKB(), "")))

	network.TxMutex.Unlock()

	w.Write([]byte("}\n"))
}

func txtMempoolFees(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}
	w.Header()["Content-Type"] = []string{"text/plain"}
	w.Write([]byte(usif.MemoryPoolFees()))
}

func jsonMempoolStats(w http.ResponseWriter, r *http.Request) {
	var division, maxweight uint64
	var e error

	if !ipchecker(r) {
		return
	}

	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	if len(r.Form["max"]) > 0 {
		maxweight, e = strconv.ParseUint(r.Form["max"][0], 10, 64)
		if e != nil {
			maxweight = network.TransactionsToSendWeight
		}
	} else {
		maxweight = network.TransactionsToSendWeight
	}

	if maxweight > network.TransactionsToSendWeight {
		maxweight = network.TransactionsToSendWeight
	}

	if len(r.Form["div"]) > 0 {
		division, e = strconv.ParseUint(r.Form["div"][0], 10, 64)
		if e != nil {
			division = maxweight / 100
		}
	} else {
		division = maxweight / 100
	}

	if division < 100 {
		division = 100
	}

	var sorted []*network.OneTxToSend
	if len(r.Form["new"]) > 0 {
		sorted = network.GetSortedMempoolNew()
	} else {
		sorted = network.GetSortedMempool()
	}

	type oneStatRow struct {
		TxsSoFar        uint
		RealLenSoFar    uint
		WeightSoFar     uint
		CurrentTxWeight uint
		CurrentTxSpb    float64
		CurrentTxID     string
		TimeReceived    uint
		FeesSoFar       uint64
	}
	var mempoolStats []oneStatRow

	var totweight, reallen, totfee uint64
	for cnt := 0; cnt < len(sorted); cnt++ {
		v := sorted[cnt]
		newtotweight := totweight + uint64(v.Weight())
		reallen += uint64(len(v.Raw))
		totfee += v.Fee

		if cnt == 0 || cnt+1 == len(sorted) || (newtotweight/division) != (totweight/division) {
			currSpb := float64(v.Fee) / (float64(v.Weight() / 4.0))
			mempoolStats = append(mempoolStats, oneStatRow{
				TxsSoFar:        uint(cnt),
				RealLenSoFar:    uint(reallen),
				WeightSoFar:     uint(totweight),
				CurrentTxWeight: uint(v.Weight()),
				CurrentTxSpb:    currSpb,
				CurrentTxID:     v.Hash.String(),
				FeesSoFar:       totfee,
				TimeReceived:    uint(v.Firstseen.Unix())})
		}
		totweight = newtotweight
		if totweight >= maxweight {
			break
		}
	}

	bx, er := json.Marshal(mempoolStats)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}

func jsonMempoolFees(w http.ResponseWriter, r *http.Request) {
	var division, maxweight uint64
	var e error

	if !ipchecker(r) {
		return
	}

	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	if len(r.Form["max"]) > 0 {
		maxweight, e = strconv.ParseUint(r.Form["max"][0], 10, 64)
		if e != nil {
			maxweight = network.TransactionsToSendWeight
		}
	} else {
		maxweight = network.TransactionsToSendWeight
	}

	if maxweight > network.TransactionsToSendWeight {
		maxweight = network.TransactionsToSendWeight
	}

	if len(r.Form["div"]) > 0 {
		division, e = strconv.ParseUint(r.Form["div"][0], 10, 64)
		if e != nil {
			division = maxweight / 100
		}
	} else {
		division = maxweight / 100
	}

	if division < 1 {
		division = 1
	}

	sorted := network.GetMempoolFees(maxweight)

	var mempoolStats [][3]uint64
	var totweight uint64
	var totfeessofar uint64
	for cnt := range sorted {
		wgh := sorted[cnt][0]
		fee := sorted[cnt][1]
		totfeessofar += fee
		newtotweight := totweight + wgh

		if cnt == 0 || cnt+1 == len(sorted) || (newtotweight/division) != (totweight/division) {
			mempoolStats = append(mempoolStats, [3]uint64{newtotweight, 4000 * fee / wgh, totfeessofar})
		}
		totweight = newtotweight
	}

	bx, er := json.Marshal(mempoolStats)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
