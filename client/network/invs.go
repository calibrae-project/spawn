//Package network -
package network

import (
	"fmt"
	//"time"
	"bytes"
	"encoding/binary"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/chain"
	"github.com/ParallelCoinTeam/duod/lib/L"
)

const (
	// MsgWitnessFlag -
	MsgWitnessFlag = 0x40000000
	// MsgTx -
	MsgTx = 1
	// MsgBlock -
	MsgBlock = 2
	// MsgCompactBlock -
	MsgCompactBlock = 4
	// MsgWitnessTx -
	MsgWitnessTx = MsgTx | MsgWitnessFlag
	// MsgWitnessBlock -
	MsgWitnessBlock = MsgBlock | MsgWitnessFlag
)

func blockReceived(bh *btc.Uint256) (ok bool) {
	MutexRcv.Lock()
	_, ok = ReceivedBlocks[bh.BIdx()]
	MutexRcv.Unlock()
	return
}

func hash2invid(hash []byte) uint64 {
	return binary.LittleEndian.Uint64(hash[4:12])
}

// InvStore - Make sure c.Mutex is locked when calling it
func (c *OneConnection) InvStore(typ uint32, hash []byte) {
	invID := hash2invid(hash)
	if len(c.InvDone.History) < MaxInvHistory {
		c.InvDone.History = append(c.InvDone.History, invID)
		c.InvDone.Map[invID] = typ
		c.InvDone.Idx++
		return
	}
	if c.InvDone.Idx == MaxInvHistory {
		c.InvDone.Idx = 0
	}
	delete(c.InvDone.Map, c.InvDone.History[c.InvDone.Idx])
	c.InvDone.History[c.InvDone.Idx] = invID
	c.InvDone.Map[invID] = typ
	c.InvDone.Idx++
}

// ProcessInv -
func (c *OneConnection) ProcessInv(pl []byte) {
	if len(pl) < 37 {
		L.Debug(c.PeerAddr.IP(), "inv payload too short", len(pl))
		c.DoS("InvEmpty")
		return
	}
	c.Mutex.Lock()
	c.X.InvsRecieved++
	c.Mutex.Unlock()

	cnt, of := btc.VLen(pl)
	if len(pl) != of+36*cnt {
		println("inv payload length mismatch", len(pl), of, cnt)
	}

	for i := 0; i < cnt; i++ {
		typ := binary.LittleEndian.Uint32(pl[of : of+4])
		c.Mutex.Lock()
		c.InvStore(typ, pl[of+4:of+36])
		ahr := c.X.AllHeadersReceived
		c.Mutex.Unlock()
		common.CountSafe(fmt.Sprint("InvGot-", typ))
		if typ == MsgBlock {
			bhash := btc.NewUint256(pl[of+4 : of+36])
			if !ahr {
				common.CountSafe("InvBlockIgnored")
			} else {
				if !blockReceived(bhash) {
					MutexRcv.Lock()
					if b2g, ok := BlocksToGet[bhash.BIdx()]; ok {
						if c.Node.Height < b2g.Block.Height {
							c.Node.Height = b2g.Block.Height
						}
						common.CountSafe("InvBlockFresh")
						L.Debug(c.PeerAddr.IP(), c.Node.Version, "also knows the block", b2g.Block.Height, bhash.String())
						c.MutexSetBool(&c.X.GetBlocksDataNow, true)
					} else {
						common.CountSafe("InvBlockNew")
						c.ReceiveHeadersNow()
						L.Debug(c.PeerAddr.IP(), c.Node.Version, "possibly new block", bhash.String())
					}
					MutexRcv.Unlock()
				} else {
					common.CountSafe("InvBlockOld")
				}
			}
		} else if typ == MsgTx {
			if common.AcceptTx() {
				c.TxInvNotify(pl[of+4 : of+36])
			} else {
				common.CountSafe("InvTxIgnored")
			}
		}
		of += 36
	}

	return
}

// NetRouteInv -
func NetRouteInv(typ uint32, h *btc.Uint256, fromConn *OneConnection) uint32 {
	var feeSpkb uint64
	if typ == MsgTx {
		TxMutex.Lock()
		if tx, ok := TransactionsToSend[h.BIdx()]; ok {
			feeSpkb = (1000 * tx.Fee) / uint64(tx.VSize())
		} else {
			L.Debug("NetRouteInv: txid", h.String(), "not in mempool")
		}
		TxMutex.Unlock()
	}
	return NetRouteInvExt(typ, h, fromConn, feeSpkb)
}

