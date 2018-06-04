package textui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ParallelCoinTeam/duod"
	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/client/network"
	"github.com/ParallelCoinTeam/duod/client/usif"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/others/peersdb"
	"github.com/ParallelCoinTeam/duod/lib/others/qdb"
	"github.com/ParallelCoinTeam/duod/lib/others/sys"
	"github.com/ParallelCoinTeam/duod/lib/utxo"
)

type oneUIcmd struct {
	cmds    []string // command name
	help    string   // a helf for this command
	sync    bool     // shall be executed in the blochcina therad
	handler func(pars string)
}

var (
	uiCmds     []*oneUIcmd
	showPrompt = true
)

// add a new UI commend handler
func newUI(cmds string, sync bool, hn func(string), help string) {
	cs := strings.Split(cmds, " ")
	if len(cs[0]) > 0 {
		var c = new(oneUIcmd)
		for i := range cs {
			c.cmds = append(c.cmds, cs[i])
		}
		c.sync = sync
		c.help = help
		c.handler = hn
		if len(uiCmds) > 0 {
			var i int
			for i = 0; i < len(uiCmds); i++ {
				if uiCmds[i].cmds[0] > c.cmds[0] {
					break // lets have them sorted
				}
			}
			tmp := make([]*oneUIcmd, len(uiCmds)+1)
			copy(tmp[:i], uiCmds[:i])
			tmp[i] = c
			copy(tmp[i+1:], uiCmds[i:])
			uiCmds = tmp
		} else {
			uiCmds = []*oneUIcmd{c}
		}
	} else {
		panic("empty command string")
	}
}

func readline() string {
	li, _, _ := bufio.NewReader(os.Stdin).ReadLine()
	return string(li)
}

// AskYesNo -
func AskYesNo(msg string) bool {
	for {
		fmt.Print(msg, " (y/n) : ")
		l := strings.ToLower(readline())
		if l == "y" {
			return true
		} else if l == "n" {
			return false
		}
	}
}

// ShowPrompt -
func ShowPrompt() {
	fmt.Print("> ")
}

// MainThread -
func MainThread() {
	time.Sleep(1e9) // hold on for 1 sencond before showing the showPrompt
	for !usif.ExitNow.Get() {
		if showPrompt {
			ShowPrompt()
		}
		showPrompt = true
		li := strings.Trim(readline(), " \n\t\r")
		if len(li) > 0 {
			cmdpar := strings.SplitN(li, " ", 2)
			cmd := cmdpar[0]
			param := ""
			if len(cmdpar) == 2 {
				param = cmdpar[1]
			}
			found := false
			for i := range uiCmds {
				for j := range uiCmds[i].cmds {
					if cmd == uiCmds[i].cmds[j] {
						found = true
						if uiCmds[i].sync {
							usif.ExecUIRequest(&usif.OneUIRequest{Param: param, Handler: uiCmds[i].handler})
							showPrompt = false
						} else {
							uiCmds[i].handler(param)
						}
					}
				}
			}
			if !found {
				fmt.Printf("Unknown command '%s'. Type 'help' for help.\n", cmd)
			}
		}
	}
}

