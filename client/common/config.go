// Package common -
package common

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ParallelCoinTeam/duod"
	"github.com/ParallelCoinTeam/duod/lib/L"
	"github.com/ParallelCoinTeam/duod/lib/others/sys"
	"github.com/ParallelCoinTeam/duod/lib/utxo"
)

var (
	// FLAG - Command line only options
	FLAG struct {
		Rescan        bool
		VolatileUTXO  bool
		UndoBlocks    uint
		TrustAll      bool
		UnbanAllPeers bool
		NoWallet      bool
		Log           bool
		SaveConfig    bool
	}

	// CFG - Options that can come from either command line or common file
	CFG struct {
		Testnet          bool
		ConnectOnly      string
		Datadir          string
		TextUIEnabled    bool
		UserAgent        string
		UTXOSaveSec      uint
		LastTrustedBlock string

		WebUI struct {
			Interface   string
			AllowedIP   string // comma separated
			ShowBlocks  uint32
			AddrListLen uint32 // size of address list in MakeTx tab popups
			Title       string
			PayCmdName  string
			ServerMode  bool
		}
		RPC struct {
			Enabled  bool
			Username string
			Password string
			TCPPort  uint32
		}
		Net struct {
			ListenTCP      bool
			TCPPort        uint16
			MaxOutCons     uint32
			MaxInCons      uint32
			MaxUpKBps      uint
			MaxDownKBps    uint
			MaxBlockAtOnce uint32
			MinSegwitCons  uint32
		}
		TXPool struct {
			Enabled        bool // Global on/off swicth
			AllowMemInputs bool
			FeePerByte     float64
			MaxTxSize      uint32
			MaxSizeMB      uint
			MaxRejectMB    uint
			MaxRejectCnt   uint
			SaveOnDisk     bool
			Debug          bool
		}
		TXRoute struct {
			Enabled    bool // Global on/off swicth
			FeePerByte float64
			MaxTxSize  uint32
			MemInputs  bool
		}
		Memory struct {
			GCPercTrshold int
			UseGoHeap     bool // Do not use OS memory functions for UTXO records
			MaxCachedBlks uint
			FreeAtStart   bool // Free all possible memory after initial loading of block chain
			CacheOnDisk   bool
			MaxDataFileMB uint   // 0 for unlimited size
			DataFilesKeep uint32 // 0 for all
		}
		AllBalances struct {
			MinValue  uint64 // Do not keep balance records for values lower than this
			UseMapCnt int
			AutoLoad  bool
		}
		Stat struct {
			HashrateHrs uint
			MiningHrs   uint
			FeesBlks    uint
			BSizeBlks   uint
		}
		DropPeers struct {
			DropEachMinutes uint // zero for never
			BlckExpireHours uint // zero for never
			PingPeriodSec   uint // zero to not ping
		}
	}

	mutexCfg sync.Mutex
)

type oneAllowedAddr struct {
	Addr, Mask uint32
}

// WebUIAllowed -
var WebUIAllowed []oneAllowedAddr