// NetRouteInvExt - This function is called from the main thread (or from an UI)
func NetRouteInvExt(typ uint32, h *btc.Uint256, fromConn *OneConnection, feeSpkb uint64) (cnt uint32) {
	common.CountSafe(fmt.Sprint("NetRouteInv", typ))

	// Prepare the inv
	inv := new([36]byte)
	binary.LittleEndian.PutUint32(inv[0:4], typ)
	copy(inv[4:36], h.Bytes())

	// Append it to PendingInvs in each open connection
	MutexNet.Lock()
	for _, v := range OpenCons {
		if v != fromConn { // except the one that this inv came from
			sendInv := true
			v.Mutex.Lock()
			if typ == MsgTx {
				if v.Node.DoNotRelayTxs {
					sendInv = false
					common.CountSafe("SendInvNoTxNode")
				} else if v.X.MinFeeSPKB > 0 && uint64(v.X.MinFeeSPKB) > feeSpkb {
					sendInv = false
					common.CountSafe("SendInvFeeTooLow")
				}

				/* This is to prevent sending own txs to "spying" peers:
				else if fromConn==nil && v.X.InvsRecieved==0 {
					sendInv = false
					common.CountSafe("SendInvOwnBlocked")
				}
				*/
			}
			if sendInv {
				if len(v.PendingInvs) < 500 {
					if typ, ok := v.InvDone.Map[hash2invid(inv[4:36])]; ok {
						common.CountSafe(fmt.Sprint("SendInvSame-", typ))
					} else {
						v.PendingInvs = append(v.PendingInvs, inv)
						cnt++
					}
				} else {
					common.CountSafe("SendInvFull")
				}
			}
			v.Mutex.Unlock()
		}
	}
	MutexNet.Unlock()
	return
}

// Call this function only when BlockIndexAccess is locked
func addInvBlockBranch(inv map[[32]byte]bool, bl *chain.BlockTreeNode, stop *btc.Uint256) {
	if len(inv) >= 500 || bl.BlockHash.Equal(stop) {
		return
	}
	inv[bl.BlockHash.Hash] = true
	for i := range bl.Childs {
		if len(inv) >= 500 {
			return
		}
		addInvBlockBranch(inv, bl.Childs[i], stop)
	}
}

// GetBlocks -
func (c *OneConnection) GetBlocks(pl []byte) {
	h2get, hashstop, e := parseLocatorsPayload(pl)

	if e != nil || len(h2get) < 1 || hashstop == nil {
		L.Debug("GetBlocks: error parsing payload from", c.PeerAddr.IP())
		c.DoS("BadGetBlks")
		return
	}

	invs := make(map[[32]byte]bool, 500)
	for i := range h2get {
		common.BlockChain.BlockIndexAccess.Lock()
		if bl, ok := common.BlockChain.BlockIndex[h2get[i].BIdx()]; ok {
			// make sure that this block is in our main chain
			common.Last.Mutex.Lock()
			end := common.Last.Block
			common.Last.Mutex.Unlock()
			for ; end != nil && end.Height >= bl.Height; end = end.Parent {
				if end == bl {
					addInvBlockBranch(invs, bl, hashstop) // Yes - this is the main chain
					if len(invs) > 0 {
						common.BlockChain.BlockIndexAccess.Unlock()

						inv := new(bytes.Buffer)
						btc.WriteVlen(inv, uint64(len(invs)))
						for k := range invs {
							binary.Write(inv, binary.LittleEndian, uint32(2))
							inv.Write(k[:])
						}
						c.SendRawMsg("inv", inv.Bytes())
						return
					}
				}
			}
		}
		common.BlockChain.BlockIndexAccess.Unlock()
	}

	common.CountSafe("GetblksMissed")
	return
}

// SendInvs -
func (c *OneConnection) SendInvs() (res bool) {
	bTxs := new(bytes.Buffer)
	bBlk := new(bytes.Buffer)
	var cBlk []*btc.Uint256

	c.Mutex.Lock()
	if len(c.PendingInvs) > 0 {
		for i := range c.PendingInvs {
			var invSentOtherwise bool
			typ := binary.LittleEndian.Uint32((*c.PendingInvs[i])[:4])
			c.InvStore(typ, (*c.PendingInvs[i])[4:36])
			if typ == MsgBlock {
				if c.Node.SendCmpctVer >= 1 && c.Node.HighBandwidth {
					cBlk = append(cBlk, btc.NewUint256((*c.PendingInvs[i])[4:]))
					invSentOtherwise = true
				} else if c.Node.SendHeaders {
					// convert block inv to block header
					common.BlockChain.BlockIndexAccess.Lock()
					bl := common.BlockChain.BlockIndex[btc.NewUint256((*c.PendingInvs[i])[4:]).BIdx()]
					if bl != nil {
						bBlk.Write(bl.BlockHeader[:])
						bBlk.Write([]byte{0}) // 0 txs
					}
					common.BlockChain.BlockIndexAccess.Unlock()
					invSentOtherwise = true
				}
			}

			if !invSentOtherwise {
				bTxs.Write((*c.PendingInvs[i])[:])
			}
		}
		res = true
	}
	c.PendingInvs = nil
	c.Mutex.Unlock()

	if len(cBlk) > 0 {
		for _, h := range cBlk {
			c.SendCompactBlock(h)
		}
	}

	if bBlk.Len() > 0 {
		common.CountSafe("InvSentAsHeader")
		b := new(bytes.Buffer)
		btc.WriteVlen(b, uint64(bBlk.Len()/81))
		c.SendRawMsg("headers", append(b.Bytes(), bBlk.Bytes()...))
		L.Debug("sent block's header(s)", bBlk.Len(), uint64(bBlk.Len()/81))
	}

	if bTxs.Len() > 0 {
		b := new(bytes.Buffer)
		btc.WriteVlen(b, uint64(bTxs.Len()/36))
		c.SendRawMsg("inv", append(b.Bytes(), bTxs.Bytes()...))
	}

	return
}