func showInfo(par string) {
	fmt.Println("main.go last seen in line:", common.BusyIn())

	network.MutexRcv.Lock()
	discarded := len(network.DiscardedBlocks)
	cached := network.CachedBlocksLen.Get()
	b2gLen := len(network.BlocksToGet)
	b2gIdxLen := len(network.IndexToBlocksToGet)
	network.MutexRcv.Unlock()

	fmt.Printf("Duod: %s,  Synced: %t,  Uptime %s,  Peers: %d,  ECDSAs: %d\n",
		Duod.Version, common.GetBool(&common.BlockChainSynchronized),
		time.Now().Sub(common.StartTime).String(), btc.EcdsaVerifyCnt(), peersdb.PeerDB.Count())

	// Memory used
	al, sy := sys.MemUsed()
	fmt.Printf("Heap_used: %d MB,  System_used: %d MB,  UTXO-X-mem: %d MB in %d recs,  Saving: %t\n", al>>20, sy>>20,
		utxo.ExtraMemoryConsumed()>>20, utxo.ExtraMemoryAllocCnt(), common.BlockChain.Unspent.WritingInProgress.Get())

	network.MutexRcv.Lock()
	fmt.Println("Last Header:", network.LastCommitedHeader.BlockHash.String(), "@", network.LastCommitedHeader.Height)
	network.MutexRcv.Unlock()

	common.Last.Mutex.Lock()
	fmt.Println("Last Block :", common.Last.Block.BlockHash.String(), "@", common.Last.Block.Height)
	fmt.Printf(" Time: %s (~%s),  Diff: %.0f,  Rcvd: %s ago\n",
		time.Unix(int64(common.Last.Block.Timestamp()), 0).Format("2006/01/02 15:04:05"),
		time.Unix(int64(common.Last.Block.GetMedianTimePast()), 0).Format("15:04:05"),
		btc.GetDifficulty(common.Last.Block.Bits()), time.Now().Sub(common.Last.Time).String())
	common.Last.Mutex.Unlock()

	network.MutexNet.Lock()
	fmt.Printf("Blocks Queued: %d,  Cached: %d,  Discarded: %d,  To Get: %d/%d\n", len(network.NetBlocks),
		cached, discarded, b2gLen, b2gIdxLen)
	network.MutexNet.Unlock()

	network.TxMutex.Lock()
	var swCnt, swBts uint64
	for _, v := range network.TransactionsToSend {
		if v.SegWit != nil {
			swCnt++
			swBts += uint64(v.Size)
		}
	}
	fmt.Printf("Txs in mem pool: %d (%dMB),  SegWit: %d (%dMB),  Rejected: %d (%dMB),  Pending:%d/%d\n",
		len(network.TransactionsToSend), network.TransactionsToSendSize>>20, swCnt, swBts>>20,
		len(network.TransactionsRejected), network.TransactionsRejectedSize>>20,
		len(network.TransactionsPending), len(network.NetTxs))
	fmt.Printf(" WaitingForInputs: %d (%d KB),  SpentOutputs: %d,  AverageFee: %.1f SpB\n",
		len(network.WaitingForInputs), network.WaitingForInputsSize, len(network.SpentOutputs), common.GetAverageFee())
	network.TxMutex.Unlock()

	var gs debug.GCStats
	debug.ReadGCStats(&gs)
	usif.BlockFeesMutex.Lock()
	fmt.Println("Go version:", runtime.Version(), "  LastGC:", time.Now().Sub(gs.LastGC).String(),
		"  NumGC:", gs.NumGC,
		"  PauseTotal:", gs.PauseTotal.String())
	usif.BlockFeesMutex.Unlock()
}

func showCounters(par string) {
	common.CounterMutex.Lock()
	ck := make([]string, 0)
	for k := range common.Counter {
		if par == "" || strings.HasPrefix(k, par) {
			ck = append(ck, k)
		}
	}
	sort.Strings(ck)

	var li string
	for i := range ck {
		k := ck[i]
		v := common.Counter[k]
		s := fmt.Sprint(k, ": ", v)
		if len(li)+len(s) >= 80 {
			fmt.Println(li)
			li = ""
		} else if li != "" {
			li += ",   "
		}
		li += s
	}
	if li != "" {
		fmt.Println(li)
	}
	common.CounterMutex.Unlock()
}

func showPending(par string) {
	network.MutexRcv.Lock()
	for _, v := range network.BlocksToGet {
		fmt.Printf(" * %d / %s / %d in progress\n", v.Block.Height, v.Block.Hash.String(), v.InProgress)
	}
	network.MutexRcv.Unlock()
}

func showHelp(par string) {
	fmt.Println("The following", len(uiCmds), "commands are supported:")
	for i := range uiCmds {
		fmt.Print("   ")
		for j := range uiCmds[i].cmds {
			if j > 0 {
				fmt.Print(", ")
			}
			fmt.Print(uiCmds[i].cmds[j])
		}
		fmt.Println(" -", uiCmds[i].help)
	}
	fmt.Println("All the commands are case sensitive.")
}

func showMem(p string) {
	al, sy := sys.MemUsed()

	fmt.Println("Allocated:", al>>20, "MB")
	fmt.Println("SystemMem:", sy>>20, "MB")

	if p == "" {
		return
	}
	if p == "free" {
		fmt.Println("Freeing the mem...")
		sys.FreeMem()
		showMem("")
		return
	}
	if p == "gc" {
		fmt.Println("Running GC...")
		runtime.GC()
		fmt.Println("Done.")
		return
	}
	i, e := strconv.ParseInt(p, 10, 64)
	if e != nil {
		println(e.Error())
		return
	}
	debug.SetGCPercent(int(i))
	fmt.Println("GC treshold set to", i, "percent")
}

func dumpBlock(s string) {
	h := btc.NewUint256FromString(s)
	if h == nil {
		println("Specify block's hash")
		return
	}
	crec, _, er := common.BlockChain.Blocks.BlockGetExt(btc.NewUint256(h.Hash[:]))
	if er != nil {
		println("BlockGetExt:", er.Error())
		return
	}

	ioutil.WriteFile(h.String()+".bin", crec.Data, 0700)
	fmt.Println("Block saved")

	if crec.Block == nil {
		crec.Block, _ = btc.NewBlock(crec.Data)
	}
	/*
		if crec.Block.NoWitnessData == nil {
			crec.Block.BuildNoWitnessData()
		}
		if !bytes.Equal(crec.Data, crec.Block.NoWitnessData) {
			ioutil.WriteFile(h.String()+".old", crec.Block.NoWitnessData, 0700)
			fmt.Println("Old block saved")
		}
	*/

}

