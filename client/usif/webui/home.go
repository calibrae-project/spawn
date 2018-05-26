package webui

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/client/network"
	"github.com/ParallelCoinTeam/duod/client/usif"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/others/peersdb"
	"github.com/ParallelCoinTeam/duod/lib/others/sys"
	"github.com/ParallelCoinTeam/duod/lib/utxo"
)

var (
	mutexHrate sync.Mutex
	lastHrate  float64
	nextHrate  time.Time
)

func pHome(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	// The handler also gets called for /favicon.ico
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	s := loadTemplate("home.html")

	if !common.CFG.WebUI.ServerMode {
		common.LockCfg()
		dat, _ := json.MarshalIndent(&common.CFG, "", "    ")
		common.UnlockCfg()
		s = strings.Replace(s, "{ConfigFile}", strings.Replace(string(dat), ",\"", ", \"", -1), 1)
	}

	s = strings.Replace(s, "<!--PUB_AUTH_KEY-->", common.PublicKey, 1)

	writeHTMLHead(w, r)
	w.Write([]byte(s))
	writeHTMLTail(w)
}

func jsonStatus(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var out struct {
		Height                 uint32
		Hash                   string
		Timestamp              uint32
		Received               int64
		TimeNow                int64
		Diff                   float64
		Median                 uint32
		Version                uint32
		MinValue               uint64
		WalletON               bool
		LastTrustedBlockHeight uint32
		LastHeaderHeight       uint32
		BlockChainSynchronized bool
	}
	common.Last.Mutex.Lock()
	out.Height = common.Last.Block.Height
	out.Hash = common.Last.Block.BlockHash.String()
	out.Timestamp = common.Last.Block.Timestamp()
	out.Received = common.Last.Time.Unix()
	out.TimeNow = time.Now().Unix()
	out.Diff = btc.GetDifficulty(common.Last.Block.Bits())
	out.Median = common.Last.Block.GetMedianTimePast()
	out.Version = common.Last.Block.BlockVersion()
	common.Last.Mutex.Unlock()
	out.MinValue = common.AllBalMinVal()
	out.WalletON = common.GetBool(&common.WalletON)
	out.LastTrustedBlockHeight = common.GetUint32(&common.LastTrustedBlockHeight)
	network.MutexRcv.Lock()
	out.LastHeaderHeight = network.LastCommitedHeader.Height
	network.MutexRcv.Unlock()
	out.BlockChainSynchronized = common.GetBool(&common.BlockChainSynchronized)

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}

func jsonSystem(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var out struct {
		BlocksCached     int
		BlocksToGet      int
		KnownPeers       int
		NodeUptime       uint64
		NetBlockQsize    int
		NetTxQsize       int
		HeapSize         uint64
		HeapSysmem       uint64
		QdbExtramem      int64
		EcdsaVerifyCount uint64
		AverageBlockSize int
		AverageFee       float64
		LastHeaderHeight uint32
		NetworkHashRate  float64
		SavingUTXO       bool
	}

	out.BlocksCached = network.CachedBlocksLen.Get()
	network.MutexRcv.Lock()
	out.BlocksToGet = len(network.BlocksToGet)
	network.MutexRcv.Unlock()
	out.KnownPeers = peersdb.PeerDB.Count()
	out.NodeUptime = uint64(time.Now().Sub(common.StartTime).Seconds())
	out.NetBlockQsize = len(network.NetBlocks)
	out.NetTxQsize = len(network.NetTxs)
	out.HeapSize, out.HeapSysmem = sys.MemUsed()
	out.QdbExtramem = utxo.ExtraMemoryConsumed()
	out.EcdsaVerifyCount = btc.EcdsaVerifyCnt()
	out.AverageBlockSize = common.AverageBlockSize.Get()
	out.AverageFee = common.GetAverageFee()
	network.MutexRcv.Lock()
	out.LastHeaderHeight = network.LastCommitedHeader.Height
	network.MutexRcv.Unlock()

	mutexHrate.Lock()
	if nextHrate.IsZero() || time.Now().After(nextHrate) {
		lastHrate = usif.GetNetworkHashRateNum()
		nextHrate = time.Now().Add(time.Minute)
	}
	out.NetworkHashRate = lastHrate
	mutexHrate.Unlock()

	out.SavingUTXO = common.BlockChain.Unspent.WritingInProgress.Get()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
