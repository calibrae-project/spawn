package webui

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/client/network"
	"github.com/ParallelCoinTeam/duod/client/usif"
	"github.com/ParallelCoinTeam/duod/client/wallet"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/utxo"
)

func pWal(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if !common.GetBool(&common.WalletON) {
		pWalletIsOff(w, r)
		return
	}

	var str string
	common.Last.Mutex.Lock()
	if common.BlockChain.Consensus.EnforceSegwit != 0 &&
		common.Last.Block.Height >= common.BlockChain.Consensus.EnforceSegwit {
		str = "var segwit_active=true"
	} else {
		str = "var segwit_active=false"
	}
	common.Last.Mutex.Unlock()
	page := loadTemplate("wallet.html")
	page = strings.Replace(page, "/*WALLET_JS_VARS*/", str, 1)
	writeHTMLHead(w, r)
	w.Write([]byte(page))
	writeHTMLTail(w)
}

func getaddrtype(aa *btc.Addr) string {
	if aa.SegwitProg != nil && aa.SegwitProg.Version == 0 && len(aa.SegwitProg.Program) == 20 {
		return "P2WPKH"
	}
	if aa.Version == btc.AddrVerPubkey(common.Testnet) {
		return "P2PKH"
	}
	if aa.Version == btc.AddrVerScript(common.Testnet) {
		return "P2SH"
	}
	return "unknown"
}