func uiQuit(par string) {
	usif.ExitNow.Set()
}

func blockchainStats(par string) {
	fmt.Println(common.BlockChain.Stats())
}

func blockchainUTXOdb(par string) {
	fmt.Println(common.BlockChain.Unspent.UTXOStats())
}

func setULmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		common.SetUploadLimit(v << 10)
	}
	if common.UploadLimit() != 0 {
		fmt.Printf("Current upload limit is %d KB/s\n", common.UploadLimit()>>10)
	} else {
		fmt.Println("The upload speed is not limited")
	}
}

func setDLmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		common.SetDownloadLimit(v << 10)
	}
	if common.DownloadLimit() != 0 {
		fmt.Printf("Current download limit is %d KB/s\n", common.DownloadLimit()>>10)
	} else {
		fmt.Println("The download speed is not limited")
	}
}

func setConfig(s string) {
	common.LockCfg()
	defer common.UnlockCfg()
	if s != "" {
		new := common.CFG
		e := json.Unmarshal([]byte("{"+s+"}"), &new)
		if e != nil {
			println(e.Error())
		} else {
			common.CFG = new
			common.Reset()
			fmt.Println("Config changed. Execute configsave, if you want to save it.")
		}
	}
	dat, _ := json.MarshalIndent(&common.CFG, "", "    ")
	fmt.Println(string(dat))
}

func loadConfig(s string) {
	d, e := ioutil.ReadFile(common.ConfigFile)
	if e != nil {
		println(e.Error())
		return
	}
	common.LockCfg()
	defer common.UnlockCfg()
	e = json.Unmarshal(d, &common.CFG)
	if e != nil {
		println(e.Error())
		return
	}
	common.Reset()
	fmt.Println("Config reloaded")
}

func saveConfig(s string) {
	common.LockCfg()
	if common.SaveConfig() {
		fmt.Println("Current settings saved to", common.ConfigFile)
	}
	common.UnlockCfg()
}

func showAddresses(par string) {
	fmt.Println(peersdb.PeerDB.Count(), "peers in the database")
	if par == "list" {
		cnt := 0
		peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			cnt++
			fmt.Printf("%4d) %s\n", cnt, peersdb.NewPeer(v).String())
			return 0
		})
	} else if par == "ban" {
		cnt := 0
		peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			pr := peersdb.NewPeer(v)
			if pr.Banned != 0 {
				cnt++
				fmt.Printf("%4d) %s\n", cnt, pr.String())
			}
			return 0
		})
		if cnt == 0 {
			fmt.Println("No banned peers in the DB")
		}
	} else if par != "" {
		limit, er := strconv.ParseUint(par, 10, 32)
		if er != nil {
			fmt.Println("Specify number of best peers to display")
			return
		}
		prs := peersdb.GetBestPeers(uint(limit), nil)
		for i := range prs {
			fmt.Printf("%4d) %s", i+1, prs[i].String())
			if network.ConnectionActive(prs[i]) {
				fmt.Print("  CONNECTED")
			}
			fmt.Print("\n")
		}
	} else {
		fmt.Println("Use 'peers list' to list them")
		fmt.Println("Use 'peers ban' to list the benned ones")
		fmt.Println("Use 'peers <number>' to show the most recent ones")
	}
}

func unbanPeer(par string) {
	if par == "" {
		fmt.Println("Specify IP of the peer to unban or use 'unban all'")
		return
	}

	var ad *peersdb.PeerAddr

	if par != "all" {
		var er error
		ad, er = peersdb.NewAddrFromString(par, false)
		if er != nil {
			fmt.Println(par, er.Error())
			return
		}
		fmt.Println("Unban", ad.IP(), "...")
	} else {
		fmt.Println("Unban all peers ...")
	}

	var keys []qdb.KeyType
	var vals [][]byte
	peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
		peer := peersdb.NewPeer(v)
		if peer.Banned != 0 {
			if ad == nil || peer.IP() == ad.IP() {
				fmt.Println(" -", peer.NetAddr.String())
				peer.Banned = 0
				keys = append(keys, k)
				vals = append(vals, peer.Bytes())
			}
		}
		return 0
	})
	for i := range keys {
		peersdb.PeerDB.Put(keys[i], vals[i])
	}

	fmt.Println(len(keys), "peer(s) un-baned")
}