// InitConfig -
func InitConfig() {
	// Fill in default values
	CFG.Net.ListenTCP = true
	CFG.Net.MaxOutCons = 9
	CFG.Net.MaxInCons = 10
	CFG.Net.MaxBlockAtOnce = 3
	CFG.Net.MinSegwitCons = 4

	CFG.TextUIEnabled = true

	CFG.WebUI.Interface = "127.0.0.1:11111"
	CFG.WebUI.AllowedIP = "127.0.0.1"
	CFG.WebUI.ShowBlocks = 144
	CFG.WebUI.AddrListLen = 15
	CFG.WebUI.Title = "Duod"
	CFG.WebUI.PayCmdName = "payCmd.txt"

	CFG.RPC.Username = "Duodrpc"
	CFG.RPC.Password = "Duodpwd"

	CFG.TXPool.Enabled = true
	CFG.TXPool.AllowMemInputs = true
	CFG.TXPool.FeePerByte = 1.0
	CFG.TXPool.MaxTxSize = 100e3
	CFG.TXPool.MaxSizeMB = 100
	CFG.TXPool.MaxRejectMB = 25
	CFG.TXPool.MaxRejectCnt = 5000
	CFG.TXPool.SaveOnDisk = true

	CFG.TXRoute.Enabled = true
	CFG.TXRoute.FeePerByte = 0.0
	CFG.TXRoute.MaxTxSize = 100e3

	CFG.Memory.GCPercTrshold = 30 // 30% (To save mem)
	CFG.Memory.MaxCachedBlks = 200
	CFG.Memory.CacheOnDisk = true
	CFG.Memory.MaxDataFileMB = 1000 // max 1GB per single data file

	CFG.Stat.HashrateHrs = 12
	CFG.Stat.MiningHrs = 24
	CFG.Stat.FeesBlks = 4 * 6   /*last 4 hours*/
	CFG.Stat.BSizeBlks = 12 * 6 /*half a day*/

	CFG.AllBalances.MinValue = 1e12 // 0.001 DUO
	CFG.AllBalances.UseMapCnt = 100
	CFG.AllBalances.AutoLoad = true

	CFG.DropPeers.DropEachMinutes = 5  // minutes
	CFG.DropPeers.BlckExpireHours = 24 // hours
	CFG.DropPeers.PingPeriodSec = 15   // seconds

	CFG.LastTrustedBlock = "000009f0fcbad3aac904d3660cfdcf238bf298cfe73adf1d39d14fc5c740ccc7" // block #0

	cfgfilecontent, e := ioutil.ReadFile(ConfigFile)
	if e == nil && len(cfgfilecontent) > 0 {
		e = json.Unmarshal(cfgfilecontent, &CFG)
		if e != nil {
			L.Error("Error in", ConfigFile, e.Error())
			os.Exit(1)
		}
	} else {
		// Create default config file
		SaveConfig()
		L.Debug("Stored default configuration in", ConfigFile)
	}

	flag.BoolVar(&FLAG.Rescan, "r", false, "Rebuild UTXO database (fixes 'Unknown input TxID' errors)")
	flag.BoolVar(&FLAG.VolatileUTXO, "v", false, "Use UTXO database in volatile mode (speeds up rebuilding)")
	flag.BoolVar(&CFG.Testnet, "t", CFG.Testnet, "Use Testnet3")
	flag.StringVar(&CFG.ConnectOnly, "c", CFG.ConnectOnly, "Connect only to this host and nowhere else")
	flag.BoolVar(&CFG.Net.ListenTCP, "l", CFG.Net.ListenTCP, "Listen for incoming TCP connections (on default port)")
	flag.StringVar(&CFG.Datadir, "d", CFG.Datadir, "Specify Duod's database root folder")
	flag.UintVar(&CFG.Net.MaxUpKBps, "ul", CFG.Net.MaxUpKBps, "Upload limit in KB/s (0 for no limit)")
	flag.UintVar(&CFG.Net.MaxDownKBps, "dl", CFG.Net.MaxDownKBps, "Download limit in KB/s (0 for no limit)")
	flag.StringVar(&CFG.WebUI.Interface, "webui", CFG.WebUI.Interface, "Serve WebUI from the given interface")
	flag.BoolVar(&CFG.TXRoute.Enabled, "txp", CFG.TXPool.Enabled, "Enable Memory Pool")
	flag.BoolVar(&CFG.TXRoute.Enabled, "txr", CFG.TXRoute.Enabled, "Enable Transaction Routing")
	flag.BoolVar(&CFG.TextUIEnabled, "textui", CFG.TextUIEnabled, "Enable processing TextUI commands (from stdin)")
	flag.UintVar(&FLAG.UndoBlocks, "undo", 0, "Undo UTXO with this many blocks and exit")
	flag.BoolVar(&FLAG.TrustAll, "trust", FLAG.TrustAll, "Trust all scripts inside new blocks (for fast syncig)")
	flag.BoolVar(&FLAG.UnbanAllPeers, "unban", FLAG.UnbanAllPeers, "Un-ban all peers in databse, before starting")
	flag.BoolVar(&FLAG.NoWallet, "nowallet", FLAG.NoWallet, "Do not automatically enable the wallet functionality (lower memory usage and faster block processing)")
	flag.BoolVar(&FLAG.Log, "log", FLAG.Log, "Store some runtime information in the log files")
	flag.BoolVar(&FLAG.SaveConfig, "sc", FLAG.SaveConfig, "Save Duod.conf file and exit (use to create default config file)")

	if CFG.Datadir == "" {
		CFG.Datadir = sys.BitcoinHome() + "Duod"
	}

	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	ApplyBalMinVal()

	if !FLAG.NoWallet {
		if FLAG.UndoBlocks != 0 {
			FLAG.NoWallet = true // this will prevent loading of balances, thus speeding up the process
		} else {
			FLAG.NoWallet = !CFG.AllBalances.AutoLoad
		}
	}

	Reset()
}

