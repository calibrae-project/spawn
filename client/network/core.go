// Package network -
package network

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/calibrae-project/spawn/client/common"
	"github.com/calibrae-project/spawn/lib/btc"
	"github.com/calibrae-project/spawn/lib/others/peersdb"
)

const (
	// AskAddrsEvery -
	AskAddrsEvery = (5 * time.Minute)
	// MaxAddrsPerMessage -
	MaxAddrsPerMessage = 500
	// SendBufSize -If you'd have this much in the send buffer, disconnect the peer
	SendBufSize = 16 * 1024 * 1024
	// SendBufMask -
	SendBufMask = SendBufSize - 1
	// GetHeadersTimeout - Timeout to receive headers
	GetHeadersTimeout = 2 * time.Minute
	// VersionMsgTimeout - Timeout to receive the version message after connecting
	VersionMsgTimeout = 20 * time.Second
	// TCPDialTimeout - If it does not connect within this time, assume it dead
	TCPDialTimeout = 20 * time.Second
	// MinProtoVersion -
	MinProtoVersion = 209
	// HammeringMinReconnect - If any incoming peer reconnects in below this time, ban it
	HammeringMinReconnect = 60 * time.Second
	// ExpireCachedAfter - If a block stays in the cache for that long, drop it
	ExpireCachedAfter = 20 * time.Minute
	// MaxPeersBlocksInProgress -
	MaxPeersBlocksInProgress = 500
	// MaxBlocksForwardCount - Never ask for a block higher than current top + this value
	MaxBlocksForwardCount = 5000
	// MaxBlocksForwardSize - this  will store about that much blocks data in RAM
	MaxBlocksForwardSize = 500e6
	// MaxGetDataForward - Download up to 2MB forward (or one block)
	MaxGetDataForward = 2e6
	// MaintenancePeriod -
	MaintenancePeriod = time.Minute
	// MaxInvHistory -
	MaxInvHistory = 500
	// ServiceSegwit -
	ServiceSegwit = 0x8
	// TxsCounterPeriod - how long for one tick
	TxsCounterPeriod = 6 * time.Second
	// TxsCounterBufLen - how many ticks
	TxsCounterBufLen = 60
	// OnlineImmunityMinutes -
	OnlineImmunityMinutes = int(TxsCounterBufLen * TxsCounterPeriod / time.Minute)
	// PeerTickPeriod - run the peer's tick not more often than this
	PeerTickPeriod = 100 * time.Millisecond
	// InvsFlushPeriod - send all the pending invs to the peer not more often than this
	InvsFlushPeriod = 10 * time.Millisecond
	// MaxGetmpTxs -
	MaxGetmpTxs = 100e3
)

var (
	// MutexNet -
	MutexNet sync.Mutex
	// OpenCons -
	OpenCons = make(map[uint64]*OneConnection)
	// InConsActive -
	InConsActive uint32
	// OutConsActive -
	OutConsActive uint32
	// LastConnID -
	LastConnID uint32
	nonce      [8]byte

	// HammeringMutex -Hammering protection (peers that keep re-connecting) map IPv4 => UnixTime
	HammeringMutex sync.Mutex
	// RecentlyDisconencted -
	RecentlyDisconencted = make(map[[4]byte]time.Time)
)

// NodeStruct -
type NodeStruct struct {
	Version       uint32
	Services      uint64
	Timestamp     uint64
	Height        uint32
	Agent         string
	DoNotRelayTxs bool
	ReportedIPv4  uint32
	SendHeaders   bool
	Nonce         [8]byte

	// BIP152:
	SendCmpctVer  uint64
	HighBandwidth bool
}

