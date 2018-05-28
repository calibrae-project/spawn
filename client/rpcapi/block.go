package rpcapi

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/client/network"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/logg"
)

// BlockSubmitted -
type BlockSubmitted struct {
	*btc.Block
	Error string
	Done  sync.WaitGroup
}

// RPCBlocks -
var RPCBlocks = make(chan *BlockSubmitted, 1)

// SubmitBlock -
func SubmitBlock(cmd *RPCCommand, resp *RPCResponse, b []byte) {
	var bd []byte
	var er error

	switch uu := cmd.Params.(type) {
	case []interface{}:
		if len(uu) < 1 {
			resp.Error = RPCError{Code: -1, Message: "empty params array"}
			return
		}
		str := uu[0].(string)
		if str[0] == '@' {
			/*
				Duod special case: if the string starts with @, it's a name of the file with block's binary data
					curl --user Duodrpc:Duodpwd --data-binary \
						'{"jsonrpc": "1.0", "id":"curltest", "method": "submitblock", "params": \
							["@450529_000000000000000000cf208f521de0424677f7a87f2f278a1042f38d159565f5.bin"] }' \
						-H 'content-type: text/plain;' http://127.0.0.1:8332/
			*/
			logg.Debug("jade z koksem", str[1:])
			bd, er = ioutil.ReadFile(str[1:])
		} else {
			bd, er = hex.DecodeString(str)
		}
		if er != nil {
			resp.Error = RPCError{Code: -3, Message: er.Error()}
			return
		}

	default:
		resp.Error = RPCError{Code: -2, Message: "incorrect params type"}
		return
	}

	bs := new(BlockSubmitted)

	bs.Block, er = btc.NewBlock(bd)
	if er != nil {
		resp.Error = RPCError{Code: -4, Message: er.Error()}
		return
	}

	network.MutexRcv.Lock()
	network.ReceivedBlocks[bs.Block.Hash.BIdx()] = &network.OneReceivedBlock{TmStart: time.Now()}
	network.MutexRcv.Unlock()

	logg.Debug("new block", bs.Block.Hash.String(), "len", len(bd), "- submitting...")
	bs.Done.Add(1)
	RPCBlocks <- bs
	bs.Done.Wait()
	if bs.Error != "" {
		//resp.Error = RPCError{Code: -10, Message: bs.Error}
		idx := strings.Index(bs.Error, "- RPC_Result:")
		if idx == -1 {
			resp.Result = "inconclusive"
		} else {
			resp.Result = bs.Error[idx+13:]
		}
		logg.Debug("submiting block error:", bs.Error)
		logg.Debug("submiting block result:", resp.Result.(string))

		logg.Debug("time_now:", time.Now().Unix())
		logg.Debug("  cur_block_ts:", bs.Block.BlockTime())
		logg.Debug("  last_given_now:", lastGivenTime)
		logg.Debug("  last_given_min:", lastGivenMinTime)
		common.Last.Mutex.Lock()
		logg.Debug("  prev_block_ts:", common.Last.Block.Timestamp())
		common.Last.Mutex.Unlock()

		return
	}

	// cress check with bitcoind...
	if false {
		BitcoindResult := processRPC(b)
		json.Unmarshal(BitcoindResult, &resp)
		switch cmd.Params.(type) {
		case string:
			logg.Debug("\007Block rejected by bitcoind:", resp.Result.(string))
			ioutil.WriteFile(fmt.Sprint(bs.Block.Height, "-", bs.Block.Hash.String()), bd, 0777)
		default:
			logg.Debug("submiting block verified OK", bs.Error)
		}
	}
}

var lastGivenTime, lastGivenMinTime uint32
