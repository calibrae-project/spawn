// Package network -
package network

import (
	"sync"
	"time"

	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/chain"
	"github.com/ParallelCoinTeam/duod/lib/others/sys"
)

// OneReceivedBlock -
type OneReceivedBlock struct {
	TmStart        time.Time // when we receioved message letting us about this block
	TmPreproc      time.Time // when we added this block to BlocksToGet
	TmDownload     time.Time // when we finished dowloading of this block
	TmQueue        time.Time // when we started comitting this block
	TmAccepted     time.Time // when the block was commited to blockchain
	Cnt            uint
	TxMissing      int
	FromConID      uint32
	NonWitnessSize int
	DoInvs         bool
}

// BlockRcvd -
type BlockRcvd struct {
	Conn *OneConnection
	*btc.Block
	*chain.BlockTreeNode
	*OneReceivedBlock
	*btc.BlockExtraInfo
}

// TxRcvd -
type TxRcvd struct {
	conn *OneConnection
	*btc.Tx
	trusted, local bool
}

// OneBlockToGet -
type OneBlockToGet struct {
	Started time.Time
	*btc.Block
	*chain.BlockTreeNode
	InProgress uint
	TmPreproc  time.Time // how long it took to start downloading this block
	SendInvs   bool
}

var (
	// ReceivedBlocks -
	ReceivedBlocks = make(map[BIDX]*OneReceivedBlock, 400e3)
	// BlocksToGet -
	BlocksToGet = make(map[BIDX]*OneBlockToGet)
	// IndexToBlocksToGet -
	IndexToBlocksToGet = make(map[uint32][]BIDX)
	// LowestIndexToBlocksToGet -
	LowestIndexToBlocksToGet uint32
	// LastCommitedHeader -
	LastCommitedHeader *chain.BlockTreeNode
	// MutexRcv -
	MutexRcv sync.Mutex
	// NetBlocks -
	NetBlocks = make(chan *BlockRcvd, MaxBlocksForwardCount+10)
	// NetTxs -
	NetTxs = make(chan *TxRcvd, 2000)
	// CachedBlocks -
	CachedBlocks []*BlockRcvd
	// CachedBlocksLen -
	CachedBlocksLen sys.SyncInt
	// DiscardedBlocks -
	DiscardedBlocks = make(map[BIDX]bool)
)

// AddB2G -
func AddB2G(b2g *OneBlockToGet) {
	bidx := b2g.Block.Hash.BIdx()
	BlocksToGet[bidx] = b2g
	bh := b2g.BlockTreeNode.Height
	IndexToBlocksToGet[bh] = append(IndexToBlocksToGet[bh], bidx)
	if LowestIndexToBlocksToGet == 0 || bh < LowestIndexToBlocksToGet {
		LowestIndexToBlocksToGet = bh
	}

	/* TODO: this was causing deadlock. Removing it for now as maybe it is not even needed.
	// Trigger each connection to as the peer for block data
	MutexNet.Lock()
	for _, v := range OpenCons {
		v.MutexSetBool(&v.X.GetBlocksDataNow, true)
	}
	MutexNet.Unlock()
	*/
}

// DelB2G -
func DelB2G(idx BIDX) {
	b2g := BlocksToGet[idx]
	if b2g == nil {
		println("DelB2G - not found")
		return
	}

	bh := b2g.BlockTreeNode.Height
	iii := IndexToBlocksToGet[bh]
	if len(iii) > 1 {
		var n []BIDX
		for _, cidx := range iii {
			if cidx != idx {
				n = append(n, cidx)
			}
		}
		if len(n)+1 != len(iii) {
			println("DelB2G - index not found")
		}
		IndexToBlocksToGet[bh] = n
	} else {
		if iii[0] != idx {
			println("DelB2G - index not matching")
		}
		delete(IndexToBlocksToGet, bh)
		if bh == LowestIndexToBlocksToGet {
			if len(IndexToBlocksToGet) > 0 {
				for LowestIndexToBlocksToGet++; ; LowestIndexToBlocksToGet++ {
					if _, ok := IndexToBlocksToGet[LowestIndexToBlocksToGet]; ok {
						break
					}
				}
			} else {
				LowestIndexToBlocksToGet = 0
			}
		}
	}

	delete(BlocksToGet, idx)
}