// ConnectionStatus -
type ConnectionStatus struct {
	Incomming                bool
	ConnectedAt              time.Time
	VersionReceived          bool
	LastBtsRcvd, LastBtsSent uint32
	LastCmdRcvd, LastCmdSent string
	LastDataGot              time.Time // if we have no data for some time, we abort this conenction
	OurGetAddrDone           bool      // Whether we shoudl issue another "getaddr"

	AllHeadersReceived   bool // keep sending getheaders until this is not set
	LastHeadersEmpty     bool
	TotalNewHeadersCount int
	GetHeadersInProgress bool
	GetHeadersTimeout    time.Time
	LastHeadersHeightAsk uint32
	GetBlocksDataNow     bool

	LastSent       time.Time
	MaxSentBufSize int

	PingHistory    [PingHistoryLength]int
	PingHistoryIdx int
	InvsRecieved   uint64

	BytesReceived, BytesSent uint64
	Counters                 map[string]uint64

	GetAddrDone bool
	MinFeeSPKB  int64 // BIP 133

	TxsReceived int // During last hour

	IsSpecial bool // Special connections get more debgs and are not being automatically dropped
	IsSpawn   bool

	Authorized bool
	AuthMsgGot uint
	AuthAckGot bool

	LastMinFeePerKByte uint64

	PingSentCnt   uint64
	BlocksExpired uint64
}

// ConnInfo -
type ConnInfo struct {
	ID     uint32
	PeerIP string

	NodeStruct
	ConnectionStatus

	BytesToSend      int
	BlocksInProgress int
	InvsToSend       int
	AveragePing      int
	InvsDone         int
	BlocksReceived   int
	GetMPInProgress  bool

	LocalAddr, RemoteAddr string

	// This one is only set inside webui's hnadler (for sorted connections)
	HasImmunity bool
}

// OneConnection -
type OneConnection struct {
	// Source of this IP:
	*peersdb.PeerAddr
	ConnID uint32

	sync.Mutex // protects concurent access to any fields inside this structure

	broken    bool // flag that the conenction has been broken / shall be disconnected
	banit     bool // Ban this client after disconnecting
	misbehave int  // When it reaches 1000, ban it

	net.Conn

	// TCP connection data:
	X ConnectionStatus

	Node NodeStruct // Data from the version message

	// Messages reception state machine:
	recv struct {
		hdr    [24]byte
		hdrLen int
		plLen  uint32 // length taken from the message header
		cmd    string
		dat    []byte
		datlen uint32
	}
	LastMsgTime time.Time

	InvDone struct {
		Map     map[uint64]uint32
		History []uint64
		Idx     int
	}

	// Message sending state machine:
	sendBuf                  [SendBufSize]byte
	SendBufProd, SendBufCons int

	// Statistics:
	PendingInvs []*[36]byte // List of pending INV to send and the mutex protecting access to it

	GetBlockInProgress map[BIDX]*oneBlockDl

	// Ping stats
	LastPingSent   time.Time
	PingInProgress []byte

	counters map[string]uint64

	blocksreceived  []time.Time
	nextMaintanence time.Time
	nextGetData     time.Time

	// we need these three below to count txs received only during last hour
	txsCur int
	txsCha chan int
	txsNxt time.Time

	writingThreadDone sync.WaitGroup
	writingThreadPush chan bool

	GetMP chan bool
}

// BIDX -
type BIDX [btc.Uint256IdxLen]byte

type oneBlockDl struct {
	hash          *btc.Uint256
	start         time.Time
	col           *CompactBlockCollector
	SentAtPingCnt uint64
}

// BCmsg -
type BCmsg struct {
	cmd string
	pl  []byte
}

// NewConnection -
func NewConnection(ad *peersdb.PeerAddr) (c *OneConnection) {
	c = new(OneConnection)
	c.PeerAddr = ad
	c.GetBlockInProgress = make(map[BIDX]*oneBlockDl)
	c.ConnID = atomic.AddUint32(&LastConnID, 1)
	c.counters = make(map[string]uint64)
	c.InvDone.Map = make(map[uint64]uint32, MaxInvHistory)
	c.GetMP = make(chan bool, 1)
	return
}

