// Package common -
package common

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"sync"

	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/L"
)

type oneMinerID struct {
	Name string
	Tag  []byte
}

// MinerIDs -
var MinerIDs []oneMinerID

// TxMiner -
// return miner ID of the given coinbase transaction
func TxMiner(cbtx *btc.Tx) (string, int) {
	txdat := cbtx.Serialize()
	for i, m := range MinerIDs {
		if bytes.Equal(m.Tag, []byte("_p2pool_")) { // P2Pool
			if len(cbtx.TxOut) > 10 &&
				bytes.Equal(cbtx.TxOut[len(cbtx.TxOut)-1].PkScript[:2], []byte{0x6A, 0x28}) {
				return m.Name, i
			}
		} else if bytes.Contains(txdat, m.Tag) {
			return m.Name, i
		}
	}

	for _, txo := range cbtx.TxOut {
		adr := btc.NewAddrFromPkScript(txo.PkScript, Testnet)
		if adr != nil {
			return adr.String(), -1
		}
	}

	return "", -1
}

// ReloadMiners -
func ReloadMiners() {
	d, _ := ioutil.ReadFile("miners.json")
	if d != nil {
		var MinerIDfile [][3]string
		e := json.Unmarshal(d, &MinerIDfile)
		if e != nil {
			L.Error("miners.json", e.Error())
			return
		}
		MinerIDs = nil
		for _, r := range MinerIDfile {
			var rec oneMinerID
			rec.Name = r[0]
			if r[1] != "" {
				rec.Tag = []byte(r[1])
			} else {
				if a, _ := btc.NewAddrFromString(r[2]); a != nil {
					rec.Tag = a.OutScript()
				} else {
					L.Error("Error in miners.json for", r[0])
					continue
				}
			}
			MinerIDs = append(MinerIDs, rec)
		}
	}
}

var (
	// AverageFeeMutex -
	AverageFeeMutex sync.Mutex
	// AverageFeeBytes -
	AverageFeeBytes uint64
	// AverageFeeTotal -
	AverageFeeTotal uint64
	// AverageFeeSPB -
	AverageFeeSPB       float64
	averageFeeLastBlock uint32 = 0xffffffff
	averageFeeLastCount uint   = 0xffffffff
)

// GetAverageFee -
func GetAverageFee() float64 {
	Last.Mutex.Lock()
	end := Last.Block
	Last.Mutex.Unlock()

	LockCfg()
	blocks := CFG.Stat.FeesBlks
	UnlockCfg()
	if blocks <= 0 {
		blocks = 1 // at leats one block
	}

	AverageFeeMutex.Lock()
	defer AverageFeeMutex.Unlock()

	if end.Height == averageFeeLastBlock && averageFeeLastCount == blocks {
		return AverageFeeSPB // we've already calculated for this block
	}

	averageFeeLastBlock = end.Height
	averageFeeLastCount = blocks

	AverageFeeBytes = 0
	AverageFeeTotal = 0

	for blocks > 0 {
		bl, _, e := BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			return 0
		}
		block, e := btc.NewBlock(bl)
		if e != nil {
			return 0
		}

		cbasetx, cbasetxlen := btc.NewTx(bl[block.TxOffset:])
		var feesFromThisBlock int64
		for o := range cbasetx.TxOut {
			feesFromThisBlock += int64(cbasetx.TxOut[o].Value)
		}
		feesFromThisBlock -= int64(btc.GetBlockReward(end.Height))

		if feesFromThisBlock > 0 {
			AverageFeeTotal += uint64(feesFromThisBlock)
		}

		AverageFeeBytes += uint64(len(bl) - block.TxOffset - cbasetxlen) /*do not count block header and conibase tx */

		blocks--
		end = end.Parent
	}
	if AverageFeeBytes == 0 {
		if AverageFeeTotal != 0 {
			panic("Impossible that miner gets a fee with no transactions in the block")
		}
		AverageFeeSPB = 0
	} else {
		AverageFeeSPB = float64(AverageFeeTotal) / float64(AverageFeeBytes)
	}
	return AverageFeeSPB
}