// DataSubdir -
func DataSubdir() string {
	if CFG.Testnet {
		return "tstnet"
	}
	return "btcnet"
}

// SaveConfig -
func SaveConfig() bool {
	dat, _ := json.MarshalIndent(&CFG, "", "    ")
	if dat == nil {
		return false
	}
	ioutil.WriteFile(ConfigFile, dat, 0660)
	return true

}

// Reset -
// make sure to call it with locked mutexCfg
func Reset() {
	SetUploadLimit(uint64(CFG.Net.MaxUpKBps) << 10)
	SetDownloadLimit(uint64(CFG.Net.MaxDownKBps) << 10)
	debug.SetGCPercent(CFG.Memory.GCPercTrshold)
	if AllBalMinVal() != CFG.AllBalances.MinValue {
		L.Info("In order to apply the new value of AllBalMinVal, restart the node or do 'wallet off' and 'wallet on'")
	}
	DropSlowestEvery = time.Duration(CFG.DropPeers.DropEachMinutes) * time.Minute
	BlockExpireEvery = time.Duration(CFG.DropPeers.BlckExpireHours) * time.Hour
	PingPeerEvery = time.Duration(CFG.DropPeers.PingPeriodSec) * time.Second

	atomic.StoreUint64(&maxMempoolSizeBytes, uint64(CFG.TXPool.MaxSizeMB)*1e6)
	atomic.StoreUint64(&maxRejectedSizeBytes, uint64(CFG.TXPool.MaxRejectMB)*1e6)
	atomic.StoreUint64(&minFeePerKB, uint64(CFG.TXPool.FeePerByte*1000))
	atomic.StoreUint64(&minminFeePerKB, MinFeePerKB())
	atomic.StoreUint64(&routeMinFeePerKB, uint64(CFG.TXRoute.FeePerByte*1000))

	ips := strings.Split(CFG.WebUI.AllowedIP, ",")
	WebUIAllowed = nil
	for i := range ips {
		oaa := str2oaa(ips[i])
		if oaa != nil {
			WebUIAllowed = append(WebUIAllowed, *oaa)
		} else {
			L.Error("ERROR: Incorrect AllowedIP:", ips[i])
		}
	}
	if len(WebUIAllowed) == 0 {
		L.Warn("No IP is currently allowed at WebUI")
	}
	ListenTCP = CFG.Net.ListenTCP

	if CFG.UTXOSaveSec != 0 {
		utxo.UXTOWritingTimeTarget = time.Second * time.Duration(CFG.UTXOSaveSec)
	}

	if CFG.UserAgent != "" {
		UserAgent = CFG.UserAgent
	} else {
		UserAgent = "/Duod:" + Duod.Version + "/"
	}

	if CFG.Memory.MaxDataFileMB != 0 && CFG.Memory.MaxDataFileMB < 8 {
		CFG.Memory.MaxDataFileMB = 8
	}

	MkTempBlocksDir()

	ReloadMiners()

	ApplyLastTrustedBlock()
}

// MkTempBlocksDir -
func MkTempBlocksDir() {
	// no point doing it before DuodHomeDir is set in hostInit()
	if CFG.Memory.CacheOnDisk && DuodHomeDir != "" {
		os.Mkdir(TempBlocksDir(), 0700)
	}
}

// RPCPort -
func RPCPort() (res uint32) {
	mutexCfg.Lock()
	defer mutexCfg.Unlock()

	if CFG.RPC.TCPPort != 0 {
		res = CFG.RPC.TCPPort
		return
	}
	if CFG.Testnet {
		res = 18332
	} else {
		res = 8332
	}
	return
}

