// Package network -
package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/L"
)

// ProcessGetData -
func (c *OneConnection) ProcessGetData(pl []byte) {
	//var notfound []byte

	//println(c.PeerAddr.IP(), "getdata")
	b := bytes.NewReader(pl)
	cnt, e := btc.ReadVLen(b)
	if e != nil {
		L.Debug("ProcessGetData:", e.Error(), c.PeerAddr.IP())
		return
	}
	for i := 0; i < int(cnt); i++ {
		var typ uint32
		var h [36]byte

		n, _ := b.Read(h[:])
		if n != 36 {
			println("ProcessGetData: pl too short", c.PeerAddr.IP())
			return
		}

		typ = binary.LittleEndian.Uint32(h[:4])
		c.Mutex.Lock()
		c.InvStore(typ, h[4:36])
		c.Mutex.Unlock()

		common.CountSafe(fmt.Sprintf("GetdataType-%x", typ))
		if typ == MsgBlock || typ == MsgWitnessBlock {
			hash := btc.NewUint256(h[4:])
			crec, _, er := common.BlockChain.Blocks.BlockGetExt(hash)

			if er == nil {
				bl := crec.Data
				if typ == MsgBlock {
					// remove witness data from the block
					if crec.Block == nil {
						crec.Block, _ = btc.NewBlock(bl)
					}
					if crec.Block.NoWitnessData == nil {
						crec.Block.BuildNoWitnessData()
					}
					//println("block size", len(crec.Data), "->", len(bl))
					bl = crec.Block.NoWitnessData
				}
				c.SendRawMsg("block", bl)
			} else {
				//fmt.Println("BlockGetExt-2 failed for", hash.String(), er.Error())
				//notfound = append(notfound, h[:]...)
			}
		} else if typ == MsgTx || typ == MsgWitnessTx {
			// transaction
			TxMutex.Lock()
			if tx, ok := TransactionsToSend[btc.NewUint256(h[4:]).BIdx()]; ok && tx.Blocked == 0 {
				tx.SentCnt++
				tx.Lastsent = time.Now()
				TxMutex.Unlock()
				if tx.SegWit == nil || typ == MsgWitnessTx {
					c.SendRawMsg("tx", tx.Raw)
				} else {
					c.SendRawMsg("tx", tx.Serialize())
				}
			} else {
				TxMutex.Unlock()
				//notfound = append(notfound, h[:]...)
			}
		} else if typ == MsgCompactBlock {
			if !c.SendCompactBlock(btc.NewUint256(h[4:])) {
				L.Debug(c.ConnID, c.PeerAddr.IP(), c.Node.Agent, "asked for CmpctBlk we don't have", btc.NewUint256(h[4:]).String())
				if c.Misbehave("GetCmpctBlk", 100) {
					break
				}
			}
		} else {
			if typ > 0 && typ <= 3 /*3 is a filtered block(we dont support it)*/ {
				//notfound = append(notfound, h[:]...)
			}
		}
	}

	/*
		if len(notfound)>0 {
			buf := new(bytes.Buffer)
			btc.WriteVlen(buf, uint64(len(notfound)/36))
			buf.Write(notfound)
			c.SendRawMsg("notfound", buf.Bytes())
		}
	*/
}

