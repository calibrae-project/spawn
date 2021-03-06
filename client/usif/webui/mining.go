package webui

import (
	"fmt"
	"sort"
	"time"
	//	"bytes"
	//	"regexp"
	"encoding/binary"
	"encoding/json"
	"net/http"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/btc"
)

type omv struct {
	unknownMiner bool
	cnt          int
	bts          uint64
	fees         uint64
	//ebad_cnt int
	//nya_cnt int
}

type onemiernstat []struct {
	name string
	omv
}

func (x onemiernstat) Len() int {
	return len(x)
}

func (x onemiernstat) Less(i, j int) bool {
	if x[i].cnt == x[j].cnt {
		return x[i].name < x[j].name // Same numbers: sort by name
	}
	return x[i].cnt > x[j].cnt
}

func (x onemiernstat) Swap(i, j int) {
	x[i], x[j] = x[j], x[i]
}

func pMiners(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	writeHTMLHead(w, r)
	w.Write([]byte(loadTemplate("miners.html")))
	writeHTMLTail(w)
}

func jsonBlkVer(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	w.Header()["Content-Type"] = []string{"application/json"}

	common.Last.Mutex.Lock()
	end := common.Last.Block
	common.Last.Mutex.Unlock()

	w.Write([]byte("["))
	if end != nil {
		MaxCount := 2 * 2016 //common.BlockChain.Consensus.Window
		for {
			w.Write([]byte(fmt.Sprint("[", end.Height, ",", binary.LittleEndian.Uint32(end.BlockHeader[0:4]), "]")))
			end = end.Parent
			if end == nil || MaxCount <= 1 {
				break
			}
			MaxCount--
			w.Write([]byte(","))
		}
	}
	w.Write([]byte("]"))
}

func jsonMiners(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	type oneMinerRow struct {
		Unknown               bool
		Name                  string
		Blocks                int
		TotalFees, TotalBytes uint64
		//BUcnt, NYAcnt int
	}

	type theMiningStats struct {
		MiningStatHours  uint
		BlockCount       uint
		FirstBlockTime   int64
		AvgBlocksPerHour float64
		AvgDifficulty    float64
		AvgHashrate      float64
		NextDiffChange   uint32
		Miners           []oneMinerRow
	}

	common.ReloadMiners()

	m := make(map[string]omv, 20)
	var om omv
	cnt := uint(0)
	common.Last.Mutex.Lock()
	end := common.Last.Block
	common.Last.Mutex.Unlock()
	var lastts int64
	var diff float64
	now := time.Now().Unix()

	nextDiffChange := 2016 - end.Height%2016

	//ebAdX := regexp.MustCompile("/EB[0-9]+/AD[0-9]+/")

	for ; end != nil; cnt++ {
		if now-int64(end.Timestamp()) > int64(common.CFG.Stat.MiningHrs)*3600 {
			break
		}
		lastts = int64(end.Timestamp())
		bl, _, e := common.BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			break
		}

		block, e := btc.NewBlock(bl)
		if e != nil {
			break
		}

		cbasetx, _ := btc.NewTx(bl[block.TxOffset:])

		diff += btc.GetDifficulty(end.Bits())
		miner, mid := common.TxMiner(cbasetx)
		om = m[miner]
		om.cnt++
		om.bts += uint64(len(bl))
		om.unknownMiner = (mid == -1)

		// Blocks reward
		var rew uint64
		for o := range cbasetx.TxOut {
			rew += cbasetx.TxOut[o].Value
		}
		fees := rew - btc.GetBlockReward(end.Height)
		if int64(fees) > 0 { // solution for a possibility of a miner not claiming the reward (see block #501726)
			om.fees += fees
		}

		/*if ebAdX.Find(cbasetx.TxIn[0].ScriptSig) != nil {
			om.ebad_cnt++
		}

		if bytes.Index(cbasetx.TxIn[0].ScriptSig, []byte("/NYA/")) != -1 {
			om.nya_cnt++
		}*/

		m[miner] = om

		end = end.Parent
	}

	if cnt == 0 {
		w.Write([]byte("{}"))
		return
	}

	srt := make(onemiernstat, len(m))
	i := 0
	for k, v := range m {
		srt[i].name = k
		srt[i].omv = v
		i++
	}
	sort.Sort(srt)

	var stats theMiningStats

	diff /= float64(cnt)
	bph := float64(cnt) / float64(common.CFG.Stat.MiningHrs)
	hrate := bph / 6 * diff * 7158278.826667

	stats.MiningStatHours = common.CFG.Stat.MiningHrs
	stats.BlockCount = cnt
	stats.FirstBlockTime = lastts
	stats.AvgBlocksPerHour = bph
	stats.AvgDifficulty = diff
	stats.AvgHashrate = hrate
	stats.NextDiffChange = nextDiffChange

	stats.Miners = make([]oneMinerRow, len(srt))
	for i := range srt {
		stats.Miners[i].Unknown = srt[i].unknownMiner
		stats.Miners[i].Name = srt[i].name
		stats.Miners[i].Blocks = srt[i].cnt
		stats.Miners[i].TotalFees = srt[i].fees
		stats.Miners[i].TotalBytes = srt[i].bts
		//stats.Miners[i].BUcnt = srt[i].ebad_cnt
		//stats.Miners[i].NYAcnt = srt[i].nya_cnt
	}

	bx, er := json.Marshal(stats)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}

}