func showCached(par string) {
	var hi, lo uint32
	for _, v := range network.CachedBlocks {
		//fmt.Printf(" * %s -> %s\n", v.Hash.String(), btc.NewUint256(v.ParentHash()).String())
		if hi == 0 {
			hi = v.Block.Height
			lo = v.Block.Height
		} else if v.Block.Height > hi {
			hi = v.Block.Height
		} else if v.Block.Height < lo {
			lo = v.Block.Height
		}
	}
	fmt.Println(len(network.CachedBlocks), "block cached with heights", lo, "to", hi, hi-lo)
}

func sendInv(par string) {
	cs := strings.Split(par, " ")
	if len(cs) != 2 {
		println("Specify hash and type")
		return
	}
	ha := btc.NewUint256FromString(cs[1])
	if ha == nil {
		println("Incorrect hash")
		return
	}
	v, e := strconv.ParseInt(cs[0], 10, 32)
	if e != nil {
		println("Incorrect type:", e.Error())
		return
	}
	network.NetRouteInv(uint32(v), ha, nil)
	fmt.Println("Inv sent to all peers")
}

func analyzeBIP9(par string) {
	all := par == "all"
	n := common.BlockChain.BlockTreeRoot
	for n != nil {
		var i uint
		startBlock := uint(n.Height)
		startTime := n.Timestamp()
		bits := make(map[byte]uint32)
		for i = 0; i < 2016 && n != nil; i++ {
			ver := n.BlockVersion()
			if (ver & 0x20000000) != 0 {
				for bit := byte(0); bit <= 28; bit++ {
					if (ver & (1 << bit)) != 0 {
						bits[bit]++
					}
				}
			}
			n = n.FindPathTo(common.BlockChain.LastBlock())
		}
		if len(bits) > 0 {
			var s string
			for k, v := range bits {
				if all || v >= common.BlockChain.Consensus.BIP9Threshold {
					if s != "" {
						s += " | "
					}
					s += fmt.Sprint(v, " x bit(", k, ")")
				}
			}
			if s != "" {
				fmt.Println("Period from", time.Unix(int64(startTime), 0).Format("2006/01/02 15:04"),
					" block #", startBlock, "-", startBlock+i-1, ":", s, " - active from", startBlock+2*2016)
			}
		}
	}
}

func switchTrust(par string) {
	if par == "0" {
		common.FLAG.TrustAll = false
	} else if par == "1" {
		common.FLAG.TrustAll = true
	}
	fmt.Println("Assume blocks trusted:", common.FLAG.TrustAll)
}

func saveUXTO(par string) {
	common.BlockChain.Unspent.DirtyDB.Set()
	common.BlockChain.Idle()
}

func purgeUXTO(par string) {
	common.BlockChain.Unspent.PurgeUnspendable(par == "all")
}

func init() {
	newUI("bchain b", true, blockchainStats, "Display blockchain statistics")
	newUI("bip9", true, analyzeBIP9, "Analyze current blockchain for BIP9 bits (add 'all' to see more)")
	newUI("cache", true, showCached, "Show blocks cached in memory")
	newUI("configload cl", false, loadConfig, "Re-load settings from the common file")
	newUI("configsave cs", false, saveConfig, "Save current settings to a common file")
	newUI("configset cfg", false, setConfig, "Set a specific common value - use JSON, omit top {}")
	newUI("counters c", false, showCounters, "Show all kind of debug counters")
	newUI("dlimit dl", false, setDLmax, "Set maximum download speed. The value is in KB/second - 0 for unlimited")
	newUI("help h ?", false, showHelp, "Shows this help")
	newUI("info i", false, showInfo, "Shows general info about the node")
	newUI("inv", false, sendInv, "Send inv message to all the peers - specify type & hash")
	newUI("mem", false, showMem, "Show detailed memory stats (optionally free, gc or a numeric param)")
	newUI("peers", false, showAddresses, "Dump pers database (specify number)")
	newUI("pend", false, showPending, "Show pending blocks, to be fetched")
	newUI("purge", true, purgeUXTO, "Purge unspendable outputs from UTXO database (add 'all' to purge everything)")
	newUI("quit q", false, uiQuit, "Quit the node")
	newUI("savebl", false, dumpBlock, "Saves a block with a given hash to a binary file")
	newUI("saveutxo s", true, saveUXTO, "Save UTXO database now")
	newUI("trust t", true, switchTrust, "Assume all donwloaded blocks trusted (1) or un-trusted (0)")
	newUI("ulimit ul", false, setULmax, "Set maximum upload speed. The value is in KB/second - 0 for unlimited")
	newUI("unban", false, unbanPeer, "Unban a peer specified by IP[:port] (or 'unban all')")
	newUI("utxo u", true, blockchainUTXOdb, "Display UTXO-db statistics")
}
