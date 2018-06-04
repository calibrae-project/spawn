// This tool can import blockchain database from satoshi client to Duod
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/chain"
	"github.com/ParallelCoinTeam/duod/lib/others/blockdb"
	"github.com/ParallelCoinTeam/duod/lib/others/sys"
)

// Trust - Set this to false if you want to re-check all scripts
const Trust = true

var (
	// Magic -
	Magic [4]byte
	// DuodHomeDir -
	DuodHomeDir string
	// BTCRootDir -
	BTCRootDir string
	// GenesisBlock -
	GenesisBlock         *btc.Uint256
	prevEcdsaVerifyCount uint64
)

func stat(totnsec, pernsec int64, totbytes, perbytes uint64, height uint32) {
	totmbs := float64(totbytes) / (1024 * 1024)
	perkbs := float64(perbytes) / (1024)
	var x string
	cn := btc.EcdsaVerifyCnt() - prevEcdsaVerifyCount
	if cn > 0 {
		x = fmt.Sprintf("|  %d -> %d us/ecdsa", cn, uint64(pernsec)/cn/1e3)
		prevEcdsaVerifyCount += cn
	}
	fmt.Printf("%.1fMB of data processed. We are at height %d. Processing speed %.3fMB/sec, recent: %.1fKB/s %s\n",
		totmbs, height, totmbs/(float64(totnsec)/1e9), perkbs/(float64(pernsec)/1e9), x)
}

func importBlockchain(dir string) {
	BlockDatabase := blockdb.NewBlockDB(dir, Magic)
	chain := chain.NewChainExt(DuodHomeDir, GenesisBlock, false, nil, nil)

	var bl *btc.Block
	var er error
	var dat []byte
	var totbytes, perbytes uint64

	fmt.Println("Be patient while importing Satoshi's database... ")
	start := time.Now().UnixNano()
	prv := start
	for {
		now := time.Now().UnixNano()
		if now-prv >= 10e9 {
			stat(now-start, now-prv, totbytes, perbytes, chain.LastBlock().Height)
			prv = now // show progress each 10 seconds
			perbytes = 0
		}

		dat, er = BlockDatabase.FetchNextBlock()
		if dat == nil || er != nil {
			println("END of DB file")
			break
		}

		bl, er = btc.NewBlock(dat[:])
		if er != nil {
			println("Block inconsistent:", er.Error())
			break
		}

		bl.Trusted = Trust

		er, _, _ = chain.CheckBlock(bl)

		if er != nil {
			if er.Error() != "Genesis" {
				println("CheckBlock failed:", er.Error())
				os.Exit(1) // Such a thing should not happen, so let's better abort here.
			}
			continue
		}

		er = chain.AcceptBlock(bl)
		if er != nil {
			println("AcceptBlock failed:", er.Error())
			os.Exit(1) // Such a thing should not happen, so let's better abort here.
		}

		totbytes += uint64(len(bl.Raw))
		perbytes += uint64(len(bl.Raw))
	}

	stop := time.Now().UnixNano()
	stat(stop-start, stop-prv, totbytes, perbytes, chain.LastBlock().Height)

	fmt.Println("Satoshi's database import finished in", (stop-start)/1e9, "seconds")

	fmt.Println("Now saving the new database...")
	chain.Close()
	fmt.Println("Database saved. No more imports should be needed.")
}

// RemoveLastSlash -
func RemoveLastSlash(p string) string {
	if len(p) > 0 && os.IsPathSeparator(p[len(p)-1]) {
		return p[:len(p)-1]
	}
	return p
}

func exists(fn string) bool {
	_, e := os.Lstat(fn)
	return e == nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Specify at least one parameter - a path to the blk0000?.dat files.")
		fmt.Println("By default it should be:", sys.BitcoinHome()+"blocks")
		fmt.Println()
		fmt.Println("If you specify a second parameter, that's where output data will be stored.")
		fmt.Println("Otherwise the output data will go to Duod's default data folder.")
		return
	}

	BTCRootDir = RemoveLastSlash(os.Args[1])
	fn := BTCRootDir + string(os.PathSeparator) + "blk00000.dat"
	fmt.Println("Looking for file", fn, "...")
	f, e := os.Open(fn)
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}
	_, e = f.Read(Magic[:])
	f.Close()
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}

	if len(os.Args) > 2 {
		DuodHomeDir = RemoveLastSlash(os.Args[2]) + string(os.PathSeparator)
	} else {
		DuodHomeDir = sys.BitcoinHome() + "Duod" + string(os.PathSeparator)
	}

	if Magic == [4]byte{0x0B, 0x11, 0x09, 0x07} {
		// testnet3
		fmt.Println("There are Testnet3 blocks")
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		DuodHomeDir += "tstnet" + string(os.PathSeparator)
	} else if Magic == [4]byte{0xF9, 0xBE, 0xB4, 0xD9} {
		fmt.Println("There are valid Bitcoin blocks")
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		DuodHomeDir += "btcnet" + string(os.PathSeparator)
	} else {
		println("blk00000.dat has an unexpected magic")
		os.Exit(1)
	}

	fmt.Println("Importing blockchain data into", DuodHomeDir, "...")

	if exists(DuodHomeDir+"blockchain.dat") ||
		exists(DuodHomeDir+"blockchain.idx") ||
		exists(DuodHomeDir+"unspent") {
		println("Destination folder contains some database files.")
		println("Either move them somewhere else or delete manually.")
		println("None of the following files/folders must exist before you proceed:")
		println(" *", DuodHomeDir+"blockchain.dat")
		println(" *", DuodHomeDir+"blockchain.idx")
		println(" *", DuodHomeDir+"unspent")
		os.Exit(1)
	}

	importBlockchain(BTCRootDir)
}