// IncCnt -
func (c *OneConnection) IncCnt(name string, val uint64) {
	c.Mutex.Lock()
	c.counters[name] += val
	c.Mutex.Unlock()
}

// MutexSetBool - mutex protected
func (c *OneConnection) MutexSetBool(addr *bool, val bool) {
	c.Mutex.Lock()
	*addr = val
	c.Mutex.Unlock()
}

// MutexGetBool - mutex protected
func (c *OneConnection) MutexGetBool(addr *bool) (val bool) {
	c.Mutex.Lock()
	val = *addr
	c.Mutex.Unlock()
	return
}

// BytesToSent - call it with locked mutex!
func (c *OneConnection) BytesToSent() int {
	if c.SendBufProd >= c.SendBufCons {
		return c.SendBufProd - c.SendBufCons
	}
	return c.SendBufProd + SendBufSize - c.SendBufCons
}

// GetStats -
func (c *OneConnection) GetStats(res *ConnInfo) {
	c.Mutex.Lock()
	res.ID = c.ConnID
	res.PeerIP = c.PeerAddr.Ip()
	if c.Conn != nil {
		res.LocalAddr = c.Conn.LocalAddr().String()
		res.RemoteAddr = c.Conn.RemoteAddr().String()
	}
	res.NodeStruct = c.Node
	res.ConnectionStatus = c.X
	res.BytesToSend = c.BytesToSent()
	res.BlocksInProgress = len(c.GetBlockInProgress)
	res.InvsToSend = len(c.PendingInvs)
	res.AveragePing = c.GetAveragePing()

	res.Counters = make(map[string]uint64, len(c.counters))
	for k, v := range c.counters {
		res.Counters[k] = v
	}

	res.InvsDone = len(c.InvDone.History)
	res.BlocksReceived = len(c.blocksreceived)
	res.GetMPInProgress = len(c.GetMP) != 0

	c.Mutex.Unlock()
}

// SendRawMsg -
func (c *OneConnection) SendRawMsg(cmd string, pl []byte) (e error) {
	c.Mutex.Lock()
	if !c.broken {
		// we never allow the buffer to be totally full because then producer would be equal consumer
		if bytesLeft := SendBufSize - c.BytesToSent(); bytesLeft <= len(pl)+24 {
			c.Mutex.Unlock()
			/*println(c.PeerAddr.Ip(), c.Node.Version, c.Node.Agent, "Peer Send Buffer Overflow @",
			cmd, bytesLeft, len(pl)+24, c.SendBufProd, c.SendBufCons, c.BytesToSent())*/
			c.Disconnect("SendBufferOverflow")
			common.CountSafe("PeerSendOverflow")
			return errors.New("Send buffer overflow")
		}

		c.counters["sent_"+cmd]++
		c.counters["sbts_"+cmd] += uint64(len(pl))

		common.CountSafe("sent_" + cmd)
		common.CountSafeAdd("sbts_"+cmd, uint64(len(pl)))
		var sbuf [24]byte

		c.X.LastCmdSent = cmd
		c.X.LastBtsSent = uint32(len(pl))

		binary.LittleEndian.PutUint32(sbuf[0:4], common.Version)
		copy(sbuf[0:4], common.Magic[:])
		copy(sbuf[4:16], cmd)
		binary.LittleEndian.PutUint32(sbuf[16:20], uint32(len(pl)))

		sh := btc.Sha2Sum(pl[:])
		copy(sbuf[20:24], sh[:4])

		c.appendToSendBuffer(sbuf[:])
		c.appendToSendBuffer(pl)

		if x := c.BytesToSent(); x > c.X.MaxSentBufSize {
			c.X.MaxSentBufSize = x
		}
	}
	c.Mutex.Unlock()
	select {
	case c.writingThreadPush <- true:
	default:
	}
	return
}