func jsonBalance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) || !common.GetBool(&common.WalletON) {
		return
	}

	if r.Method != "POST" {
		return
	}

	summary := len(r.Form["summary"]) > 0
	mempool := len(r.Form["mempool"]) > 0
	getrawtx := len(r.Form["rawtx"]) > 0

	inp, er := ioutil.ReadAll(r.Body)
	if er != nil {
		println(er.Error())
		return
	}

	var addrs []string
	er = json.Unmarshal(inp, &addrs)
	if er != nil {
		println(er.Error())
		return
	}

	type OneOut struct {
		TxID     string
		Vout     uint32
		Value    uint64
		Height   uint32
		Coinbase bool
		Message  string
		Addr     string
		AddrType string
		Spending bool   // if true the spending tx is in the mempool
		RawTx    string `json:",omitempty"`
	}

	type OneOuts struct {
		Value            uint64
		OutCnt           int
		SegWitCnt        int
		SegWitAddr       string
		SegWitNativeCnt  int
		SegWitNativeAddr string
		Outs             []OneOut

		PendingCnt   int
		PendingValue uint64
		PendingOuts  []OneOut

		SpendingValue uint64
		SpendingCnt   uint64
	}

	out := make(map[string]*OneOuts)

	lck := new(usif.OneLock)
	lck.In.Add(1)
	lck.Out.Add(1)
	usif.LocksChan <- lck
	lck.In.Wait()

	var addrMap map[string]string

	if mempool {
		// make addrs -> idx
		addrMap = make(map[string]string, 2*len(addrs))
	}

	for _, a := range addrs {
		aa, e := btc.NewAddrFromString(a)
		if e != nil {
			continue
		}

		unsp := wallet.GetAllUnspent(aa)
		newrec := new(OneOuts)
		if len(unsp) > 0 {
			newrec.OutCnt = len(unsp)
			for _, u := range unsp {
				newrec.Value += u.Value
				network.TxMutex.Lock()
				_, spending := network.SpentOutputs[u.TxPrevOut.UIdx()]
				network.TxMutex.Unlock()
				if spending {
					newrec.SpendingValue += u.Value
					newrec.SpendingCnt++
				}
				if !summary {
					txid := btc.NewUint256(u.TxPrevOut.Hash[:])
					var rawtx string
					if getrawtx {
						dat, er := common.GetRawTx(uint32(u.MinedAt), txid)
						if er == nil {
							rawtx = hex.EncodeToString(dat)
						}
					}
					newrec.Outs = append(newrec.Outs, OneOut{
						TxID: btc.NewUint256(u.TxPrevOut.Hash[:]).String(), Vout: u.Vout,
						Value: u.Value, Height: u.MinedAt, Coinbase: u.Coinbase,
						Message: html.EscapeString(string(u.Message)), Addr: a, Spending: spending,
						RawTx: rawtx, AddrType: getaddrtype(aa)})
				}
			}
		}

		out[a] = newrec

		if mempool {
			addrMap[string(aa.OutScript())] = a
		}

		/* For P2KH addr, we wlso check its segwit's P2SH-P2WPKH and Native P2WPKH */
		if aa.SegwitProg == nil && aa.Version == btc.AddrVerPubkey(common.Testnet) {
			p2kh := aa.Hash160

			// P2SH SegWit if applicable
			h160 := btc.Rimp160AfterSha256(append([]byte{0, 20}, p2kh[:]...))
			aa = btc.NewAddrFromHash160(h160[:], btc.AddrVerScript(common.Testnet))
			newrec.SegWitAddr = aa.String()
			unsp = wallet.GetAllUnspent(aa)
			if len(unsp) > 0 {
				newrec.OutCnt += len(unsp)
				newrec.SegWitCnt = len(unsp)
				as := aa.String()
				for _, u := range unsp {
					newrec.Value += u.Value
					network.TxMutex.Lock()
					_, spending := network.SpentOutputs[u.TxPrevOut.UIdx()]
					network.TxMutex.Unlock()
					if spending {
						newrec.SpendingValue += u.Value
						newrec.SpendingCnt++
					}
					if !summary {
						txid := btc.NewUint256(u.TxPrevOut.Hash[:])
						var rawtx string
						if getrawtx {
							dat, er := common.GetRawTx(uint32(u.MinedAt), txid)
							if er == nil {
								rawtx = hex.EncodeToString(dat)
							}
						}
						newrec.Outs = append(newrec.Outs, OneOut{
							TxID: txid.String(), Vout: u.Vout,
							Value: u.Value, Height: u.MinedAt, Coinbase: u.Coinbase,
							Message: html.EscapeString(string(u.Message)), Addr: as,
							Spending: spending, RawTx: rawtx, AddrType: "P2SH-P2WPKH"})
					}
				}
			}
			if mempool {
				addrMap[string(aa.OutScript())] = a
			}

			// Native SegWit if applicable
			aa = btc.NewAddrFromPkScript(append([]byte{0, 20}, p2kh[:]...), common.Testnet)
			newrec.SegWitNativeAddr = aa.String()
			unsp = wallet.GetAllUnspent(aa)
			if len(unsp) > 0 {
				newrec.OutCnt += len(unsp)
				newrec.SegWitNativeCnt = len(unsp)
				as := aa.String()
				for _, u := range unsp {
					newrec.Value += u.Value
					network.TxMutex.Lock()
					_, spending := network.SpentOutputs[u.TxPrevOut.UIdx()]
					network.TxMutex.Unlock()
					if spending {
						newrec.SpendingValue += u.Value
						newrec.SpendingCnt++
					}
					if !summary {
						txid := btc.NewUint256(u.TxPrevOut.Hash[:])
						var rawtx string
						if getrawtx {
							dat, er := common.GetRawTx(uint32(u.MinedAt), txid)
							if er == nil {
								rawtx = hex.EncodeToString(dat)
							}
						}
						newrec.Outs = append(newrec.Outs, OneOut{
							TxID: txid.String(), Vout: u.Vout,
							Value: u.Value, Height: u.MinedAt, Coinbase: u.Coinbase,
							Message: html.EscapeString(string(u.Message)), Addr: as,
							Spending: spending, RawTx: rawtx, AddrType: "P2WPKH"})
					}
				}
			}
			if mempool {
				addrMap[string(aa.OutScript())] = a
			}

		}
	}

	// check memory pool
	if mempool {
		network.TxMutex.Lock()
		for _, t2s := range network.TransactionsToSend {
			for vo, to := range t2s.TxOut {
				if a, ok := addrMap[string(to.PkScript)]; ok {
					newrec := out[a]
					newrec.PendingValue += to.Value
					newrec.PendingCnt++
					if !summary {
						po := &btc.TxPrevOut{Hash: t2s.Hash.Hash, Vout: uint32(vo)}
						_, spending := network.SpentOutputs[po.UIdx()]
						newrec.PendingOuts = append(newrec.PendingOuts, OneOut{
							TxID: t2s.Hash.String(), Vout: uint32(vo),
							Value: to.Value, Spending: spending})
					}
				}
			}
		}
		network.TxMutex.Unlock()
	}

	lck.Out.Done()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}

