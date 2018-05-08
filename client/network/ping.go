// Package network -
package network

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/calibrae-project/spawn/client/common"
)

const (
	// PingHistoryLength -
	PingHistoryLength = 20
	// PingAssumedIfUnsupported -
	PingAssumedIfUnsupported = 4999 // ms
)

// HandlePong -
func (c *OneConnection) HandlePong(pl []byte) {
	if pl != nil {
		if !bytes.Equal(pl, c.PingInProgress) {
			common.CountSafe("PongMismatch")
			return
		}
		common.CountSafe("PongOK")
		c.ExpireBlocksToGet(nil, c.X.PingSentCnt)
	} else {
		common.CountSafe("PongTimeout")
	}
	ms := time.Now().Sub(c.LastPingSent) / time.Millisecond
	if ms == 0 {
		//println(c.ConnID, "Ping returned after 0ms")
		ms = 1
	}
	c.Mutex.Lock()
	c.X.PingHistory[c.X.PingHistoryIdx] = int(ms)
	c.X.PingHistoryIdx = (c.X.PingHistoryIdx + 1) % PingHistoryLength
	c.PingInProgress = nil
	c.Mutex.Unlock()
}

// GetAveragePing -
// Returns (median) average ping
// Make sure to called it within c.Mutex.Lock()
func (c *OneConnection) GetAveragePing() int {
	if !c.X.VersionReceived {
		return 0
	}
	if c.Node.Version > 60000 {
		var pgs [PingHistoryLength]int
		var actLen int
		for _, p := range c.X.PingHistory {
			if p != 0 {
				pgs[actLen] = p
				actLen++
			}
		}
		if actLen == 0 {
			return 0
		}
		sort.Ints(pgs[:actLen])
		return pgs[actLen/2]
	}
	return PingAssumedIfUnsupported
}

// SortedConnections -
type SortedConnections []struct {
	Conn          *OneConnection
	Ping          int
	BlockCount    int
	TxsCount      int
	MinutesOnline int
	Special       bool
}

// GetSortedConnections -
// Returns the slowest peers first
// Make suure to call it with locked MutexNet
func GetSortedConnections() (list SortedConnections, anyPing bool, segwitCount int) {
	var cnt int
	var now time.Time
	var tlist SortedConnections
	now = time.Now()
	tlist = make(SortedConnections, len(OpenCons))
	for _, v := range OpenCons {
		v.Mutex.Lock()
		tlist[cnt].Conn = v
		tlist[cnt].Ping = v.GetAveragePing()
		tlist[cnt].BlockCount = len(v.blocksreceived)
		tlist[cnt].TxsCount = v.X.TxsReceived
		tlist[cnt].Special = v.X.IsSpecial
		if v.X.VersionReceived == false || v.X.ConnectedAt.IsZero() {
			tlist[cnt].MinutesOnline = 0
		} else {
			tlist[cnt].MinutesOnline = int(now.Sub(v.X.ConnectedAt) / time.Minute)
		}
		v.Mutex.Unlock()

		if tlist[cnt].Ping > 0 {
			anyPing = true
		}
		if (v.Node.Services & ServiceSegwit) != 0 {
			segwitCount++
		}

		cnt++
	}
	if cnt > 0 {
		list = make(SortedConnections, len(tlist))
		var ignoreBcnt bool // otherwise count blocks
		var idx, bestIdx, bcnt, bestBcnt, bestTcnt, bestPing int

		for idx = len(list) - 1; idx >= 0; idx-- {
			bestIdx = -1
			for i, v := range tlist {
				if v.Conn == nil {
					continue
				}
				if bestIdx < 0 {
					bestIdx = i
					bestTcnt = v.TxsCount
					bestBcnt = v.BlockCount
					bestPing = v.Ping
				} else {
					if ignoreBcnt {
						bcnt = bestBcnt
					} else {
						bcnt = v.BlockCount
					}
					if bestBcnt < bcnt ||
						bestBcnt == bcnt && bestTcnt < v.TxsCount ||
						bestBcnt == bcnt && bestTcnt == v.TxsCount && bestPing > v.Ping {
						bestBcnt = v.BlockCount
						bestTcnt = v.TxsCount
						bestPing = v.Ping
						bestIdx = i
					}
				}
			}
			list[idx] = tlist[bestIdx]
			tlist[bestIdx].Conn = nil
			ignoreBcnt = !ignoreBcnt
		}
	}
	return
}

// This function should be called only when OutConsActive >= MaxOutCons
func dropWorstPeer() bool {
	var list SortedConnections
	var anyPing bool
	var segwitCount int

	MutexNet.Lock()
	defer MutexNet.Unlock()

	list, anyPing, segwitCount = GetSortedConnections()
	if !anyPing { // if "list" is empty "anyPing" will also be false
		return false
	}

	for _, v := range list {
		if v.MinutesOnline < OnlineImmunityMinutes {
			continue
		}
		if v.Special {
			continue
		}
		if common.CFG.Net.MinSegwitCons > 0 && segwitCount <= int(common.CFG.Net.MinSegwitCons) &&
			(v.Conn.Node.Services&ServiceSegwit) != 0 {
			continue
		}
		if v.Conn.X.Incomming {
			if InConsActive+2 > common.GetUint32(&common.CFG.Net.MaxInCons) {
				common.CountSafe("PeerInDropped")
				if common.FLAG.Log {
					f, _ := os.OpenFile("drop_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
					if f != nil {
						fmt.Fprintf(f, "%s: Drop incomming id:%d  blks:%d  txs:%d  ping:%d  mins:%d\n",
							time.Now().Format("2006-01-02 15:04:05"),
							v.Conn.ConnID, v.BlockCount, v.TxsCount, v.Ping, v.MinutesOnline)
						f.Close()
					}
				}
				v.Conn.Disconnect("PeerInDropped")
				return true
			}
		} else {
			if OutConsActive+2 > common.GetUint32(&common.CFG.Net.MaxOutCons) {
				common.CountSafe("PeerOutDropped")
				if common.FLAG.Log {
					f, _ := os.OpenFile("drop_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
					if f != nil {
						fmt.Fprintf(f, "%s: Drop outgoing id:%d  blks:%d  txs:%d  ping:%d  mins:%d\n",
							time.Now().Format("2006-01-02 15:04:05"),
							v.Conn.ConnID, v.BlockCount, v.TxsCount, v.Ping, v.MinutesOnline)
						f.Close()
					}
				}
				v.Conn.Disconnect("PeerOutDropped")
				return true
			}
		}
	}
	return false
}

// TryPing -
func (c *OneConnection) TryPing() bool {
	if common.GetDuration(&common.PingPeerEvery) == 0 {
		return false // pinging disabled in global config
	}

	if c.Node.Version <= 60000 {
		return false // insufficient protocol version
	}

	if time.Now().Before(c.LastPingSent.Add(common.GetDuration(&common.PingPeerEvery))) {
		return false // not yet...
	}

	if c.PingInProgress != nil {
		c.HandlePong(nil) // this will set PingInProgress to nil
	}

	c.X.PingSentCnt++
	c.PingInProgress = make([]byte, 8)
	rand.Read(c.PingInProgress[:])
	c.SendRawMsg("ping", c.PingInProgress)
	c.LastPingSent = time.Now()
	//println(c.PeerAddr.IP(), "ping...")
	return true
}