// appendToSendBuffer - this function assumes that there is enough room inside sendBuf
func (c *OneConnection) appendToSendBuffer(d []byte) {
	roomLeft := SendBufSize - c.SendBufProd
	if roomLeft >= len(d) {
		copy(c.sendBuf[c.SendBufProd:], d)
		roomLeft = c.SendBufProd + len(d)
	} else {
		copy(c.sendBuf[c.SendBufProd:], d[:roomLeft])
		copy(c.sendBuf[:], d[roomLeft:])
	}
	c.SendBufProd = (c.SendBufProd + len(d)) & SendBufMask
}

// Disconnect -
func (c *OneConnection) Disconnect(why string) {
	c.Mutex.Lock()
	/*if c.X.IsSpecial {
		print("Disconnect " + c.PeerAddr.Ip() + " (" + c.Node.Agent + ") because " + why + "\n> ")
	}*/
	c.broken = true
	c.Mutex.Unlock()
}

// IsBroken -
func (c *OneConnection) IsBroken() (res bool) {
	c.Mutex.Lock()
	res = c.broken
	c.Mutex.Unlock()
	return
}

// DoS -
func (c *OneConnection) DoS(why string) {
	common.CountSafe("Ban" + why)
	c.Mutex.Lock()
	if c.X.IsSpecial {
		print("BAN " + c.PeerAddr.Ip() + " (" + c.Node.Agent + ") because " + why + "\n> ")
	}
	c.banit = true
	c.broken = true
	c.Mutex.Unlock()
}

// Misbehave -
func (c *OneConnection) Misbehave(why string, howMuch int) (res bool) {
	c.Mutex.Lock()
	if c.X.IsSpecial {
		print("Misbehave " + c.PeerAddr.Ip() + " (" + c.Node.Agent + ") because " + why + "\n> ")
	}
	if !c.banit {
		common.CountSafe("Bad" + why)
		c.misbehave += howMuch
		if c.misbehave >= 1000 {
			common.CountSafe("BanMisbehave")
			res = true
			c.banit = true
			c.broken = true
			//print("Ban " + c.PeerAddr.Ip() + " (" + c.Node.Agent + ") because " + why + "\n> ")
		}
	}
	c.Mutex.Unlock()
	return
}

// HandleError -
func (c *OneConnection) HandleError(e error) error {
	if nerr, ok := e.(net.Error); ok && nerr.Timeout() {
		//fmt.Println("Just a timeout - ignore")
		return nil
	}
	c.recv.hdrLen = 0
	c.recv.dat = nil
	c.Disconnect("Error:" + e.Error())
	return e
}

