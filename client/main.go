package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"time"
	"unsafe"

	"github.com/ParallelCoinTeam/duod"
	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/client/network"
	"github.com/ParallelCoinTeam/duod/client/rpcapi"
	"github.com/ParallelCoinTeam/duod/client/usif"
	"github.com/ParallelCoinTeam/duod/client/usif/textui"
	"github.com/ParallelCoinTeam/duod/client/usif/webui"
	"github.com/ParallelCoinTeam/duod/client/wallet"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/chain"
	"github.com/ParallelCoinTeam/duod/lib/L"
	"github.com/ParallelCoinTeam/duod/lib/others/peersdb"
	"github.com/ParallelCoinTeam/duod/lib/others/qdb"
	"github.com/ParallelCoinTeam/duod/lib/others/sys"
)

var (
	retryCachedBlocks bool
	// SaveBlockChain -
	SaveBlockChain = time.NewTimer(24 * time.Hour)
)

const (
	// SaveBlockChainAfter -
	SaveBlockChainAfter = 2 * time.Second
)

func resetSaveTimer() {
	SaveBlockChain.Stop()
	for len(SaveBlockChain.C) > 0 {
		<-SaveBlockChain.C
	}
	SaveBlockChain.Reset(SaveBlockChainAfter)
}

func blockMined(bl *btc.Block) {
	network.BlockMined(bl)
	if int(bl.LastKnownHeight)-int(bl.Height) < 144 { // do not run it when syncing chain
		usif.ProcessBlockFees(bl.Height, bl)
	}
}

// LocalAcceptBlock -
func LocalAcceptBlock(newbl *network.BlockRcvd) (e error) {
	bl := newbl.Block
	if common.FLAG.TrustAll || newbl.BlockTreeNode.Trusted {
		bl.Trusted = true
	}

	common.BlockChain.Unspent.AbortWriting() // abort saving of UTXO.db
	common.BlockChain.Blocks.BlockAdd(newbl.BlockTreeNode.Height, bl)
	newbl.TmQueue = time.Now()

	if newbl.DoInvs {
		common.Busy()
		network.NetRouteInv(network.MsgBlock, bl.Hash, newbl.Conn)
	}

	network.MutexRcv.Lock()
	bl.LastKnownHeight = network.LastCommitedHeader.Height
	network.MutexRcv.Unlock()
	e = common.BlockChain.CommitBlock(bl, newbl.BlockTreeNode)

	if e == nil {
		// new block accepted
		newbl.TmAccepted = time.Now()

		newbl.NonWitnessSize = bl.NoWitnessSize

		common.RecalcAverageBlockSize()

		common.Last.Mutex.Lock()
		common.Last.Time = time.Now()
		common.Last.Block = common.BlockChain.LastBlock()
		common.Last.Mutex.Unlock()

		resetSaveTimer()
	} else {
		L.Debug("Warning: AcceptBlock failed. If the block was valid, you may need to rebuild the unspent DB (-r)")
		newEnd := common.BlockChain.LastBlock()
		common.Last.Mutex.Lock()
		common.Last.Block = newEnd
		common.Last.Mutex.Unlock()
		// update network.LastCommitedHeader
		network.MutexRcv.Lock()
		if network.LastCommitedHeader != newEnd {
			network.LastCommitedHeader = newEnd
			L.Debug("LastCommitedHeader moved to", network.LastCommitedHeader.Height)
		}
		network.DiscardedBlocks[newbl.Hash.BIdx()] = true
		network.MutexRcv.Unlock()
	}
	return
}

