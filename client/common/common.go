// Package common -
package common

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/chain"
	"github.com/ParallelCoinTeam/duod/lib/others/sys"
	"github.com/ParallelCoinTeam/duod/lib/others/utils"
)

const (
	// ConfigFile -
	ConfigFile = "duod.conf"
	// Version -
	Version = uint32(80000) // 70015)
	// Services -
	Services = uint64(1) // 0x00000009)
)

var (
	// LogBuffer -
	LogBuffer = new(bytes.Buffer)
	// Log -
	Log = log.New(LogBuffer, "", 0)
	// BlockChain -
	BlockChain *chain.Chain
	// GenesisBlock -
	GenesisBlock *btc.Uint256
	// Magic -
	Magic [4]byte
	// Testnet -
	Testnet bool
	// Last -
	Last TheLastBlock
	// DuodHomeDir -
	DuodHomeDir string
	// StartTime -
	StartTime time.Time
	// MaxPeersNeeded -
	MaxPeersNeeded int
	// CounterMutex -
	CounterMutex sync.Mutex
	// Counter -
	Counter = make(map[string]uint64)

	busyLine int32
	// NetworkClosed -
	NetworkClosed sys.SyncBool
	// AverageBlockSize -
	AverageBlockSize sys.SyncInt

	allBalMinVal uint64
	// DropSlowestEvery -
	DropSlowestEvery time.Duration
	// BlockExpireEvery -
	BlockExpireEvery time.Duration
	// PingPeerEvery -
	PingPeerEvery time.Duration
	// UserAgent -
	UserAgent string
	// ListenTCP -
	ListenTCP bool

	minFeePerKB, routeMinFeePerKB, minminFeePerKB uint64
	maxMempoolSizeBytes, maxRejectedSizeBytes     uint64
	// KillChan -
	KillChan = make(chan os.Signal)
	// SecretKey - 32 bytes for secret key
	SecretKey []byte
	// PublicKey - 32 bytes for public key
	PublicKey string
	// WalletON -
	WalletON bool
	// WalletProgress - 0 to 1000 range
	WalletProgress uint32
	// WalletOnIn -
	WalletOnIn uint32
	// BlockChainSynchronized -
	BlockChainSynchronized bool
	lastTrustedBlock       *btc.Uint256
	// LastTrustedBlockHeight -
	LastTrustedBlockHeight uint32
)

// TheLastBlock -
type TheLastBlock struct {
	sync.Mutex // use it for writing and reading from non-chain thread
	Block      *chain.BlockTreeNode
	time.Time
}

// BlockHeight -
func (b *TheLastBlock) BlockHeight() (res uint32) {
	b.Mutex.Lock()
	res = b.Block.Height
	b.Mutex.Unlock()
	return
}

// CountSafe -
func CountSafe(k string) {
	CounterMutex.Lock()
	Counter[k]++
	CounterMutex.Unlock()
}

// CountSafeAdd -
func CountSafeAdd(k string, val uint64) {
	CounterMutex.Lock()
	Counter[k] += val
	CounterMutex.Unlock()
}

// Busy -
func Busy() {
	var line int
	_, _, line, _ = runtime.Caller(1)
	atomic.StoreInt32(&busyLine, int32(line))
}

// BusyIn -
func BusyIn() int {
	return int(atomic.LoadInt32(&busyLine))
}

// BytesToString -
func BytesToString(val uint64) string {
	if val < 1e6 {
		return fmt.Sprintf("%.1f KB", float64(val)/1e3)
	} else if val < 1e9 {
		return fmt.Sprintf("%.2f MB", float64(val)/1e6)
	}
	return fmt.Sprintf("%.2f GB", float64(val)/1e9)
}

// NumberToString -
func NumberToString(num float64) string {
	if num > 1e24 {
		return fmt.Sprintf("%.2f Y", num/1e24)
	}
	if num > 1e21 {
		return fmt.Sprintf("%.2f Z", num/1e21)
	}
	if num > 1e18 {
		return fmt.Sprintf("%.2f E", num/1e18)
	}
	if num > 1e15 {
		return fmt.Sprintf("%.2f P", num/1e15)
	}
	if num > 1e12 {
		return fmt.Sprintf("%.2f T", num/1e12)
	}
	if num > 1e9 {
		return fmt.Sprintf("%.2f G", num/1e9)
	}
	if num > 1e6 {
		return fmt.Sprintf("%.2f M", num/1e6)
	}
	if num > 1e3 {
		return fmt.Sprintf("%.2f K", num/1e3)
	}
	return fmt.Sprintf("%.2f", num)
}

// HashrateToString -
func HashrateToString(hr float64) string {
	return NumberToString(hr) + "H/s"
}

// RecalcAverageBlockSize -
// Calculates average blocks size over the last "CFG.Stat.BSizeBlks" blocks
// Only call from blockchain thread.
func RecalcAverageBlockSize() {
	n := BlockChain.LastBlock()
	var sum, cnt uint
	for maxcnt := CFG.Stat.BSizeBlks; maxcnt > 0 && n != nil; maxcnt-- {
		sum += uint(n.BlockSize)
		cnt++
		n = n.Parent
	}
	if sum > 0 && cnt > 0 {
		AverageBlockSize.Store(int(sum / cnt))
	} else {
		AverageBlockSize.Store(204)
	}
}

// GetRawTx -
func GetRawTx(BlockHeight uint32, txid *btc.Uint256) (data []byte, er error) {
	data, er = BlockChain.GetRawTx(BlockHeight, txid)
	if er != nil {
		if Testnet {
			data = utils.GetTestnetTxFromWeb(txid)
		} else {
			data = utils.GetTxFromWeb(txid)
		}
		if data != nil {
			er = nil
		} else {
			er = errors.New("GetRawTx and GetTxFromWeb failed for " + txid.String())
		}
	}
	return
}

// WalletPendingTick -
func WalletPendingTick() (res bool) {
	mutexCfg.Lock()
	if WalletOnIn > 0 && BlockChainSynchronized {
		WalletOnIn--
		res = WalletOnIn == 0
	}
	mutexCfg.Unlock()
	return
}

// ApplyLastTrustedBlock -
// Make sure to call it with mutexCfg locked
func ApplyLastTrustedBlock() {
	hash := btc.NewUint256FromString(CFG.LastTrustedBlock)
	lastTrustedBlock = hash
	LastTrustedBlockHeight = 0

	if BlockChain != nil {
		BlockChain.BlockIndexAccess.Lock()
		node := BlockChain.BlockIndex[hash.BIdx()]
		BlockChain.BlockIndexAccess.Unlock()
		if node != nil {
			LastTrustedBlockHeight = node.Height
			for node != nil {
				node.Trusted = true
				node = node.Parent
			}
		}
	}
}

// LastTrustedBlockMatch -
func LastTrustedBlockMatch(h *btc.Uint256) (res bool) {
	mutexCfg.Lock()
	res = lastTrustedBlock != nil && lastTrustedBlock.Equal(h)
	mutexCfg.Unlock()
	return
}

// AcceptTx -
func AcceptTx() (res bool) {
	mutexCfg.Lock()
	res = CFG.TXPool.Enabled && BlockChainSynchronized
	mutexCfg.Unlock()
	return
}
