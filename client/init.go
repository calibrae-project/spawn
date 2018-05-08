package main

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/calibrae-project/spawn/client/common"
	"github.com/calibrae-project/spawn/lib/btc"
	"github.com/calibrae-project/spawn/lib/chain"
	"github.com/calibrae-project/spawn/lib/others/sys"
	"github.com/calibrae-project/spawn/lib/utxo"
)

func hostInit() {
	common.SpawnHomeDir = common.CFG.Datadir + string(os.PathSeparator)

	common.Testnet = common.CFG.Testnet // So chaging this value would will only affect the behaviour after restart
	if common.CFG.Testnet {             // testnet3
		common.GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		common.Magic = [4]byte{0x0B, 0x11, 0x09, 0x07}
		common.SpawnHomeDir += common.DataSubdir() + string(os.PathSeparator)
		common.MaxPeersNeeded = 2000
	} else {
		common.GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		common.Magic = [4]byte{0xF9, 0xBE, 0xB4, 0xD9}
		common.SpawnHomeDir += common.DataSubdir() + string(os.PathSeparator)
		common.MaxPeersNeeded = 5000
	}

	// Lock the folder
	os.MkdirAll(common.SpawnHomeDir, 0770)
	sys.LockDatabaseDir(common.SpawnHomeDir)

	common.SecretKey, _ = ioutil.ReadFile(common.SpawnHomeDir + "authkey")
	if len(common.SecretKey) != 32 {
		common.SecretKey = make([]byte, 32)
		rand.Read(common.SecretKey)
		ioutil.WriteFile(common.SpawnHomeDir+"authkey", common.SecretKey, 0600)
	}
	common.PublicKey = btc.Encodeb58(btc.PublicFromPrivate(common.SecretKey, true))
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
	common.BlockChain = chain.NewChainExt(common.SpawnHomeDir, common.GenesisBlock, common.FLAG.Rescan, ext,
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
