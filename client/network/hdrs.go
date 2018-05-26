// Package network -
package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/chain"
)

const (
	// PHstatusNew -
	PHstatusNew = iota
	// PHstatusFresh -
	PHstatusFresh
	// PHstatusOld -
	PHstatusOld
	// PHstatusError -
	PHstatusError
	// PHstatusFatal -
	PHstatusFatal
)

// ProcessNewHeader -
func (c *OneConnection) ProcessNewHeader(hdr []byte) (int, *OneBlockToGet) {
	var ok bool
	var b2g *OneBlockToGet
	bl, _ := btc.NewBlock(hdr)

	c.Mutex.Lock()
	c.InvStore(MsgBlock, bl.Hash.Hash[:])
	c.Mutex.Unlock()

	if _, ok = ReceivedBlocks[bl.Hash.BIdx()]; ok {
		common.CountSafe("HeaderOld")
		//fmt.Println("", i, bl.Hash.String(), "-already received")
		return PHstatusOld, nil
	}

	if b2g, ok = BlocksToGet[bl.Hash.BIdx()]; ok {
		common.CountSafe("HeaderFresh")
		//fmt.Println(c.PeerAddr.IP(), "block", bl.Hash.String(), " not new but get it")
		return PHstatusFresh, b2g
	}

	common.CountSafe("HeaderNew")
	//fmt.Println("", i, bl.Hash.String(), " - NEW!")

	common.BlockChain.BlockIndexAccess.Lock()
	defer common.BlockChain.BlockIndexAccess.Unlock()

	if _, dos, er := common.BlockChain.PreCheckBlock(bl); er != nil {
		common.CountSafe("PreCheckBlockFail")
		//println("PreCheckBlock err", dos, er.Error())
		if dos {
			return PHstatusFatal, nil
		}
		return PHstatusError, nil
	}

	node := common.BlockChain.AcceptHeader(bl)
	b2g = &OneBlockToGet{Started: c.LastMsgTime, Block: bl, BlockTreeNode: node, InProgress: 0}
	AddB2G(b2g)
	LastCommitedHeader = node

	if common.LastTrustedBlockMatch(node.BlockHash) {
		common.SetUint32(&common.LastTrustedBlockHeight, node.Height)
		for node != nil {
			node.Trusted = true
			node = node.Parent
		}
	}
	b2g.Block.Trusted = b2g.BlockTreeNode.Trusted

	return PHstatusNew, b2g
}

// HandleHeaders -
func (c *OneConnection) HandleHeaders(pl []byte) (newHeadersGot int) {
	var highestBlockFound uint32

	c.MutexSetBool(&c.X.GetHeadersInProgress, false)

	b := bytes.NewReader(pl)
	cnt, e := btc.ReadVLen(b)
	if e != nil {
		println("HandleHeaders:", e.Error(), c.PeerAddr.IP())
		return
	}

	if cnt > 0 {
		MutexRcv.Lock()
		defer MutexRcv.Unlock()

		for i := 0; i < int(cnt); i++ {
			var hdr [81]byte

			n, _ := b.Read(hdr[:])
			if n != 81 {
				println("HandleHeaders: pl too short", c.PeerAddr.IP())
				c.DoS("HdrErr1")
				return
			}

			if hdr[80] != 0 {
				fmt.Println("Unexpected value of txn_count from", c.PeerAddr.IP())
				c.DoS("HdrErr2")
				return
			}

			sta, b2g := c.ProcessNewHeader(hdr[:])
			if b2g == nil {
				if sta == PHstatusFatal {
					//println("c.DoS(BadHeader)")
					c.DoS("BadHeader")
					return
				} else if sta == PHstatusError {
					//println("c.Misbehave(BadHeader)")
					c.Misbehave("BadHeader", 50) // do it 20 times and you are banned
				}
			} else {
				if sta == PHstatusNew {
					if cnt == 1 {
						b2g.SendInvs = true
					}
					newHeadersGot++
				}
				if b2g.Block.Height > highestBlockFound {
					highestBlockFound = b2g.Block.Height
				}
				if c.Node.Height < b2g.Block.Height {
					c.Mutex.Lock()
					c.Node.Height = b2g.Block.Height
					c.Mutex.Unlock()
				}
				c.MutexSetBool(&c.X.GetBlocksDataNow, true)
				if b2g.TmPreproc.IsZero() { // do not overwrite TmPreproc (in case of PHstatusFresh)
					b2g.TmPreproc = time.Now()
				}
			}
		}
	}

	c.Mutex.Lock()
	c.X.LastHeadersEmpty = highestBlockFound <= c.X.LastHeadersHeightAsk
	c.X.TotalNewHeadersCount += newHeadersGot
	if newHeadersGot == 0 {
		c.X.AllHeadersReceived = true
	}
	c.Mutex.Unlock()

	return
}

