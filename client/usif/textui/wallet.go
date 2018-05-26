package textui

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/client/network"
	"github.com/ParallelCoinTeam/duod/client/wallet"
	"github.com/ParallelCoinTeam/duod/lib/btc"
)

// OneWalletAddrs -
type OneWalletAddrs struct {
	Typ int // 0-p2kh, 1-p2sh, 2-segwitProg
	Key []byte
	rec *wallet.OneAllAddrBal
}

// SortedWalletAddrs -
type SortedWalletAddrs []OneWalletAddrs

var sortByCount bool

// Len -
func (sk SortedWalletAddrs) Len() int {
	return len(sk)
}

// Less -
func (sk SortedWalletAddrs) Less(a, b int) bool {
	if sortByCount {
		return sk[a].rec.Count() > sk[b].rec.Count()
	}
	return sk[a].rec.Value > sk[b].rec.Value
}

func (sk SortedWalletAddrs) Swap(a, b int) {
	sk[a], sk[b] = sk[b], sk[a]
}

func maxOuts(par string) {
	sortByCount = true
	allAddrs(par)
}

func bestVal(par string) {
	sortByCount = false
	allAddrs(par)
}

func newSlice(in []byte) (kk []byte) {
	kk = make([]byte, len(in))
	copy(kk, in)
	return
}

func allAddrs(par string) {
	var ptkhOuts, ptkhVals, ptshOuts, ptshVals uint64
	var ptwkhOuts, ptwkhVals, ptwshOuts, ptwshVals uint64
	var best SortedWalletAddrs
	var cnt = 15
	var mode int

	if par != "" {
		if c, e := strconv.ParseUint(par, 10, 32); e == nil {
			if c > 3 {
				cnt = int(c)
			} else {
				mode = int(c + 1)
				fmt.Println("Counting only addr type", ([]string{"P2KH", "P2SH", "P2WKH", "P2WSH"})[int(c)])
			}
		}
	}

	var MinBTC uint64 = 100e8
	var MinOuts = 1000

	if mode != 0 {
		MinBTC = 0
		MinOuts = 0
	}

	if mode == 0 || mode == 1 {
		for k, rec := range wallet.AllBalancesP2KH {
			ptkhVals += rec.Value
			ptkhOuts += uint64(rec.Count())
			if sortByCount && rec.Count() >= MinOuts || !sortByCount && rec.Value >= MinBTC {
				best = append(best, OneWalletAddrs{Typ: 0, Key: newSlice(k[:]), rec: rec})
			}
		}
		fmt.Println(btc.UintToBtc(ptkhVals), "BTC in", ptkhOuts, "unspent recs from", len(wallet.AllBalancesP2KH), "P2KH addresses")
	}

	if mode == 0 || mode == 2 {
		for k, rec := range wallet.AllBalancesP2SH {
			ptshVals += rec.Value
			ptshOuts += uint64(rec.Count())
			if sortByCount && rec.Count() >= MinOuts || !sortByCount && rec.Value >= MinBTC {
				best = append(best, OneWalletAddrs{Typ: 1, Key: newSlice(k[:]), rec: rec})
			}
		}
		fmt.Println(btc.UintToBtc(ptshVals), "BTC in", ptshOuts, "unspent recs from", len(wallet.AllBalancesP2SH), "P2SH addresses")
	}

	if mode == 0 || mode == 3 {
		for k, rec := range wallet.AllBalancesP2WKH {
			ptwkhVals += rec.Value
			ptwkhOuts += uint64(rec.Count())
			if sortByCount && rec.Count() >= MinOuts || !sortByCount && rec.Value >= MinBTC {
				best = append(best, OneWalletAddrs{Typ: 2, Key: newSlice(k[:]), rec: rec})
			}
		}
		fmt.Println(btc.UintToBtc(ptwkhVals), "BTC in", ptwkhOuts, "unspent recs from", len(wallet.AllBalancesP2WKH), "P2WKH addresses")
	}

	if mode == 0 || mode == 4 {
		for k, rec := range wallet.AllBalancesP2WSH {
			ptwshVals += rec.Value
			ptwshOuts += uint64(rec.Count())
			if sortByCount && rec.Count() >= MinOuts || !sortByCount && rec.Value >= MinBTC {
				best = append(best, OneWalletAddrs{Typ: 2, Key: newSlice(k[:]), rec: rec})
			}
		}
		fmt.Println(btc.UintToBtc(ptwshVals), "BTC in", ptwshOuts, "unspent recs from", len(wallet.AllBalancesP2WSH), "P2WSH addresses")
	}

	if sortByCount {
		fmt.Println("Top addresses with at least", MinOuts, "unspent outputs:", len(best))
	} else {
		fmt.Println("Top addresses with at least", btc.UintToBtc(MinBTC), "BTC:", len(best))
	}

	sort.Sort(best)

	var pkscrP2sk [23]byte
	var pkscrP2kh [25]byte
	var ad *btc.Addr

	pkscrP2sk[0] = 0xa9
	pkscrP2sk[1] = 20
	pkscrP2sk[22] = 0x87

	pkscrP2kh[0] = 0x76
	pkscrP2kh[1] = 0xa9
	pkscrP2kh[2] = 20
	pkscrP2kh[23] = 0x88
	pkscrP2kh[24] = 0xac

	for i := 0; i < len(best) && i < cnt; i++ {
		switch best[i].Typ {
		case 0:
			copy(pkscrP2kh[3:23], best[i].Key)
			ad = btc.NewAddrFromPkScript(pkscrP2kh[:], common.CFG.Testnet)
		case 1:
			copy(pkscrP2sk[2:22], best[i].Key)
			ad = btc.NewAddrFromPkScript(pkscrP2sk[:], common.CFG.Testnet)
		case 2:
			ad = new(btc.Addr)
			ad.SegwitProg = new(btc.SegwitProg)
			ad.SegwitProg.HRP = btc.GetSegwitHRP(common.CFG.Testnet)
			ad.SegwitProg.Program = best[i].Key
		}
		fmt.Println(i+1, ad.String(), btc.UintToBtc(best[i].rec.Value), "BTC in", best[i].rec.Count(), "inputs")
	}
}

