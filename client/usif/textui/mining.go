package textui

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/btc"
)

func doMining(s string) {
	var totbtc, hrs, segwitCount uint64
	if s != "" {
		hrs, _ = strconv.ParseUint(s, 10, 64)
	}
	if hrs == 0 {
		hrs = uint64(common.CFG.Stat.MiningHrs)
	}
	fmt.Println("Looking back", hrs, "hours...")
	lim := uint32(time.Now().Add(-time.Hour * time.Duration(hrs)).Unix())
	common.Last.Mutex.Lock()
	bte := common.Last.Block
	end := bte
	common.Last.Mutex.Unlock()
	cnt, diff := 0, float64(0)
	totalBlocks, totalBlocksLength := 0, 0

	bip100voting := make(map[string]uint)
	bip100x := regexp.MustCompile("/BV{0,1}[0-9]+[M]{0,1}/")

	ebAdVoting := make(map[string]uint)
	ebAdX := regexp.MustCompile("/EB[0-9]+/AD[0-9]+/")

	for end.Timestamp() >= lim {
		bl, _, e := common.BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			println(cnt, e.Error())
			return
		}
		block, e := btc.NewBlock(bl)
		if e != nil {
			println("btc.NewBlock failed", e.Error())
			return
		}

		bt, _ := btc.NewBlock(bl)
		cbasetx, _ := btc.NewTx(bl[bt.TxOffset:])

		totalBlocks++
		totalBlocksLength += len(bl)
		diff += btc.GetDifficulty(block.Bits())

		if (block.Version() & 0x20000002) == 0x20000002 {
			segwitCount++
		}

		res := bip100x.Find(cbasetx.TxIn[0].ScriptSig)
		if res != nil {
			bip100voting[string(res)]++
			nimer, _ := common.TxMiner(cbasetx)
			fmt.Println("      block", end.Height, "by", nimer, "BIP100 voting", string(res), " total:", bip100voting[string(res)])
		}

		res = ebAdX.Find(cbasetx.TxIn[0].ScriptSig)
		if res != nil {
			ebAdVoting[string(res)]++
		}

		end = end.Parent
	}
	if totalBlocks == 0 {
		fmt.Println("There are no blocks from the last", hrs, "hour(s)")
		return
	}
	diff /= float64(totalBlocks)
	if cnt > 0 {
		fmt.Printf("Projected weekly income : %.0f BTC,  estimated hashrate : %s\n",
			7*24*float64(totbtc)/float64(hrs)/1e8,
			common.HashrateToString(float64(cnt)/float64(6*hrs)*diff*7158278.826667))
	}
	bph := float64(totalBlocks) / float64(hrs)
	fmt.Printf("Total network hashrate : %s @ average diff %.0f  (%.2f bph)\n",
		common.HashrateToString(bph/6*diff*7158278.826667), diff, bph)
	fmt.Printf("%d blocks in %d hours. Average size %.1f KB,  next diff in %d blocks\n",
		totalBlocks, hrs, float64(totalBlocksLength/totalBlocks)/1e3, 2016-bte.Height%2016)

	fmt.Printf("\nSegWit Voting: %d (%.1f%%)\n", segwitCount, float64(segwitCount)*100/float64(totalBlocks))
	fmt.Println()
	fmt.Println("BU Voting")
	for k, v := range ebAdVoting {
		fmt.Printf(" %s \t %d \t %.1f%%\n", k, v, float64(v)*100/float64(totalBlocks))
	}
}

func init() {
	newUI("minerstat m", false, doMining, "Look for the miner ID in recent blocks (optionally specify number of hours)")
}
