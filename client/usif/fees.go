package usif

import (
	"bufio"
	"encoding/gob"
	"os"
	"sync"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/L"
)

const (
	// BlkFeesFileName -
	BlkFeesFileName = "blkfees.gob"
)

var (
	// BlockFeesMutex -
	BlockFeesMutex sync.Mutex
	// BlockFees -
	BlockFees = make(map[uint32][][3]uint64) // [0]=Weight  [1]-Fee  [2]-Group
	// BlockFeesDirty -
	BlockFeesDirty bool // it true, clean up old data
)

// ProcessBlockFees -
func ProcessBlockFees(height uint32, bl *btc.Block) {
	if len(bl.Txs) < 2 {
		return
	}

	txs := make(map[[32]byte]int, len(bl.Txs)) // group_id -> transaciton_idx
	txs[bl.Txs[0].Hash.Hash] = 0

	fees := make([][3]uint64, len(bl.Txs)-1)

	for i := 1; i < len(bl.Txs); i++ {
		txs[bl.Txs[i].Hash.Hash] = i
		fees[i-1][0] = uint64(3*bl.Txs[i].NoWitSize + bl.Txs[i].Size)
		fees[i-1][1] = uint64(bl.Txs[i].Fee)
		fees[i-1][2] = uint64(i)
	}

	for i := 1; i < len(bl.Txs); i++ {
		for _, inp := range bl.Txs[i].TxIn {
			if paretidx, yes := txs[inp.Input.Hash]; yes {
				if fees[paretidx-1][2] < fees[i-1][2] { // only update it for a lower index
					fees[i-1][2] = fees[paretidx-1][2]
				}
			}
		}
	}

	BlockFeesMutex.Lock()
	BlockFees[height] = fees
	BlockFeesDirty = true
	BlockFeesMutex.Unlock()
}

// ExpireBlockFees -
func ExpireBlockFees() {
	var height uint32
	common.Last.Lock()
	height = common.Last.Block.Height
	common.Last.Unlock()

	if height <= 144 {
		return
	}
	height -= 144

	BlockFeesMutex.Lock()
	if BlockFeesDirty {
		for k := range BlockFees {
			if k < height {
				delete(BlockFees, k)
			}
		}
		BlockFeesDirty = false
	}
	BlockFeesMutex.Unlock()
}

// SaveBlockFees -
func SaveBlockFees() {
	f, er := os.Create(common.DuodHomeDir + BlkFeesFileName)
	if er != nil {
		L.Error("SaveBlockFees:", er.Error())
		return
	}

	ExpireBlockFees()
	buf := bufio.NewWriter(f)
	er = gob.NewEncoder(buf).Encode(BlockFees)

	if er != nil {
		L.Error("SaveBlockFees:", er.Error())
	}

	buf.Flush()
	f.Close()

}

// LoadBlockFees -
func LoadBlockFees() {
	f, er := os.Open(common.DuodHomeDir + BlkFeesFileName)
	if er != nil {
		println("LoadBlockFees:", er.Error())
		return
	}

	buf := bufio.NewReader(f)
	er = gob.NewDecoder(buf).Decode(&BlockFees)
	if er != nil {
		println("LoadBlockFees:", er.Error())
	}

	f.Close()
}