func listUnspent(addr string) {
	fmt.Println("Checking unspent coins for addr", addr)

	ad, e := btc.NewAddrFromString(addr)
	if e != nil {
		println(e.Error())
		return
	}

	outscr := ad.OutScript()

	unsp := wallet.GetAllUnspent(ad)
	if len(unsp) == 0 {
		fmt.Println(ad.String(), "has no coins")
	} else {
		var tot uint64
		sort.Sort(unsp)
		for i := range unsp {
			unsp[i].Addr = nil // no need to print the address here
			tot += unsp[i].Value
		}
		fmt.Println(ad.String(), "has", btc.UintToBtc(tot), "BTC in", len(unsp), "records:")
		for i := range unsp {
			fmt.Println(unsp[i].String())
			network.TxMutex.Lock()
			bidx, spending := network.SpentOutputs[unsp[i].TxPrevOut.UIdx()]
			var t2s *network.OneTxToSend
			if spending {
				t2s, spending = network.TransactionsToSend[bidx]
			}
			network.TxMutex.Unlock()
			if spending {
				fmt.Println("\t- being spent by TxID", t2s.Hash.String())
			}
		}
	}

	network.TxMutex.Lock()
	for _, t2s := range network.TransactionsToSend {
		for vo, to := range t2s.TxOut {
			if bytes.Equal(to.PkScript, outscr) {
				fmt.Println(fmt.Sprintf("Mempool Tx: %15s BTC comming with %s-%03d",
					btc.UintToBtc(to.Value), t2s.Hash.String(), vo))
			}
		}
	}
	network.TxMutex.Unlock()
}

func allValStats(s string) {
	wallet.PrintStat()
}

func walletOnOff(s string) {
	if s == "on" {
		select {
		case wallet.OnOff <- true:
		default:
		}
		return
	} else if s == "off" {
		select {
		case wallet.OnOff <- false:
		default:
		}
		return
	}

	if common.GetBool(&common.WalletON) {
		fmt.Println("Wallet functionality is currently ENABLED. Execute 'wallet off' to disable it.")
		fmt.Println("")
	} else {
		if perc := common.GetUint32(&common.WalletProgress); perc != 0 {
			fmt.Println("Enabling wallet functionality -", (perc-1)/10, "percent complete. Execute 'wallet off' to abort it.")
		} else {
			fmt.Println("Wallet functionality is currently DISABLED. Execute 'wallet on' to enable it.")
		}
	}

	if pend := common.GetUint32(&common.WalletOnIn); pend > 0 {
		fmt.Println("Wallet functionality will auto enable in", pend, "seconds")
	}
}

func init() {
	newUI("richest r", true, bestVal, "Show addresses with most coins [0,1,2,3 or count]")
	newUI("maxouts o", true, maxOuts, "Show addresses with highest number of outputs [0,1,2,3 or count]")
	newUI("balance a", true, listUnspent, "List balance of given bitcoin address")
	newUI("allbal ab", true, allValStats, "Show Allbalance statistics")
	newUI("wallet w", false, walletOnOff, "Enable (on) or disable (off) wallet functionality")
}
