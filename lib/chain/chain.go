package chain

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"sync"

	"github.com/calibrae-project/spawn/lib/btc"
	"github.com/calibrae-project/spawn/lib/utxo"
)

// AbortNow -
var AbortNow bool // set it to true to abort any activity

// Chain -
type Chain struct {
	Blocks  *BlockDB        // blockchain.dat and blockchain.idx
	Unspent *utxo.UnspentDB // unspent folder

	BlockTreeRoot   *BlockTreeNode
	blockTreeEnd    *BlockTreeNode
	blockTreeAccess sync.Mutex
	Genesis         *btc.Uint256

	BlockIndexAccess sync.Mutex
	BlockIndex       map[[btc.Uint256IdxLen]byte]*BlockTreeNode

	CB NewChanOpts // callbacks used by Unspent database

	Consensus struct {
		Window, EnforceUpgrade, RejectBlock uint
		MaxPOWBits                          uint32
		MaxPOWValue                         *big.Int
		GensisTimestamp                     uint32
		EnforceCSV                          uint32 // if non zero CVS verifications will be enforced from this block onwards
		EnforceSegwit                       uint32 // if non zero CVS verifications will be enforced from this block onwards
		BIP9Threshold                       uint32 // It is not really used at this moment, but maybe one day...
		BIP34Height                         uint32
		BIP65Height                         uint32
		BIP66Height                         uint32
		BIP91Height                         uint32
		S2XHeight                           uint32
	}
}

// NewChanOpts -
type NewChanOpts struct {
	UTXOVolatileMode bool
	UndoBlocks       uint // undo this many blocks when opening the chain
	UTXOCallbacks    utxo.CallbackFunctions
	BlockMinedCB     func(*btc.Block) // used to remove mined txs from memory pool
}

// NewChainExt - This is the very first function one should call in order to use this package
func NewChainExt(dbrootdir string, genesis *btc.Uint256, rescan bool, opts *NewChanOpts, bdbopts *BlockDBOpts) (ch *Chain) {
	ch = new(Chain)
	ch.Genesis = genesis

	if opts == nil {
		opts = &NewChanOpts{}
	}

	ch.CB = *opts

	ch.Consensus.GensisTimestamp = 1231006505
	ch.Consensus.MaxPOWBits = 0x1d00ffff
	ch.Consensus.MaxPOWValue, _ = new(big.Int).SetString("00000000FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", 16)
	if ch.testnet() {
		ch.Consensus.BIP34Height = 21111
		ch.Consensus.BIP65Height = 581885
		ch.Consensus.BIP66Height = 330776
		ch.Consensus.EnforceCSV = 770112
		ch.Consensus.EnforceSegwit = 834624
		ch.Consensus.BIP9Threshold = 1512
	} else {
		ch.Consensus.BIP34Height = 227931
		ch.Consensus.BIP65Height = 388381
		ch.Consensus.BIP66Height = 363725
		ch.Consensus.EnforceCSV = 419328
		ch.Consensus.EnforceSegwit = 481824 // https://www.reddit.com/r/Bitcoin/comments/6okd1n/bip91_lock_in_is_guaranteed_as_of_block_476768/
		ch.Consensus.BIP91Height = 477120
		ch.Consensus.BIP9Threshold = 1916
	}

	ch.Blocks = NewBlockDBExt(dbrootdir, bdbopts)

	ch.Unspent = utxo.NewUnspentDb(&utxo.NewUnspentOpts{
		Dir: dbrootdir, Rescan: rescan, VolatimeMode: opts.UTXOVolatileMode,
		CB: opts.UTXOCallbacks, AbortNow: &AbortNow})

	if AbortNow {
		return
	}

	ch.loadBlockIndex()
	if AbortNow {
		return
	}

	if rescan {
		ch.SetLast(ch.BlockTreeRoot)
	}

	if AbortNow {
		return
	}

	if opts.UndoBlocks > 0 {
		fmt.Println("Undo", opts.UndoBlocks, "block(s) and exit...")
		for opts.UndoBlocks > 0 {
			ch.UndoLastBlock()
			opts.UndoBlocks--
		}
		return
	}

	// And now re-apply the blocks which you have just reverted :)
	end, _ := ch.BlockTreeRoot.FindFarthestNode()
	if end.Height > ch.LastBlock().Height {
		ch.ParseUntilBlock(end)
	} else {
		ch.Unspent.LastBlockHeight = end.Height
	}

	return
}

// RebuildGenesisHeader - Calculate an imaginary header of the genesis block (for Timestamp() and Bits() functions from chain_tree.go)
func (ch *Chain) RebuildGenesisHeader() {
	binary.LittleEndian.PutUint32(ch.BlockTreeRoot.BlockHeader[0:4], 1) // Version
	// [4:36] - prev_block
	// [36:68] - merkle_root
	binary.LittleEndian.PutUint32(ch.BlockTreeRoot.BlockHeader[68:72], ch.Consensus.GensisTimestamp) // Timestamp
	binary.LittleEndian.PutUint32(ch.BlockTreeRoot.BlockHeader[72:76], ch.Consensus.MaxPOWBits)      // Bits
	// [76:80] - nonce
}

// Idle - Call this function periodically (i.e. each second)
// when your client is idle, to defragment databases.
func (ch *Chain) Idle() bool {
	ch.Blocks.Idle()
	return ch.Unspent.Idle()
}

// Stats - Return blockchain stats in one string.
func (ch *Chain) Stats() (s string) {
	last := ch.LastBlock()
	ch.BlockIndexAccess.Lock()
	s = fmt.Sprintf("CHAIN: blocks:%d  Height:%d  MedianTime:%d\n",
		len(ch.BlockIndex), last.Height, last.GetMedianTimePast())
	ch.BlockIndexAccess.Unlock()
	s += ch.Blocks.GetStats()
	s += ch.Unspent.GetStats()
	return
}

// Close the databases.
func (ch *Chain) Close() {
	ch.Blocks.Close()
	ch.Unspent.Close()
}

// Returns true if we are on Testnet3 chain
func (ch *Chain) testnet() bool {
	return ch.Genesis.Hash[0] == 0x43 // it's simple, but works
}

// MaxBlockWeight - For SegWit2X
func (ch *Chain) MaxBlockWeight(height uint32) uint {
	if ch.Consensus.S2XHeight != 0 && height >= ch.Consensus.S2XHeight {
		return 2 * btc.MaxBlockWeight
	}
	return btc.MaxBlockWeight
}

// MaxBlockSigopsCost - For SegWit2X
func (ch *Chain) MaxBlockSigopsCost(height uint32) uint32 {
	if ch.Consensus.S2XHeight != 0 && height >= ch.Consensus.S2XHeight {
		return 2 * btc.MaxBlockSigOpsCost
	}
	return btc.MaxBlockSigOpsCost
}

// LastBlock -
func (ch *Chain) LastBlock() (res *BlockTreeNode) {
	ch.blockTreeAccess.Lock()
	res = ch.blockTreeEnd
	ch.blockTreeAccess.Unlock()
	return
}

// SetLast -
func (ch *Chain) SetLast(val *BlockTreeNode) {
	ch.blockTreeAccess.Lock()
	ch.blockTreeEnd = val
	ch.blockTreeAccess.Unlock()
	return
}