// FetchMessage -
func (c *OneConnection) FetchMessage() (ret *BCmsg, timeoutOrData bool) {
	var e error
	var n int

	for c.recv.hdrLen < 24 {
		n, e = common.SockRead(c.Conn, c.recv.hdr[c.recv.hdrLen:24])
		if n < 0 {
			n = 0
		} else {
			timeoutOrData = true
		}
		c.Mutex.Lock()
		if n > 0 {
			c.X.BytesReceived += uint64(n)
			c.X.LastDataGot = time.Now()
			c.recv.hdrLen += n
		}
		if e != nil {
			c.Mutex.Unlock()
			c.HandleError(e)
			return // Make sure to exit here, in case of timeout
		}
		if c.recv.hdrLen >= 4 && !bytes.Equal(c.recv.hdr[:4], common.Magic[:]) {
			if c.X.IsSpecial {
				fmt.Printf("BadMagic from %s %s \n hdr:%s  n:%d\n R: %s %d / S: %s %d\n> ", c.PeerAddr.Ip(), c.Node.Agent,
					hex.EncodeToString(c.recv.hdr[:c.recv.hdrLen]), n,
					c.X.LastCmdRcvd, c.X.LastBtsRcvd, c.X.LastCmdSent, c.X.LastBtsSent)
			}
			c.Mutex.Unlock()
			common.CountSafe("NetBadMagic")
			c.Disconnect("BadMagic")
			return
		}
		if c.broken {
			c.Mutex.Unlock()
			return
		}
		if c.recv.hdrLen == 24 {
			c.recv.plLen = binary.LittleEndian.Uint32(c.recv.hdr[16:20])
			c.recv.cmd = strings.TrimRight(string(c.recv.hdr[4:16]), "\000")
			c.Mutex.Unlock()
		} else {
			if c.recv.hdrLen > 24 {
				panic("c.recv.hdrLen > 24")
			}
			c.Mutex.Unlock()
			return
		}
	}

	if c.recv.plLen > 0 {
		if c.recv.dat == nil {
			msi := maxMsgSize(c.recv.cmd)
			if c.recv.plLen > msi {
				c.DoS("Big-" + c.recv.cmd)
				return
			}
			c.Mutex.Lock()
			c.recv.dat = make([]byte, c.recv.plLen)
			c.recv.datlen = 0
			c.Mutex.Unlock()
		}
		if c.recv.datlen < c.recv.plLen {
			n, e = common.SockRead(c.Conn, c.recv.dat[c.recv.datlen:])
			if n < 0 {
				n = 0
			} else {
				timeoutOrData = true
			}
			if n > 0 {
				c.Mutex.Lock()
				c.X.BytesReceived += uint64(n)
				c.recv.datlen += uint32(n)
				c.Mutex.Unlock()
				if c.recv.datlen > c.recv.plLen {
					println(c.PeerAddr.Ip(), "is sending more of", c.recv.cmd, "then it should have", c.recv.datlen, c.recv.plLen)
					c.DoS("MsgSizeMismatch")
					return
				}
			}
			if e != nil {
				c.HandleError(e)
				return
			}
			if c.MutexGetBool(&c.broken) || c.recv.datlen < c.recv.plLen {
				return
			}
		}
	}

	sh := btc.Sha2Sum(c.recv.dat)
	if !bytes.Equal(c.recv.hdr[20:24], sh[:4]) {
		//println(c.PeerAddr.Ip(), "Msg checksum error")
		c.DoS("MsgBadChksum")
		return
	}

	ret = new(BCmsg)
	ret.cmd = c.recv.cmd
	ret.pl = c.recv.dat

	c.Mutex.Lock()
	c.recv.hdrLen = 0
	c.recv.cmd = ""
	c.recv.dat = nil
	c.Mutex.Unlock()

	c.LastMsgTime = time.Now()

	return
}

// GetMPNow -
func (c *OneConnection) GetMPNow() {
	if c.X.Authorized && common.GetBool(&common.CFG.TXPool.Enabled) {
		select {
		case c.GetMP <- true:
		default:
			fmt.Println(c.ConnID, "GetMP channel full")
		}
	}
}