// RetryCachedBlocks -
func RetryCachedBlocks() bool {
	var idx int
	common.CountSafe("RedoCachedBlks")
	for idx < len(network.CachedBlocks) {
		newbl := network.CachedBlocks[idx]
		if CheckParentDiscarded(newbl.BlockTreeNode) {
			common.CountSafe("DiscardCachedBlock")
			if newbl.Block == nil {
				os.Remove(common.TempBlocksDir() + newbl.BlockTreeNode.BlockHash.String())
			}
			network.CachedBlocks = append(network.CachedBlocks[:idx], network.CachedBlocks[idx+1:]...)
			network.CachedBlocksLen.Store(len(network.CachedBlocks))
			return len(network.CachedBlocks) > 0
		}
		if common.BlockChain.HasAllParents(newbl.BlockTreeNode) {
			common.Busy()

			if newbl.Block == nil {
				tmpfn := common.TempBlocksDir() + newbl.BlockTreeNode.BlockHash.String()
				dat, e := ioutil.ReadFile(tmpfn)
				os.Remove(tmpfn)
				if e != nil {
					panic(e.Error())
				}
				if newbl.Block, e = btc.NewBlock(dat); e != nil {
					panic(e.Error())
				}
				if e = newbl.Block.BuildTxList(); e != nil {
					panic(e.Error())
				}
				newbl.Block.BlockExtraInfo = *newbl.BlockExtraInfo
			}

			e := LocalAcceptBlock(newbl)
			if e != nil {
				L.Debug("AcceptBlock2", newbl.BlockTreeNode.BlockHash.String(), "-", e.Error())
				newbl.Conn.Misbehave("LocalAcceptBl2", 250)
			}
			if usif.ExitNow.Get() {
				return false
			}
			// remove it from cache
			network.CachedBlocks = append(network.CachedBlocks[:idx], network.CachedBlocks[idx+1:]...)
			network.CachedBlocksLen.Store(len(network.CachedBlocks))
			return len(network.CachedBlocks) > 0
		}
		idx++
	}
	return false
}

// CheckParentDiscarded -
// Return true iof the block's parent is on the DiscardedBlocks list
// Add it to DiscardedBlocks, if returning true
func CheckParentDiscarded(n *chain.BlockTreeNode) bool {
	network.MutexRcv.Lock()
	defer network.MutexRcv.Unlock()
	if network.DiscardedBlocks[n.Parent.BlockHash.BIdx()] {
		network.DiscardedBlocks[n.BlockHash.BIdx()] = true
		return true
	}
	return false
}

// HandleNetBlock -
// Called from the blockchain thread
func HandleNetBlock(newbl *network.BlockRcvd) {
	defer func() {
		common.CountSafe("MainNetBlock")
		if common.GetUint32(&common.WalletOnIn) > 0 {
			common.SetUint32(&common.WalletOnIn, 5) // snooze the timer to 5 seconds from now
		}
	}()

	if CheckParentDiscarded(newbl.BlockTreeNode) {
		common.CountSafe("DiscardFreshBlockA")
		if newbl.Block == nil {
			os.Remove(common.TempBlocksDir() + newbl.BlockTreeNode.BlockHash.String())
		}
		retryCachedBlocks = len(network.CachedBlocks) > 0
		return
	}

	if !common.BlockChain.HasAllParents(newbl.BlockTreeNode) {
		// it's not linking - keep it for later
		network.CachedBlocks = append(network.CachedBlocks, newbl)
		network.CachedBlocksLen.Store(len(network.CachedBlocks))
		common.CountSafe("BlockPostone")
		return
	}

	if newbl.Block == nil {
		tmpfn := common.TempBlocksDir() + newbl.BlockTreeNode.BlockHash.String()
		dat, e := ioutil.ReadFile(tmpfn)
		os.Remove(tmpfn)
		if e != nil {
			panic(e.Error())
		}
		if newbl.Block, e = btc.NewBlock(dat); e != nil {
			panic(e.Error())
		}
		if e = newbl.Block.BuildTxList(); e != nil {
			panic(e.Error())
		}
		newbl.Block.BlockExtraInfo = *newbl.BlockExtraInfo
	}

	common.Busy()
	if e := LocalAcceptBlock(newbl); e != nil {
		common.CountSafe("DiscardFreshBlockB")
		L.Debug("AcceptBlock1", newbl.Block.Hash.String(), "-", e.Error())
		newbl.Conn.Misbehave("LocalAcceptBl1", 250)
	} else {
		//println("block", newbl.Block.Height, "accepted")
		retryCachedBlocks = RetryCachedBlocks()
	}
}