func dlBalance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) || !common.GetBool(&common.WalletON) {
		return
	}

	if r.Method != "POST" {
		return
	}

	var addrs []string
	var labels []string

	if len(r.Form["addrcnt"]) != 1 {
		println("no addrcnt")
		return
	}
	addrcnt, _ := strconv.ParseUint(r.Form["addrcnt"][0], 10, 32)

	for i := 0; i < int(addrcnt); i++ {
		is := fmt.Sprint(i)
		if len(r.Form["addr"+is]) == 1 {
			addrs = append(addrs, r.Form["addr"+is][0])
			if len(r.Form["label"+is]) == 1 {
				labels = append(labels, r.Form["label"+is][0])
			} else {
				labels = append(labels, "")
			}
		}
	}

	type oneUnspRec struct {
		btc.TxPrevOut
		Value    uint64
		Addr     string
		MinedAt  uint32
		Coinbase bool
	}

	var thisbal utxo.AllUnspentTx

	lck := new(usif.OneLock)
	lck.In.Add(1)
	lck.Out.Add(1)
	usif.LocksChan <- lck
	lck.In.Wait()

	for idx, a := range addrs {
		aa, e := btc.NewAddrFromString(a)
		aa.Extra.Label = labels[idx]
		if e == nil {
			newrecs := wallet.GetAllUnspent(aa)
			if len(newrecs) > 0 {
				thisbal = append(thisbal, newrecs...)
			}

			/* Segwit P2WPKH: */
			if aa.SegwitProg == nil && aa.Version == btc.AddrVerPubkey(common.Testnet) {
				p2kh := aa.Hash160

				// P2SH SegWit if applicable
				h160 := btc.Rimp160AfterSha256(append([]byte{0, 20}, aa.Hash160[:]...))
				aa = btc.NewAddrFromHash160(h160[:], btc.AddrVerScript(common.Testnet))
				newrecs = wallet.GetAllUnspent(aa)
				if len(newrecs) > 0 {
					thisbal = append(thisbal, newrecs...)
				}

				// Native SegWit if applicable
				aa = btc.NewAddrFromPkScript(append([]byte{0, 20}, p2kh[:]...), common.Testnet)
				newrecs = wallet.GetAllUnspent(aa)
				if len(newrecs) > 0 {
					thisbal = append(thisbal, newrecs...)
				}
			}
		}
	}
	lck.Out.Done()

	buf := new(bytes.Buffer)
	zi := zip.NewWriter(buf)
	wasTx := make(map[[32]byte]bool)

	sort.Sort(thisbal)
	for i := range thisbal {
		if wasTx[thisbal[i].TxPrevOut.Hash] {
			continue
		}
		wasTx[thisbal[i].TxPrevOut.Hash] = true
		txid := btc.NewUint256(thisbal[i].TxPrevOut.Hash[:])
		fz, _ := zi.Create("balance/" + txid.String() + ".tx")
		if dat, er := common.GetRawTx(thisbal[i].MinedAt, txid); er == nil {
			fz.Write(dat)
		} else {
			println(er.Error())
		}
	}

	fz, _ := zi.Create("balance/unspent.txt")
	for i := range thisbal {
		fmt.Fprintln(fz, thisbal[i].UnspentTextLine())
	}

	zi.Close()
	w.Header()["Content-Type"] = []string{"application/zip"}
	w.Write(buf.Bytes())

}

func jsonWalletStatus(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var out struct {
		WalletON       bool
		WalletProgress uint32
		WalletOnIn     uint32
	}
	common.LockCfg()
	out.WalletON = common.WalletON
	out.WalletProgress = common.WalletProgress
	out.WalletOnIn = common.WalletOnIn
	common.UnlockCfg()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
