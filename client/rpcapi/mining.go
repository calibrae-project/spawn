package rpcapi

import (
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/client/network"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/L"
)

// MaxTxsLength - 999KB, with 1KB margin to not exceed 1MB with conibase
const MaxTxsLength = 999e3 //

// OneTransaction -
type OneTransaction struct {
	Data    string `json:"data"`
	Hash    string `json:"hash"`
	Depends []uint `json:"depends"`
	Fee     uint64 `json:"fee"`
	Sigops  uint64 `json:"sigops"`
}

// GetBlockTemplateResp -
type GetBlockTemplateResp struct {
	Capabilities      []string         `json:"capabilities"`
	Version           uint32           `json:"version"`
	PreviousBlockHash string           `json:"previousblockhash"`
	Transactions      []OneTransaction `json:"transactions"`
	Coinbaseaux       struct {
		Flags string `json:"flags"`
	} `json:"coinbaseaux"`
	Coinbasevalue uint64   `json:"coinbasevalue"`
	Longpollid    string   `json:"longpollid"`
	Target        string   `json:"target"`
	Mintime       uint     `json:"mintime"`
	Mutable       []string `json:"mutable"`
	Noncerange    string   `json:"noncerange"`
	Sigoplimit    uint     `json:"sigoplimit"`
	Sizelimit     uint     `json:"sizelimit"`
	Curtime       uint     `json:"curtime"`
	Bits          string   `json:"bits"`
	Height        uint     `json:"height"`
}

// RPCGetBlockTemplateResp -
type RPCGetBlockTemplateResp struct {
	ID     interface{}          `json:"id"`
	Result GetBlockTemplateResp `json:"result"`
	Error  interface{}          `json:"error"`
}

// GetNextBlockTemplate -
func GetNextBlockTemplate(r *GetBlockTemplateResp) {
	var zer [32]byte

	common.Last.Mutex.Lock()

	r.Curtime = uint(time.Now().Unix())
	r.Mintime = uint(common.Last.Block.GetMedianTimePast()) + 1
	if r.Curtime < r.Mintime {
		r.Curtime = r.Mintime
	}
	height := common.Last.Block.Height + 1
	bits := common.BlockChain.GetNextWorkRequired(common.Last.Block, uint32(r.Curtime))
	target := btc.SetCompact(bits).Bytes()

	r.Capabilities = []string{"proposal"}
	r.Version = 4
	r.PreviousBlockHash = common.Last.Block.BlockHash.String()
	r.Transactions, r.Coinbasevalue = GetTransactions(height, uint32(r.Mintime))
	r.Coinbasevalue += btc.GetBlockReward(height)
	r.Coinbaseaux.Flags = ""
	r.Longpollid = r.PreviousBlockHash
	r.Target = hex.EncodeToString(append(zer[:32-len(target)], target...))
	r.Mutable = []string{"time", "transactions", "prevblock"}
	r.Noncerange = "00000000ffffffff"
	r.Sigoplimit = btc.MaxBlockSigOpsCost / btc.WitnessScaleFactor
	r.Sizelimit = 1e6
	r.Bits = fmt.Sprintf("%08x", bits)
	r.Height = uint(height)

	lastGivenTime = uint32(r.Curtime)
	lastGivenMinTime = uint32(r.Mintime)

	common.Last.Mutex.Unlock()
}

/* memory pool transaction sorting stuff */
type oneMiningTx struct {
	*network.OneTxToSend
	depends []uint
	startat int
}

type sortedTxList []*oneMiningTx

func (tl sortedTxList) Len() int           { return len(tl) }
func (tl sortedTxList) Swap(i, j int)      { tl[i], tl[j] = tl[j], tl[i] }
func (tl sortedTxList) Less(i, j int) bool { return tl[j].Fee < tl[i].Fee }

var txsSoFar map[[32]byte]uint
var totlen int
var sigops uint64

// getNextTrancheOfTxs -
func getNextTrancheOfTxs(height, timestamp uint32) (res sortedTxList) {
	var unsp *btc.TxOut
	var allInputsFound bool
	for _, v := range network.TransactionsToSend {
		tx := v.Tx

		if _, ok := txsSoFar[tx.Hash.Hash]; ok {
			continue
		}

		if !tx.IsFinal(height, timestamp) {
			continue
		}

		if totlen+len(v.Raw) > 1e6 {
			L.Debug("Too many txs - limit to 999000 bytes")
			return
		}
		totlen += len(v.Raw)

		if sigops+v.SigopsCost > btc.MaxBlockSigOpsCost {
			L.Debug("Too many sigops - limit to 999000 bytes")
			return
		}
		sigops += v.SigopsCost

		allInputsFound = true
		var depends []uint
		for i := range tx.TxIn {
			unsp = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if unsp == nil {
				// not found in the confirmed blocks
				// check if txid is in txsSoFar
				if idx, ok := txsSoFar[tx.TxIn[i].Input.Hash]; !ok {
					// also not in txsSoFar
					allInputsFound = false
					break
				} else {
					depends = append(depends, idx)
				}
			}
		}

		if allInputsFound {
			res = append(res, &oneMiningTx{OneTxToSend: v, depends: depends, startat: 1 + len(txsSoFar)})
		}
	}
	return
}

// GetTransactions -
func GetTransactions(height, timestamp uint32) (res []OneTransaction, totfees uint64) {

	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	var cnt int
	var sorted sortedTxList
	txsSoFar = make(map[[32]byte]uint)
	totlen = 0
	sigops = 0
	L.Debug("\ngetting txs from the pool of", len(network.TransactionsToSend), "...")
	for {
		newPiece := getNextTrancheOfTxs(height, timestamp)
		if newPiece.Len() == 0 {
			break
		}
		L.Debug("adding another", len(newPiece))
		sort.Sort(newPiece)

		for i := 0; i < len(newPiece); i++ {
			txsSoFar[newPiece[i].Tx.Hash.Hash] = uint(1 + len(sorted) + i)
		}

		sorted = append(sorted, newPiece...)
	}
	/*if len(txsSoFar)!=len(network.TransactionsToSend) {
		println("ERROR: txsSoFar len", len(txsSoFar), " - please report!")
	}*/
	txsSoFar = nil // leave it for the garbage collector

	res = make([]OneTransaction, len(sorted))
	for cnt = 0; cnt < len(sorted); cnt++ {
		v := sorted[cnt]
		res[cnt].Data = hex.EncodeToString(v.Raw)
		res[cnt].Hash = v.Tx.Hash.String()
		res[cnt].Fee = v.Fee
		res[cnt].Sigops = v.SigopsCost
		res[cnt].Depends = v.depends
		totfees += v.Fee
		// L.Debug("", cnt+1, v.Tx.Hash.String(), "  turn:", v.startat, "  spb:", int(v.Fee)/len(v.Data), "  depend:", fmt.Sprint(v.depends))
	}

	L.Debug("returning transacitons:", totlen, len(res))
	return
}