// This function is called from a net conn thread
func netBlockReceived(conn *OneConnection, b []byte) {
	if len(b) < 100 {
		conn.DoS("ShortBlock")
		return
	}

	hash := btc.NewSha2Hash(b[:80])
	idx := hash.BIdx()
	//println("got block data", hash.String())

	MutexRcv.Lock()

	// the blocks seems to be fine
	if rb, got := ReceivedBlocks[idx]; got {
		rb.Cnt++
		common.CountSafe("BlockSameRcvd")
		conn.Mutex.Lock()
		delete(conn.GetBlockInProgress, idx)
		conn.Mutex.Unlock()
		MutexRcv.Unlock()
		return
	}

	// remove from BlocksToGet:
	b2g := BlocksToGet[idx]
	if b2g == nil {
		//println("Block", hash.String(), " from", conn.PeerAddr.IP(), conn.Node.Agent, " was not expected")

		var hdr [81]byte
		var sta int
		copy(hdr[:80], b[:80])
		sta, b2g = conn.ProcessNewHeader(hdr[:])
		if b2g == nil {
			if sta == PHstatusFatal {
				L.Debug("Unrequested Block: FAIL - Ban", conn.PeerAddr.IP(), conn.Node.Agent)
				conn.DoS("BadUnreqBlock")
			} else {
				common.CountSafe("ErrUnreqBlock")
			}
			//conn.Disconnect()
			MutexRcv.Unlock()
			return
		}
		if sta == PHstatusNew {
			b2g.SendInvs = true
		}
		//println(c.ConnID, " - taking this new block")
		common.CountSafe("UnxpectedBlockNEW")
	}

	//println("block", b2g.BlockTreeNode.Height," len", len(b), " got from", conn.PeerAddr.IP(), b2g.InProgress)
	b2g.Block.Raw = b
	if conn.X.Authorized {
		b2g.Block.Trusted = true
	}

	er := common.BlockChain.PostCheckBlock(b2g.Block)
	if er != nil {
		b2g.InProgress--
		L.Debug("Corrupt block received from", conn.PeerAddr.IP(), er.Error())
		//ioutil.WriteFile(hash.String() + ".bin", b, 0700)
		conn.DoS("BadBlock")

		// we don't need to remove from conn.GetBlockInProgress as we're disconnecting

		if b2g.Block.MerkleRootMatch() {
			L.Debug("It was a wrongly mined one - clean it up")
			DelB2G(idx) //remove it from BlocksToGet
			if b2g.BlockTreeNode == LastCommitedHeader {
				LastCommitedHeader = LastCommitedHeader.Parent
			}
			common.BlockChain.DeleteBranch(b2g.BlockTreeNode, delB2Gcallback)
		}

		MutexRcv.Unlock()
		return
	}

	orb := &OneReceivedBlock{TmStart: b2g.Started, TmPreproc: b2g.TmPreproc,
		TmDownload: conn.LastMsgTime, FromConID: conn.ConnID, DoInvs: b2g.SendInvs}

	conn.Mutex.Lock()
	bip := conn.GetBlockInProgress[idx]
	if bip == nil {
		L.Debug(conn.ConnID, "received unrequested block", hash.String())
		common.CountSafe("UnreqBlockRcvd")
		conn.counters["NewBlock!"]++
		orb.TxMissing = -2
	} else {
		delete(conn.GetBlockInProgress, idx)
		conn.counters["NewBlock"]++
		orb.TxMissing = -1
	}
	conn.blocksreceived = append(conn.blocksreceived, time.Now())
	conn.Mutex.Unlock()

	ReceivedBlocks[idx] = orb
	DelB2G(idx) //remove it from BlocksToGet if no more pending downloads

	storeOnDisk := len(BlocksToGet) > 10 && common.GetBool(&common.CFG.Memory.CacheOnDisk) && len(b2g.Block.Raw) > 16*1024
	MutexRcv.Unlock()

	var bei *btc.BlockExtraInfo

	if storeOnDisk {
		if e := ioutil.WriteFile(common.TempBlocksDir()+hash.String(), b2g.Block.Raw, 0600); e == nil {
			bei = new(btc.BlockExtraInfo)
			*bei = b2g.Block.BlockExtraInfo
			b2g.Block = nil
		} else {
			println("write tmp block:", e.Error())
		}
	}

	NetBlocks <- &BlockRcvd{Conn: conn, Block: b2g.Block, BlockTreeNode: b2g.BlockTreeNode, OneReceivedBlock: orb, BlockExtraInfo: bei}
}

// Read VLen followed by the number of locators
// parse the payload of getblocks and getheaders messages
func parseLocatorsPayload(pl []byte) (h2get []*btc.Uint256, hashstop *btc.Uint256, er error) {
	var cnt uint64
	var h [32]byte
	var ver uint32

	b := bytes.NewReader(pl)

	// version
	if er = binary.Read(b, binary.LittleEndian, &ver); er != nil {
		return
	}

	// hash count
	cnt, er = btc.ReadVLen(b)
	if er != nil {
		return
	}

	// block locator hashes
	if cnt > 0 {
		h2get = make([]*btc.Uint256, cnt)
		for i := 0; i < int(cnt); i++ {
			if _, er = b.Read(h[:]); er != nil {
				return
			}
			h2get[i] = btc.NewUint256(h[:])
		}
	}

	// hash_stop
	if _, er = b.Read(h[:]); er != nil {
		return
	}
	hashstop = btc.NewUint256(h[:])

	return
}

// Call it with locked MutexRcv
func getBlockToFetch(maxHeight uint32, countInProgress, avgBlockSize uint) (lowestFound *OneBlockToGet) {
	for _, v := range BlocksToGet {
		if v.InProgress == countInProgress && v.Block.Height <= maxHeight &&
			(lowestFound == nil || v.Block.Height < lowestFound.Block.Height) {
			lowestFound = v
		}
	}
	return
}