// DefaultTCPport -
func DefaultTCPport() (res uint16) {
	mutexCfg.Lock()
	defer mutexCfg.Unlock()

	if CFG.Net.TCPPort != 0 {
		res = CFG.Net.TCPPort
		return
	}
	if CFG.Testnet {
		res = 21047
	} else {
		res = 11047
	}
	return
}

// Converts an IP range to addr/mask
func str2oaa(ip string) (res *oneAllowedAddr) {
	var a, b, c, d, x uint32
	n, _ := fmt.Sscanf(ip, "%d.%d.%d.%d/%d", &a, &b, &c, &d, &x)
	if n < 4 {
		return
	}
	if (a|b|c|d) > 255 || n == 5 && (x < 0 || x > 32) {
		return
	}
	res = new(oneAllowedAddr)
	res.Addr = (a << 24) | (b << 16) | (c << 8) | d
	if n == 4 || x == 32 {
		res.Mask = 0xffffffff
	} else {
		res.Mask = uint32((uint64(1)<<(32-x))-1) ^ 0xffffffff
	}
	res.Addr &= res.Mask
	return
}

// LockCfg -
func LockCfg() {
	mutexCfg.Lock()
}

// UnlockCfg -
func UnlockCfg() {
	mutexCfg.Unlock()
}

// CloseBlockChain -
func CloseBlockChain() {
	if BlockChain != nil {
		L.Debug("Closing BlockChain")
		BlockChain.Close()
		BlockChain = nil
	}
}

// GetDuration -
func GetDuration(addr *time.Duration) (res time.Duration) {
	mutexCfg.Lock()
	res = *addr
	mutexCfg.Unlock()
	return
}

// GetUint64 -
func GetUint64(addr *uint64) (res uint64) {
	mutexCfg.Lock()
	res = *addr
	mutexCfg.Unlock()
	return
}

// GetUint32 -
func GetUint32(addr *uint32) (res uint32) {
	mutexCfg.Lock()
	res = *addr
	mutexCfg.Unlock()
	return
}

// SetUint32 -
func SetUint32(addr *uint32, val uint32) {
	mutexCfg.Lock()
	*addr = val
	mutexCfg.Unlock()
	return
}

// GetBool -
func GetBool(addr *bool) (res bool) {
	mutexCfg.Lock()
	res = *addr
	mutexCfg.Unlock()
	return
}

// SetBool -
func SetBool(addr *bool, val bool) {
	mutexCfg.Lock()
	*addr = val
	mutexCfg.Unlock()
}

// AllBalMinVal -
func AllBalMinVal() uint64 {
	return atomic.LoadUint64(&allBalMinVal)
}

// ApplyBalMinVal -
func ApplyBalMinVal() {
	atomic.StoreUint64(&allBalMinVal, CFG.AllBalances.MinValue)
}

// MinFeePerKB -
func MinFeePerKB() uint64 {
	return atomic.LoadUint64(&minFeePerKB)
}

// SetMinFeePerKB -
func SetMinFeePerKB(val uint64) bool {
	minmin := atomic.LoadUint64(&minminFeePerKB)
	if val < minmin {
		val = minmin
	}
	if val == MinFeePerKB() {
		return false
	}
	atomic.StoreUint64(&minFeePerKB, val)
	return true
}

// RouteMinFeePerKB -
func RouteMinFeePerKB() uint64 {
	return atomic.LoadUint64(&routeMinFeePerKB)
}

// IsListenTCP -
func IsListenTCP() (res bool) {
	mutexCfg.Lock()
	res = CFG.ConnectOnly == "" && ListenTCP
	mutexCfg.Unlock()
	return
}

// MaxMempoolSize -
func MaxMempoolSize() uint64 {
	return atomic.LoadUint64(&maxMempoolSizeBytes)
}

// RejectedTxsLimits -
func RejectedTxsLimits() (size uint64, cnt int) {
	mutexCfg.Lock()
	size = maxRejectedSizeBytes
	cnt = int(CFG.TXPool.MaxRejectCnt)
	mutexCfg.Unlock()
	return
}

// TempBlocksDir -
func TempBlocksDir() string {
	return DuodHomeDir + "tmpblk" + string(os.PathSeparator)
}