// writingThread -
func (c *OneConnection) writingThread() {
	for !c.IsBroken() {
		c.Mutex.Lock() // protect access to c.SendBufProd

		if c.SendBufProd == c.SendBufCons {
			c.Mutex.Unlock()
			// wait for a new write, but time out just in case
			select {
			case <-c.writingThreadPush:
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}

		bytesToSend := c.SendBufProd - c.SendBufCons
		c.Mutex.Unlock() // unprotect access to c.SendBufProd

		if bytesToSend < 0 {
			bytesToSend += SendBufSize
		}
		if c.SendBufCons+bytesToSend > SendBufSize {
			bytesToSend = SendBufSize - c.SendBufCons
		}

		n, e := common.SockWrite(c.Conn, c.sendBuf[c.SendBufCons:c.SendBufCons+bytesToSend])
		if n > 0 {
			c.Mutex.Lock()
			c.X.LastSent = time.Now()
			c.X.BytesSent += uint64(n)
			c.SendBufCons = (c.SendBufCons + n) & SendBufMask
			c.Mutex.Unlock()
		} else if e != nil {
			c.Disconnect("SendErr:" + e.Error())
		} else if n < 0 {
			// It comes here if we could not send a single byte because of BW limit
			time.Sleep(10 * time.Millisecond)
		}
	}
	c.writingThreadDone.Done()
}

// ConnectionActive -
func ConnectionActive(ad *peersdb.PeerAddr) (yes bool) {
	MutexNet.Lock()
	_, yes = OpenCons[ad.UniqID()]
	MutexNet.Unlock()
	return
}

// Returns maximum accepted payload size of a given type of message
func maxMsgSize(cmd string) uint32 {
	switch cmd {
	case "inv":
		return 3 + 50000*36 // the spec says "max 50000 entries"
	case "tx":
		return 500e3 // max segwit tx size 500KB
	case "addr":
		return 3 + 1000*30 // max 1000 addrs
	case "block":
		return 8e6 // max seg2x block size 8MB
	case "getblocks":
		return 4 + 3 + 500*32 + 32 // we allow up to 500 locator hashes
	case "getdata":
		return 3 + 50000*36 // the spec says "max 50000 entries"
	case "headers":
		return 3 + 50000*36 // the spec says "max 50000 entries"
	case "getheaders":
		return 4 + 3 + 500*32 + 32 // we allow up to 500 locator hashes
	case "cmpctblock":
		return 1e6 // 1MB shall be enough
	case "getblocktxn":
		return 1e6 // 1MB shall be enough
	case "blocktxn":
		return 8e6 // all txs that can fit withing 1MB block
	case "notfound":
		return 3 + 50000*36 // maximum size of getdata
	case "getmp":
		return 5 + 8*MaxGetmpTxs
	default:
		return 1024 // Any other type of block: maximum 1KB payload limit
	}
}

// NetCloseAll -
func NetCloseAll() {
	sta := time.Now()
	println("Closing network")
	common.NetworkClosed.Set()
	common.SetBool(&common.ListenTCP, false)
	MutexNet.Lock()
	if InConsActive > 0 || OutConsActive > 0 {
		for _, v := range OpenCons {
			v.Disconnect("CloseAll")
		}
	}
	MutexNet.Unlock()
	time.Sleep(1e9) // give one second for WebUI requests to complete
	// now wait for all the connections to close
	for {
		MutexNet.Lock()
		allDone := len(OpenCons) == 0
		MutexNet.Unlock()
		if allDone {
			return
		}
		if time.Now().Sub(sta) > 2*time.Second {
			MutexNet.Lock()
			fmt.Println("Still have open connections:", InConsActive, OutConsActive, len(OpenCons), "- please report")
			MutexNet.Unlock()
			break
		}
		time.Sleep(1e7)
	}
	for TCPServerStarted {
		time.Sleep(1e7) // give one second for all the pending messages to get processed
	}
}

// DropPeer -
func DropPeer(conid uint32) {
	MutexNet.Lock()
	defer MutexNet.Unlock()
	for _, v := range OpenCons {
		if uint32(conid) == v.ConnID {
			v.DoS("FromUI")
			//fmt.Println("The connection with", v.PeerAddr.Ip(), "is being dropped and the peer is banned")
			return
		}
	}
	fmt.Println("DropPeer: There is no such an active connection", conid)
}

// GetMP -
func GetMP(conid uint32) {
	MutexNet.Lock()
	for _, v := range OpenCons {
		if uint32(conid) == v.ConnID {
			MutexNet.Unlock()
			v.GetMPNow()
			return
		}
	}
	MutexNet.Unlock()
	fmt.Println("GetMP: There is no such an active connection", conid)
}

// BlocksToGetCnt -
func BlocksToGetCnt() (res int) {
	MutexRcv.Lock()
	res = len(BlocksToGet)
	MutexRcv.Unlock()
	return
}

func init() {
	rand.Read(nonce[:])
}