// GetBlockData -
func (c *OneConnection) GetBlockData() (yes bool) {
	//MaxGetDataForward
	// Need to send getdata...?
	MutexRcv.Lock()
	defer MutexRcv.Unlock()

	if LowestIndexToBlocksToGet == 0 || len(BlocksToGet) == 0 {
		c.IncCnt("FetchNoBlocksToGet", 1)
		// wake up in one minute, just in case
		c.nextGetData = time.Now().Add(60 * time.Second)
		return
	}

	c.Mutex.Lock()
	if c.X.BlocksExpired > 0 { // Do not fetch blocks from nodes that had not given us some in the past
		c.Mutex.Unlock()
		c.IncCnt("FetchHasBlocksExpired", 1)
		return
	}
	cbip := len(c.GetBlockInProgress)
	c.Mutex.Unlock()

	if cbip >= MaxPeersBlocksInProgress {
		c.IncCnt("FetchMaxCountInProgress", 1)
		// wake up in a few seconds, maybe some blocks will complete by then
		c.nextGetData = time.Now().Add(1 * time.Second)
		return
	}

	avgBlockSize := common.AverageBlockSize.Get()
	blockDataInProgress := cbip * avgBlockSize

	if blockDataInProgress > 0 && (blockDataInProgress+avgBlockSize) > MaxGetDataForward {
		c.IncCnt("FetchMaxBytesInProgress", 1)
		// wake up in a few seconds, maybe some blocks will complete by then
		c.nextGetData = time.Now().Add(1 * time.Second) // wait for some blocks to complete
		return
	}

	var cnt uint64
	var blockType uint32

	if (c.Node.Services & ServiceSegwit) != 0 {
		blockType = MsgWitnessBlock
	} else {
		blockType = MsgBlock
	}

	// We can issue getdata for this peer
	// Let's look for the lowest height block in BlocksToGet that isn't being downloaded yet

	common.Last.Mutex.Lock()
	maxHeight := common.Last.Block.Height + uint32(MaxBlocksForwardSize/avgBlockSize)
	if maxHeight > common.Last.Block.Height+MaxBlocksForwardCount {
		maxHeight = common.Last.Block.Height + MaxBlocksForwardCount
	}
	common.Last.Mutex.Unlock()
	if maxHeight > c.Node.Height {
		maxHeight = c.Node.Height
	}
	if maxHeight > LastCommitedHeader.Height {
		maxHeight = LastCommitedHeader.Height
	}

	if common.BlockChain.Consensus.EnforceSegwit != 0 && (c.Node.Services&ServiceSegwit) == 0 { // no segwit node
		if maxHeight >= common.BlockChain.Consensus.EnforceSegwit-1 {
			maxHeight = common.BlockChain.Consensus.EnforceSegwit - 1
			if maxHeight <= common.Last.Block.Height {
				c.IncCnt("FetchNoWitness", 1)
				c.nextGetData = time.Now().Add(3600 * time.Second) // never do getdata
				return
			}
		}
	}

	invs := new(bytes.Buffer)
	var countInProgress uint

	for {
		var lowestFound *OneBlockToGet

		// Get block to fetch:

		for bh := LowestIndexToBlocksToGet; bh <= maxHeight; bh++ {
			if idxlst, ok := IndexToBlocksToGet[bh]; ok {
				for _, idx := range idxlst {
					v := BlocksToGet[idx]
					if v.InProgress == countInProgress && (lowestFound == nil || v.Block.Height < lowestFound.Block.Height) {
						c.Mutex.Lock()
						if _, ok := c.GetBlockInProgress[idx]; !ok {
							lowestFound = v
						}
						c.Mutex.Unlock()
					}
				}
			}
		}

		if lowestFound == nil {
			countInProgress++
			if countInProgress >= uint(common.CFG.Net.MaxBlockAtOnce) {
				break
			}
			continue
		}

		binary.Write(invs, binary.LittleEndian, blockType)
		invs.Write(lowestFound.BlockHash.Hash[:])
		lowestFound.InProgress++
		cnt++

		c.Mutex.Lock()
		c.GetBlockInProgress[lowestFound.BlockHash.BIdx()] =
			&oneBlockDl{hash: lowestFound.BlockHash, start: time.Now(), SentAtPingCnt: c.X.PingSentCnt}
		cbip = len(c.GetBlockInProgress)
		c.Mutex.Unlock()

		if cbip >= MaxPeersBlocksInProgress {
			break // no more than 2000 blocks in progress / peer
		}
		blockDataInProgress += avgBlockSize
		if blockDataInProgress > MaxGetDataForward {
			break
		}
	}

	if cnt == 0 {
		//println(c.ConnID, "fetch nothing", cbip, blockDataInProgress, maxHeight-common.Last.Block.Height, countInProgress)
		c.IncCnt("FetchNothing", 1)
		// wake up in a few seconds, maybe it will be different next time
		c.nextGetData = time.Now().Add(5 * time.Second)
		return
	}

	bu := new(bytes.Buffer)
	btc.WriteVlen(bu, uint64(cnt))
	pl := append(bu.Bytes(), invs.Bytes()...)
	L.Debug(c.ConnID, "fetching", cnt, "new blocks ->", cbip)
	c.SendRawMsg("getdata", pl)
	yes = true

	return
}
