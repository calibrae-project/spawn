package textui

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/client/network"
	"github.com/ParallelCoinTeam/duod/client/usif"
	"github.com/ParallelCoinTeam/duod/lib/btc"
)

func loadTx(par string) {
	if par == "" {
		fmt.Println("Specify a name of a transaction file")
		return
	}
	f, e := os.Open(par)
	if e != nil {
		println(e.Error())
		return
	}
	n, _ := f.Seek(0, os.SEEK_END)
	f.Seek(0, os.SEEK_SET)
	buf := make([]byte, n)
	f.Read(buf)
	f.Close()
	fmt.Println(usif.LoadRawTx(buf))
}

func sendTx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid == nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		listTxs("")
		return
	}
	network.TxMutex.Lock()
	if ptx, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		network.TxMutex.Unlock()
		cnt := network.NetRouteInv(1, txid, nil)
		ptx.Invsentcnt += cnt
		fmt.Println("INV for TxID", txid.String(), "sent to", cnt, "node(s)")
		fmt.Println("If it does not appear in the chain, you may want to redo it.")
	} else {
		network.TxMutex.Unlock()
		fmt.Println("No such transaction ID in the memory pool.")
		listTxs("")
	}
}

func send1Tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid == nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		listTxs("")
		return
	}
	network.TxMutex.Lock()
	if ptx, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		network.TxMutex.Unlock()
		usif.SendInvToRandomPeer(1, txid)
		ptx.Invsentcnt++
		fmt.Println("INV for TxID", txid.String(), "sent to a random node")
		fmt.Println("If it does not appear in the chain, you may want to redo it.")
	} else {
		network.TxMutex.Unlock()
		fmt.Println("No such transaction ID in the memory pool.")
		listTxs("")
	}
}

func delTx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid == nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		listTxs("")
		return
	}
	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()
	tx, ok := network.TransactionsToSend[txid.BIdx()]
	if !ok {
		network.TxMutex.Unlock()
		fmt.Println("No such transaction ID in the memory pool.")
		listTxs("")
		return
	}
	tx.Delete(true, 0)
	fmt.Println("Transaction", txid.String(), "and all its children removed from the memory pool")
}

func decTx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid == nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		listTxs("")
		return
	}
	if tx, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		s, _, _, _, _ := usif.DecodeTx(tx.Tx)
		fmt.Println(s)
	} else {
		fmt.Println("No such transaction ID in the memory pool.")
	}
}

func saveTx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid == nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		listTxs("")
		return
	}
	if tx, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		fn := tx.Hash.String() + ".tx"
		ioutil.WriteFile(fn, tx.Raw, 0600)
		fmt.Println("Saved to", fn)
	} else {
		fmt.Println("No such transaction ID in the memory pool.")
	}
}

func mempoolStats(par string) {
	fmt.Print(usif.MemoryPoolFees())
}

func listTxs(par string) {
	limitbytes, _ := strconv.ParseUint(par, 10, 64)
	fmt.Println("Transactions in the memory pool:", limitbytes)
	cnt := 0
	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	sorted := network.GetSortedMempool()

	var totlen uint64
	for cnt = 0; cnt < len(sorted); cnt++ {
		v := sorted[cnt]
		totlen += uint64(len(v.Raw))

		if limitbytes != 0 && totlen > limitbytes {
			break
		}

		var oe, snt string
		if v.Local {
			oe = " *OWN*"
		} else {
			oe = ""
		}

		snt = fmt.Sprintf("INV sent %d times,   ", v.Invsentcnt)

		if v.SentCnt == 0 {
			snt = "never sent"
		} else {
			snt = fmt.Sprintf("sent %d times, last %s ago", v.SentCnt,
				time.Now().Sub(v.Lastsent).String())
		}

		spb := float64(v.Fee) / float64(len(v.Raw))

		fmt.Println(fmt.Sprintf("%5d) ...%10d %s  %6d bytes / %6.1fspb - %s%s", cnt, totlen, v.Tx.Hash.String(), len(v.Raw), spb, snt, oe))

	}
}

func bannedTxs(par string) {
	fmt.Println("Rejected transactions:")
	cnt := 0
	network.TxMutex.Lock()
	for k, v := range network.TransactionsRejected {
		cnt++
		fmt.Println("", cnt, btc.NewUint256(k[:]).String(), "-", v.Size, "bytes",
			"-", v.Reason, "-", time.Now().Sub(v.Time).String(), "ago")
	}
	network.TxMutex.Unlock()
}

func sendAllTxs(par string) {
	network.TxMutex.Lock()
	for k, v := range network.TransactionsToSend {
		if v.Local {
			cnt := network.NetRouteInv(1, btc.NewUint256(k[:]), nil)
			v.Invsentcnt += cnt
			fmt.Println("INV for TxID", v.Hash.String(), "sent to", cnt, "node(s)")
		}
	}
	network.TxMutex.Unlock()
}

func saveMempool(par string) {
	network.MempoolSave(true)
}

func checkTxs(par string) {
	network.TxMutex.Lock()
	network.MempoolCheck()
	network.TxMutex.Unlock()
}

func loadMempool(par string) {
	if par == "" {
		par = common.DuodHomeDir + "mempool.dmp"
	}
	var abort bool
	_Exit := make(chan bool)
	_Done := make(chan bool)
	go func() {
		for {
			select {
			case s := <-common.KillChan:
				fmt.Println(s)
				abort = true
			case <-_Exit:
				_Done <- true
				return
			}
		}
	}()
	fmt.Println("Press Ctrl+C to abort...")
	network.MempoolLoadNew(par, &abort)
	_Exit <- true
	_ = <-_Done
	if abort {
		fmt.Println("Aborted")
	}
}

func getMempool(par string) {
	conid, e := strconv.ParseUint(par, 10, 32)
	if e != nil {
		fmt.Println("Specify ID of the peer")
		return
	}

	fmt.Println("Getting mempool from connection ID", conid, "...")
	network.GetMP(uint32(conid))
}

func init() {
	newUI("txload tx", true, loadTx, "Load transaction data from the given file, decode it and store in memory")
	newUI("txsend stx", true, sendTx, "Broadcast transaction from memory pool (identified by a given <txid>)")
	newUI("tx1send stx1", true, send1Tx, "Broadcast transaction to a single random peer (identified by a given <txid>)")
	newUI("txsendall stxa", true, sendAllTxs, "Broadcast all the transactions (what you see after ltx)")
	newUI("txdel dtx", true, delTx, "Remove a transaction from memory pool (identified by a given <txid>)")
	newUI("txdecode td", true, decTx, "Decode a transaction from memory pool (identified by a given <txid>)")
	newUI("txlist ltx", true, listTxs, "List all the transaction loaded into memory pool up to 1MB space <max_size>")
	newUI("txlistban ltxb", true, bannedTxs, "List the transaction that we have rejected")
	newUI("mempool mp", true, mempoolStats, "Show the mempool statistics")
	newUI("txsave", true, saveTx, "Save raw transaction from memory pool to disk")
	newUI("txmpsave mps", true, saveMempool, "Save memory pool to disk")
	newUI("txcheck txc", true, checkTxs, "Verify consistency of mempool")
	newUI("txmpload mpl", true, loadMempool, "Load transaction from the given file (must be in mempool.dmp format)")
	newUI("getmp mpg", true, getMempool, "Get getmp message to the peer with teh given ID")
}
