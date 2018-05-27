package main

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/chain"
	"github.com/ParallelCoinTeam/duod/lib/others/sys"
	"github.com/ParallelCoinTeam/duod/lib/utxo"
)

func hostInit() {
	common.DuodHomeDir = common.CFG.Datadir + string(os.PathSeparator)

	common.Testnet = common.CFG.Testnet // So chaging this value would will only affect the behaviour after restart
	if common.CFG.Testnet {             // testnet3
		common.GenesisBlock = btc.NewUint256FromString("00000e41ecbaa35ef91b0c2c22ed4d85fa12bbc87da2668fe17572695fb30cdf")
		common.Magic = [4]byte{0x08, 0xb2, 0x99, 0x88}
		common.DuodHomeDir += common.DataSubdir() + string(os.PathSeparator)
		common.MaxPeersNeeded = 2000
	} else {
		common.GenesisBlock = btc.NewUint256FromString("000009f0fcbad3aac904d3660cfdcf238bf298cfe73adf1d39d14fc5c740ccc7")
		common.Magic = [4]byte{0xcd, 0x08, 0xac, 0xff}
		common.DuodHomeDir += common.DataSubdir() + string(os.PathSeparator)
		common.MaxPeersNeeded = 5000
	}

	// Lock the folder
	os.MkdirAll(common.DuodHomeDir, 0770)
	sys.LockDatabaseDir(common.DuodHomeDir)

	common.SecretKey, _ = ioutil.ReadFile(common.DuodHomeDir + "authkey")
	if len(common.SecretKey) != 32 {
		common.SecretKey = make([]byte, 32)
		rand.Read(common.SecretKey)
		ioutil.WriteFile(common.DuodHomeDir+"authkey", common.SecretKey, 0600)
	}
	common.PublicKey = btc.EncodeBase58(btc.PublicFromPrivate(common.SecretKey, true))
	fmt.Println("Public auth key:", common.PublicKey)

	_Exit := make(chan bool)
	_Done := make(chan bool)
	go func() {
		for {
			select {
			case s := <-common.KillChan:
				fmt.Println(s)
				chain.AbortNow = true
			case <-_Exit:
				_Done <- true
				return
			}
		}
	}()

	if chain.AbortNow {
		sys.UnlockDatabaseDir()
		os.Exit(1)
	}

	if common.CFG.Memory.UseGoHeap {
		fmt.Println("Using native Go heap with the garbage collector for UTXO records")
	} else {
		utxo.MembindInit()
	}

	fmt.Print(string(common.LogBuffer.Bytes()))
	common.LogBuffer = nil

	if btc.ECVerify == nil {
		fmt.Println("Using native secp256k1 lib for ECVerify (consider installing a speedup)")
	}

	ext := &chain.NewChanOpts{
		UTXOVolatileMode: common.FLAG.VolatileUTXO,
		UndoBlocks:       common.FLAG.UndoBlocks,
		BlockMinedCB:     blockMined}

	sta := time.Now()
	common.BlockChain = chain.NewChainExt(common.DuodHomeDir, common.GenesisBlock, common.FLAG.Rescan, ext,
		&chain.BlockDBOpts{
			MaxCachedBlocks: int(common.CFG.Memory.MaxCachedBlks),
			MaxDataFileSize: uint64(common.CFG.Memory.MaxDataFileMB) << 20,
			DataFilesKeep:   common.CFG.Memory.DataFilesKeep})
	if chain.AbortNow {
		fmt.Printf("Blockchain opening aborted after %s seconds\n", time.Now().Sub(sta).String())
		common.BlockChain.Close()
		sys.UnlockDatabaseDir()
		os.Exit(1)
	}

	common.Last.Block = common.BlockChain.LastBlock()
	common.Last.Time = time.Unix(int64(common.Last.Block.Timestamp()), 0)
	if common.Last.Time.After(time.Now()) {
		common.Last.Time = time.Now()
	}

	common.LockCfg()
	common.ApplyLastTrustedBlock()
	common.UnlockCfg()

	if common.CFG.Memory.FreeAtStart {
		fmt.Print("Freeing memory... ")
		sys.FreeMem()
		fmt.Print("\r                  \r")
	}
	sto := time.Now()

	al, sy := sys.MemUsed()
	fmt.Printf("Blockchain open in %s.  %d + %d MB of RAM used (%d)\n",
		sto.Sub(sta).String(), al>>20, utxo.ExtraMemoryConsumed()>>20, sy>>20)

	common.StartTime = time.Now()
	_Exit <- true
	_ = <-_Done

}