// ReceiveHeadersNow -
func (c *OneConnection) ReceiveHeadersNow() {
	c.Mutex.Lock()
	c.X.AllHeadersReceived = false
	c.Mutex.Unlock()
}

// GetHeaders -
// Handle getheaders protocol command
// https://en.bitcoin.it/wiki/Protocol_specification#getheaders
func (c *OneConnection) GetHeaders(pl []byte) {
	h2get, hashstop, e := parseLocatorsPayload(pl)
	if e != nil || hashstop == nil {
		println("GetHeaders: error parsing payload from", c.PeerAddr.IP())
		c.DoS("BadGetHdrs")
		return
	}

	var bestBlock, lastBlock *chain.BlockTreeNode

	//common.Last.Mutex.Lock()
	MutexRcv.Lock()
	lastBlock = LastCommitedHeader
	MutexRcv.Unlock()
	//common.Last.Mutex.Unlock()

	common.BlockChain.BlockIndexAccess.Lock()

	//println("GetHeaders", len(h2get), hashstop.String())
	if len(h2get) > 0 {
		for i := range h2get {
			if bl, ok := common.BlockChain.BlockIndex[h2get[i].BIdx()]; ok {
				if bestBlock == nil || bl.Height > bestBlock.Height {
					//println(" ... bbl", i, bl.Height, bl.BlockHash.String())
					bestBlock = bl
				}
			}
		}
	} else {
		bestBlock = common.BlockChain.BlockIndex[hashstop.BIdx()]
	}

	if bestBlock == nil {
		common.CountSafe("GetHeadersBadBlock")
		bestBlock = common.BlockChain.BlockTreeRoot
	}

	var resp []byte
	var cnt uint32

	defer func() {
		// If we get a hash of an old orphaned blocks, FindPathTo() will panic, so...
		if r := recover(); r != nil {
			common.CountSafe("GetHeadersOrphBlk")
		}

		common.BlockChain.BlockIndexAccess.Unlock()

		// send the response
		out := new(bytes.Buffer)
		btc.WriteVlen(out, uint64(cnt))
		out.Write(resp)
		c.SendRawMsg("headers", out.Bytes())
	}()

	for cnt < 2000 {
		if lastBlock.Height <= bestBlock.Height {
			break
		}
		bestBlock = bestBlock.FindPathTo(lastBlock)
		if bestBlock == nil {
			break
		}
		resp = append(resp, append(bestBlock.BlockHeader[:], 0)...) // 81st byte is always zero
		cnt++
	}

	// Note: the deferred function will be called before exiting

	return
}

func (c *OneConnection) sendGetHeaders() {
	MutexRcv.Lock()
	lb := LastCommitedHeader
	MutexRcv.Unlock()
	minHeight := int(lb.Height) - chain.MovingCheckopintDepth
	if minHeight < 0 {
		minHeight = 0
	}

	blks := new(bytes.Buffer)
	var cnt uint64
	var step int
	step = 1
	for cnt < 50 /*it shoudl never get that far, but just in case...*/ {
		blks.Write(lb.BlockHash.Hash[:])
		cnt++
		//println(" geth", cnt, "height", lb.Height, lb.BlockHash.String())
		if int(lb.Height) <= minHeight {
			break
		}
		for tmp := 0; tmp < step && lb != nil && int(lb.Height) > minHeight; tmp++ {
			lb = lb.Parent
		}
		if lb == nil {
			break
		}
		if cnt >= 10 {
			step = step * 2
		}
	}
	var nullStop [32]byte
	blks.Write(nullStop[:])

	bhdr := new(bytes.Buffer)
	binary.Write(bhdr, binary.LittleEndian, common.Version)
	btc.WriteVlen(bhdr, cnt)

	c.SendRawMsg("getheaders", append(bhdr.Bytes(), blks.Bytes()...))
	c.X.LastHeadersHeightAsk = lb.Height
	c.MutexSetBool(&c.X.GetHeadersInProgress, true)
	c.X.GetHeadersTimeout = time.Now().Add(GetHeadersTimeout)
}