// HandleRPCblock -
func HandleRPCblock(msg *rpcapi.BlockSubmitted) {
	common.CountSafe("RPCNewBlock")

	network.MutexRcv.Lock()
	rb := network.ReceivedBlocks[msg.Block.Hash.BIdx()]
	network.MutexRcv.Unlock()
	if rb == nil {
		panic("Block " + msg.Block.Hash.String() + " not in ReceivedBlocks map")
	}

	common.BlockChain.Unspent.AbortWriting()
	rb.TmQueue = time.Now()

	_, _, e := common.BlockChain.CheckBlock(msg.Block)
	if e == nil {
		e = common.BlockChain.AcceptBlock(msg.Block)
		rb.TmAccepted = time.Now()
	}
	if e != nil {
		common.CountSafe("RPCBlockError")
		msg.Error = e.Error()
		msg.Done.Done()
		return
	}

	network.NetRouteInv(network.MsgBlock, msg.Block.Hash, nil)
	common.RecalcAverageBlockSize()

	common.CountSafe("RPCBlockOK")
	L.Debug("New mined block", msg.Block.Height, "accepted OK in", rb.TmAccepted.Sub(rb.TmQueue).String())

	common.Last.Mutex.Lock()
	common.Last.Time = time.Now()
	common.Last.Block = common.BlockChain.LastBlock()
	common.Last.Mutex.Unlock()

	msg.Done.Done()
}

func main() {
	var ptr *byte
	if unsafe.Sizeof(ptr) < 8 {
		L.Warn("Duod client shall be build for 64-bit arch. It will likely crash now.")
	}

	L.Info("Duod client version", Duod.Version)
	runtime.GOMAXPROCS(runtime.NumCPU()) // It seems that Go does not do it by default

	// Disable Ctrl+C
	signal.Notify(common.KillChan, os.Interrupt, os.Kill)
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("pkg: %v", r)
			}
			L.Debug("main panic recovered:", err.Error())
			L.Debug(string(debug.Stack()))
			network.NetCloseAll()
			common.CloseBlockChain()
			peersdb.ClosePeerDB()
			sys.UnlockDatabaseDir()
			os.Exit(1)
		}
	}()

	common.InitConfig()

	if common.FLAG.SaveConfig {
		common.SaveConfig()
		L.Debug("Configuration file saved")
		os.Exit(0)
	}

	if common.FLAG.VolatileUTXO {
		L.Warn("Using UTXO database in a volatile mode. Make sure to close the client properly (do not kill it!)")
	}

	if common.FLAG.TrustAll {
		L.Warn("Assuming all scripts inside new blocks to PASS. Verify the last block's hash when finished.")
	}

	hostInit() // This will create the DB lock file and keep it open

	os.RemoveAll(common.TempBlocksDir())
	common.MkTempBlocksDir()

	if common.FLAG.UndoBlocks > 0 {
		usif.ExitNow.Set()
	}

	if common.FLAG.Rescan && common.FLAG.VolatileUTXO {

		L.Debug("UTXO database rebuilt complete in the volatile mode, so flush DB to disk and exit...")

	} else if !usif.ExitNow.Get() {

		common.RecalcAverageBlockSize()

		peersTick := time.Tick(5 * time.Minute)
		netTick := time.Tick(time.Second)

		resetSaveTimer() // we wil do one save try after loading, in case if ther was a rescan

		peersdb.Testnet = common.Testnet
		peersdb.ConnectOnly = common.CFG.ConnectOnly
		peersdb.Services = common.Services
		peersdb.InitPeers(common.DuodHomeDir)
		if common.FLAG.UnbanAllPeers {
			var keys []qdb.KeyType
			var vals [][]byte
			peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
				peer := peersdb.NewPeer(v)
				if peer.Banned != 0 {
					L.Debug("Unban", peer.NetAddr.String())
					peer.Banned = 0
					keys = append(keys, k)
					vals = append(vals, peer.Bytes())
				}
				return 0
			})
			for i := range keys {
				peersdb.PeerDB.Put(keys[i], vals[i])
			}

			L.Debug(len(keys), "peers unbanned")
		}

		for k, v := range common.BlockChain.BlockIndex {
			network.ReceivedBlocks[k] = &network.OneReceivedBlock{TmStart: time.Unix(int64(v.Timestamp()), 0)}
		}
		network.LastCommitedHeader = common.Last.Block

		if common.CFG.TXPool.SaveOnDisk {
			network.MempoolLoad2()
		}

		if common.CFG.TextUIEnabled {
			go textui.MainThread()
		}

		if common.CFG.WebUI.Interface != "" {
			L.Infof("Starting WebUI at http://%s\n", common.CFG.WebUI.Interface)
			go webui.ServerThread(common.CFG.WebUI.Interface)
		}

		if common.CFG.RPC.Enabled {
			go rpcapi.StartServer(common.RPCPort())
		}

		usif.LoadBlockFees()

		wallet.FetchingBalanceTick = func() bool {
			select {
			case rec := <-usif.LocksChan:
				common.CountSafe("DoMainLocks")
				rec.In.Done()
				rec.Out.Wait()

			case newtx := <-network.NetTxs:
				common.CountSafe("DoMainNetTx")
				network.HandleNetTx(newtx, false)

			case <-netTick:
				common.CountSafe("DoMainNetTick")
				network.Ticking()

			case on := <-wallet.OnOff:
				if !on {
					return true
				}

			default:
			}
			return usif.ExitNow.Get()
		}

		startupTicks := 5 // give 5 seconds for finding out missing blocks
		if !common.FLAG.NoWallet {
			// snooze the timer to 10 seconds after startupTicks goes down
			common.SetUint32(&common.WalletOnIn, 10)
		}

		for !usif.ExitNow.Get() {
			common.Busy()

			common.CountSafe("MainThreadLoops")
			for retryCachedBlocks {
				retryCachedBlocks = RetryCachedBlocks()
				// We have done one per loop - now do something else if pending...
				if len(network.NetBlocks) > 0 || len(usif.UIChannel) > 0 {
					break
				}
			}

			// first check for priority messages; kill signal or a new block
			select {
			case <-common.KillChan:
				common.Busy()
				usif.ExitNow.Set()
				continue

			case newbl := <-network.NetBlocks:
				common.Busy()
				HandleNetBlock(newbl)

			case rpcbl := <-rpcapi.RPCBlocks:
				common.Busy()
				HandleRPCblock(rpcbl)

			default: // timeout immediatelly if no priority message
			}

			common.Busy()

			select {
			case <-common.KillChan:
				common.Busy()
				usif.ExitNow.Set()
				continue

			case newbl := <-network.NetBlocks:
				common.Busy()
				HandleNetBlock(newbl)

			case rpcbl := <-rpcapi.RPCBlocks:
				common.Busy()
				HandleRPCblock(rpcbl)

			case rec := <-usif.LocksChan:
				common.Busy()
				common.CountSafe("MainLocks")
				rec.In.Done()
				rec.Out.Wait()

			case <-SaveBlockChain.C:
				common.Busy()
				common.CountSafe("SaveBlockChain")
				if common.BlockChain.Idle() {
					common.CountSafe("ChainIdleUsed")
				}

			case newtx := <-network.NetTxs:
				common.Busy()
				common.CountSafe("MainNetTx")
				network.HandleNetTx(newtx, false)

			case <-netTick:
				common.Busy()
				common.CountSafe("MainNetTick")
				network.Ticking()
				if startupTicks > 0 {
					startupTicks--
					break
				}
				if !common.BlockChainSynchronized && network.BlocksToGetCnt() == 0 &&
					len(network.NetBlocks) == 0 && network.CachedBlocksLen.Get() == 0 {
					// only when we have no pending blocks startupTicks go down..
					common.SetBool(&common.BlockChainSynchronized, true)
				}
				if common.WalletPendingTick() {
					wallet.OnOff <- true
				}

			case cmd := <-usif.UIChannel:
				common.Busy()
				common.CountSafe("MainUICmd")
				cmd.Handler(cmd.Param)
				cmd.Done.Done()

			case <-peersTick:
				common.Busy()
				peersdb.ExpirePeers()
				usif.ExpireBlockFees()

			case on := <-wallet.OnOff:
				common.Busy()
				if on {
					wallet.LoadBalance()
				} else {
					wallet.Disable()
					common.SetUint32(&common.WalletOnIn, 0)
				}
			}
		}

		common.BlockChain.Unspent.HurryUp()
		wallet.UpdateMapSizes()
		network.NetCloseAll()
	}

	sta := time.Now()
	common.CloseBlockChain()
	if common.FLAG.UndoBlocks == 0 {
		network.MempoolSave(false)
	}
	L.Debug("Blockchain closed in ", time.Now().Sub(sta).String())
	peersdb.ClosePeerDB()
	usif.SaveBlockFees()
	sys.UnlockDatabaseDir()
	os.RemoveAll(common.TempBlocksDir())
	L.Debug("Completed shutdown")
	L.DebugNoInfo("\n\n")
}
