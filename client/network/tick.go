//Package network -
package network

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/L"
	"github.com/ParallelCoinTeam/duod/lib/others/peersdb"
)

var (
	// TCPServerStarted -
	TCPServerStarted bool
	nextDropPeer     time.Time
	nextCleanHammers time.Time
	// NextConnectFriends -
	NextConnectFriends = time.Now()
	// AuthPubkeys -
	AuthPubkeys [][]byte
	// GetMPInProgressTicket -
	GetMPInProgressTicket = make(chan bool, 1)
)

// ExpireBlocksToGet -
func (c *OneConnection) ExpireBlocksToGet(now *time.Time, currPingCount uint64) {
	MutexRcv.Lock()
	for k, v := range c.GetBlockInProgress {
		if currPingCount > v.SentAtPingCnt {
			common.CountSafe("BlockInprogNotfound")
			c.counters["BlockTotFound"]++
		} else if now != nil && now.After(v.start.Add(5*time.Minute)) {
			common.CountSafe("BlockInprogTimeout")
			c.counters["BlockTimeout"]++
		} else {
			continue
		}
		c.X.BlocksExpired++
		delete(c.GetBlockInProgress, k)
		if bip, ok := BlocksToGet[k]; ok {
			bip.InProgress--
		}
	}
	MutexRcv.Unlock()
}

// Maintanence - Call this once a minute
func (c *OneConnection) Maintanence(now time.Time) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	// Expire GetBlockInProgress after five minutes, if they are not in BlocksToGet
	c.ExpireBlocksToGet(&now, 0)

	// Expire BlocksReceived after two days
	if len(c.blocksreceived) > 0 {
		var i int
		for i = 0; i < len(c.blocksreceived); i++ {
			if c.blocksreceived[i].Add(common.GetDuration(&common.BlockExpireEvery)).After(now) {
				break
			}
			common.CountSafe("BlksRcvdExpired")
		}
		if i > 0 {
			//println(c.ConnID, "expire", i, "block(s)")
			c.blocksreceived = c.blocksreceived[i:]
		}
	}
}

// Tick -
func (c *OneConnection) Tick(now time.Time) {
	if !c.X.VersionReceived {
		// Wait only certain amount of time for the version message
		if c.X.ConnectedAt.Add(VersionMsgTimeout).Before(now) {
			c.Disconnect("VersionTimeout")
			common.CountSafe("NetVersionTout")
			return
		}
		// If we have no ack, do nothing more.
		return
	}

	if c.MutexGetBool(&c.X.GetHeadersInProgress) && now.After(c.X.GetHeadersTimeout) {
		//println(c.ConnID, "- GetHdrs Timeout")
		c.Disconnect("HeadersTimeout")
		common.CountSafe("NetHeadersTout")
		return
	}

	if common.GetBool(&common.BlockChainSynchronized) {
		// See if to send "getmp" command
		select {
		case GetMPInProgressTicket <- true:
			// ticket received - check for the request...
			if len(c.GetMP) == 0 || c.SendGetMP() != nil {
				// no request for "getmp" here or sending failed - clear the global flag/channel
				_ = <-GetMPInProgressTicket
			}
		default:
			// failed to get the ticket - just do nothing
		}
	}

	// Tick the recent transactions counter
	if now.After(c.txsNxt) {
		c.Mutex.Lock()
		if len(c.txsCha) == cap(c.txsCha) {
			tmp := <-c.txsCha
			c.X.TxsReceived -= tmp
		}
		c.txsCha <- c.txsCur
		c.txsCur = 0
		c.txsNxt = c.txsNxt.Add(TxsCounterPeriod)
		c.Mutex.Unlock()
	}

	if mfpb := common.MinFeePerKB(); mfpb != c.X.LastMinFeePerKByte {
		c.X.LastMinFeePerKByte = mfpb
		if c.Node.Version >= 70013 {
			c.SendFeeFilter()
		}
	}

	if now.After(c.nextMaintanence) {
		c.Maintanence(now)
		c.nextMaintanence = now.Add(MaintenancePeriod)
	}

	// Ask node for new addresses...?
	if !c.X.OurGetAddrDone && peersdb.PeerDB.Count() < common.MaxPeersNeeded {
		common.CountSafe("AddrWanted")
		c.SendRawMsg("getaddr", nil)
		c.X.OurGetAddrDone = true
	}

	c.Mutex.Lock()
	if !c.X.GetHeadersInProgress && !c.X.AllHeadersReceived && len(c.GetBlockInProgress) == 0 {
		c.Mutex.Unlock()
		c.sendGetHeaders()
		c.Mutex.Lock()
	} else {
		if c.X.AllHeadersReceived {
			if !c.X.GetBlocksDataNow && now.After(c.nextGetData) {
				c.X.GetBlocksDataNow = true
			}
			if c.X.GetBlocksDataNow {
				c.X.GetBlocksDataNow = false
				c.Mutex.Unlock()
				c.GetBlockData()
				c.Mutex.Lock()
			}
		}
	}

	if !c.X.GetHeadersInProgress && len(c.GetBlockInProgress) == 0 {
		c.Mutex.Unlock()
		// Ping if we dont do anything
		c.TryPing()
	} else {
		c.Mutex.Unlock()
	}
}

// DoNetwork -
func DoNetwork(ad *peersdb.PeerAddr) {
	conn := NewConnection(ad)
	MutexNet.Lock()
	if _, ok := OpenCons[ad.UniqID()]; ok {
		common.CountSafe("ConnectingAgain")
		MutexNet.Unlock()
		return
	}
	if ad.Friend || ad.Manual {
		conn.MutexSetBool(&conn.X.IsSpecial, true)
	}
	OpenCons[ad.UniqID()] = conn
	OutConsActive++
	MutexNet.Unlock()
	go func() {
		var con net.Conn
		var e error
		connDone := make(chan bool, 1)

		go func(addr string) {
			// we do net.Dial() in paralell routine, so we can abort quickly upon request
			con, e = net.DialTimeout("tcp4", addr, TCPDialTimeout)
			connDone <- true
		}(fmt.Sprintf("%d.%d.%d.%d:%d", ad.IPv4[0], ad.IPv4[1], ad.IPv4[2], ad.IPv4[3], ad.Port))

		for {
			select {
			case <-connDone:
				if e == nil {
					MutexNet.Lock()
					conn.Conn = con
					conn.X.ConnectedAt = time.Now()
					MutexNet.Unlock()
					conn.Run()
				}
			case <-time.After(10 * time.Millisecond):
				if !conn.IsBroken() {
					continue
				}
			}
			break
		}

		MutexNet.Lock()
		delete(OpenCons, ad.UniqID())
		OutConsActive--
		MutexNet.Unlock()
		ad.Dead()
	}()
}

// TCP server
func tcpServer() {
	ad, e := net.ResolveTCPAddr("tcp4", fmt.Sprint("0.0.0.0:", common.DefaultTCPport()))
	if e != nil {
		L.Debug("ResolveTCPAddr", e.Error())
		return
	}

	lis, e := net.ListenTCP("tcp4", ad)
	if e != nil {
		println("ListenTCP", e.Error())
		return
	}
	defer lis.Close()

	L.Debug("TCP server started at ", ad.String())

	for common.IsListenTCP() {
		common.CountSafe("NetServerLoops")
		MutexNet.Lock()
		ica := InConsActive
		MutexNet.Unlock()
		if ica < common.GetUint32(&common.CFG.Net.MaxInCons) {
			lis.SetDeadline(time.Now().Add(100 * time.Millisecond))
			tc, e := lis.AcceptTCP()
			if e == nil && common.IsListenTCP() {
				var terminate bool

				// set port to default, for incomming connections
				ad, e := peersdb.NewPeerFromString(tc.RemoteAddr().String(), true)
				if e == nil {
					// Hammering protection
					HammeringMutex.Lock()
					ti, ok := RecentlyDisconencted[ad.NetAddr.IPv4]
					HammeringMutex.Unlock()
					if ok && time.Now().Sub(ti) < HammeringMinReconnect {
						//println(ad.IP(), "is hammering within", time.Now().Sub(ti).String())
						common.CountSafe("BanHammerIn")
						ad.Ban()
						terminate = true
					}

					if !terminate {
						// Incoming IP passed all the initial checks - talk to it
						conn := NewConnection(ad)
						conn.X.ConnectedAt = time.Now()
						conn.X.Incomming = true
						conn.Conn = tc
						MutexNet.Lock()
						if _, ok := OpenCons[ad.UniqID()]; ok {
							L.Debug(ad.IP(), "already connected")
							common.CountSafe("SameIpReconnect")
							MutexNet.Unlock()
							terminate = true
						} else {
							OpenCons[ad.UniqID()] = conn
							InConsActive++
							MutexNet.Unlock()
							go func() {
								conn.Run()
								MutexNet.Lock()
								delete(OpenCons, ad.UniqID())
								InConsActive--
								MutexNet.Unlock()
							}()
						}
					}
				} else {
					common.CountSafe("InConnRefused")
					terminate = true
				}

				// had any error occured - close teh TCP connection
				if terminate {
					tc.Close()
				}
			}
		} else {
			time.Sleep(1e8)
		}
	}
	MutexNet.Lock()
	for _, c := range OpenCons {
		if c.X.Incomming {
			c.Disconnect("CloseAllIn")
		}
	}
	TCPServerStarted = false
	MutexNet.Unlock()
	L.Debug("TCP server stopped")
}

// ConnectFriends -
func ConnectFriends() {
	common.CountSafe("ConnectFriends")

	f, _ := os.Open(common.DuodHomeDir + "friends.txt")
	if f == nil {
		return
	}
	defer f.Close()

	AuthPubkeys = nil
	friendIDs := make(map[uint64]bool)

	rd := bufio.NewReader(f)
	if rd != nil {
		for {
			ln, _, er := rd.ReadLine()
			if er != nil {
				break
			}
			ls := strings.SplitN(strings.Trim(string(ln), "\r\n\t"), " ", 2)
			ad, _ := peersdb.NewAddrFromString(ls[0], false)
			if ad != nil {
				MutexNet.Lock()
				curr, _ := OpenCons[ad.UniqID()]
				MutexNet.Unlock()
				if curr == nil {
					//print("Connecting friend ", ad.IP(), " ...\n> ")
					ad.Friend = true
					DoNetwork(ad)
				} else {
					curr.Mutex.Lock()
					curr.PeerAddr.Friend = true
					curr.X.IsSpecial = true
					curr.Mutex.Unlock()
				}
				friendIDs[ad.UniqID()] = true
				continue
			}
			pk := btc.DecodeBase58(ls[0])
			if len(pk) == 33 {
				AuthPubkeys = append(AuthPubkeys, pk)
				//println("Using pubkey:", hex.EncodeToString(pk))
			}
		}
	}

	// Unmark those that are not longer friends
	MutexNet.Lock()
	for _, v := range OpenCons {
		v.Lock()
		if v.PeerAddr.Friend && !friendIDs[v.PeerAddr.UniqID()] {
			v.PeerAddr.Friend = false
			if !v.PeerAddr.Manual {
				v.X.IsSpecial = false
			}
		}
		v.Unlock()
	}
	MutexNet.Unlock()
}

// Ticking - Start listener
func Ticking() {
	if common.IsListenTCP() {
		if !TCPServerStarted {
			TCPServerStarted = true
			go tcpServer()
		}
	}

	now := time.Now()

	// Push GetHeaders if not in progress
	MutexNet.Lock()
	var countHeadersInProgress int
	var maxHeadersGotCount int
	var _v *OneConnection
	for _, v := range OpenCons {
		v.Mutex.Lock() // TODO: Sometimes it might hang here - check why!!
		if !v.X.AllHeadersReceived || v.X.GetHeadersInProgress {
			countHeadersInProgress++
		} else if !v.X.LastHeadersEmpty {
			if _v == nil || v.X.TotalNewHeadersCount > maxHeadersGotCount {
				maxHeadersGotCount = v.X.TotalNewHeadersCount
				_v = v
			}
		}
		v.Mutex.Unlock()
	}
	connCount := OutConsActive
	MutexNet.Unlock()

	if countHeadersInProgress == 0 {
		if _v != nil {
			common.CountSafe("GetHeadersPush")
			L.Debug("No headers_in_progress, so take it from", _v.ConnID,
				_v.X.TotalNewHeadersCount, _v.X.LastHeadersEmpty)
			_v.Mutex.Lock()
			_v.X.AllHeadersReceived = false
			_v.Mutex.Unlock()
		} else {
			common.CountSafe("GetHeadersNone")
		}
	}

	if common.CFG.DropPeers.DropEachMinutes != 0 {
		if nextDropPeer.IsZero() {
			nextDropPeer = now.Add(common.GetDuration(&common.DropSlowestEvery))
		} else if now.After(nextDropPeer) {
			if dropWorstPeer() {
				nextDropPeer = now.Add(common.GetDuration(&common.DropSlowestEvery))
			} else {
				// If no peer dropped this time, try again sooner
				nextDropPeer = now.Add(common.GetDuration(&common.DropSlowestEvery) >> 2)
			}
		}
	}

	// hammering protection - expire recently disconnected
	if nextCleanHammers.IsZero() {
		nextCleanHammers = now.Add(HammeringMinReconnect)
	} else if now.After(nextCleanHammers) {
		HammeringMutex.Lock()
		for k, t := range RecentlyDisconencted {
			if now.Sub(t) >= HammeringMinReconnect {
				delete(RecentlyDisconencted, k)
			}
		}
		HammeringMutex.Unlock()
		nextCleanHammers = now.Add(HammeringMinReconnect)
	}

	// Connect friends
	MutexNet.Lock()
	if now.After(NextConnectFriends) {
		MutexNet.Unlock()
		ConnectFriends()
		MutexNet.Lock()
		NextConnectFriends = now.Add(15 * time.Minute)
	}
	MutexNet.Unlock()

	for connCount < common.GetUint32(&common.CFG.Net.MaxOutCons) {
		var segwitConns uint32
		if common.CFG.Net.MinSegwitCons > 0 {
			MutexNet.Lock()
			for _, cc := range OpenCons {
				cc.Mutex.Lock()
				if (cc.Node.Services & ServiceSegwit) != 0 {
					segwitConns++
				}
				cc.Mutex.Unlock()
			}
			MutexNet.Unlock()
		}

		adrs := peersdb.GetBestPeers(128, func(ad *peersdb.PeerAddr) bool {
			if segwitConns < common.CFG.Net.MinSegwitCons && (ad.Services&ServiceSegwit) == 0 {
				return true
			}
			return ConnectionActive(ad)
		})
		if len(adrs) == 0 && segwitConns < common.CFG.Net.MinSegwitCons {
			// we have only non-segwit peers in the database - take them
			adrs = peersdb.GetBestPeers(128, func(ad *peersdb.PeerAddr) bool {
				return ConnectionActive(ad)
			})
		}
		if len(adrs) == 0 {
			common.LockCfg()
			common.UnlockCfg()
			break
		}
		DoNetwork(adrs[rand.Int31n(int32(len(adrs)))])
		MutexNet.Lock()
		connCount = OutConsActive
		MutexNet.Unlock()
	}

	if expireTxsNow {
		ExpireTxs()
	} else if now.After(lastTxsExpire.Add(time.Minute)) {
		expireTxsNow = true
	}
}

// SendFeeFilter -
func (c *OneConnection) SendFeeFilter() {
	var pl [8]byte
	binary.LittleEndian.PutUint64(pl[:], c.X.LastMinFeePerKByte)
	c.SendRawMsg("feefilter", pl[:])
}

// SendAuth -
func (c *OneConnection) SendAuth() {
	rnd := make([]byte, 32)
	copy(rnd, c.Node.Nonce[:])
	r, s, er := btc.EcdsaSign(common.SecretKey, rnd)
	if er != nil {
		L.Debug(er.Error())
		return
	}
	var sig btc.Signature
	sig.R.Set(r)
	sig.S.Set(s)
	c.SendRawMsg("auth", sig.Bytes())
}

// AuthRvcd -
func (c *OneConnection) AuthRvcd(pl []byte) {
	if c.X.AuthMsgGot > 0 {
		c.DoS("AuthMsgCnt") // Only allow one auth message per connection (DoS prevention)
		return
	}
	c.X.AuthMsgGot++
	rnd := make([]byte, 32)
	copy(rnd, nonce[:])
	for _, pub := range AuthPubkeys {
		if btc.EcdsaVerify(pub, pl, rnd) {
			c.X.Authorized = true
			c.SendRawMsg("authack", nil)
			return
		}
	}
	c.X.Authorized = false
}

// GetMPDone - call it upon receiving "getmpdone" message or when the peer disconnects
func (c *OneConnection) GetMPDone(pl []byte) {
	if len(c.GetMP) > 0 {
		if len(pl) != 1 || pl[0] == 0 || c.SendGetMP() != nil {
			_ = <-c.GetMP
			if len(GetMPInProgressTicket) > 0 {
				_ = <-GetMPInProgressTicket
			}
		}
	}
}

// Run - Process that handles communication with a single peer
func (c *OneConnection) Run() {
	c.writingThreadPush = make(chan bool, 1)

	c.SendVersion()

	c.Mutex.Lock()
	now := time.Now()
	c.X.LastDataGot = now
	c.nextMaintanence = now.Add(time.Minute)
	c.LastPingSent = now.Add(5*time.Second - common.GetDuration(&common.PingPeerEvery)) // do first ping ~5 seconds from now

	c.txsNxt = now.Add(TxsCounterPeriod)
	c.txsCha = make(chan int, TxsCounterBufLen)

	c.Mutex.Unlock()

	nextTick := now
	nextInvs := now

	c.writingThreadDone.Add(1)
	go c.writingThread()

	for !c.IsBroken() {
		if c.IsBroken() {
			break
		}

		cmd, readTried := c.FetchMessage()

		now = time.Now()
		if c.X.VersionReceived && now.After(nextInvs) {
			c.SendInvs()
			nextInvs = now.Add(InvsFlushPeriod)
		}

		if now.After(nextTick) {
			c.Tick(now)
			nextTick = now.Add(PeerTickPeriod)
		}

		if cmd == nil {
			if !readTried {
				// it will end up here if we did not even try to read anything because of BW limit
				time.Sleep(10 * time.Millisecond)
			}
			continue
		}

		if c.X.VersionReceived {
			c.PeerAddr.Alive()
		}

		c.Mutex.Lock()
		c.counters["rcvd_"+cmd.cmd]++
		c.counters["rbts_"+cmd.cmd] += uint64(len(cmd.pl))
		c.X.LastCmdRcvd = cmd.cmd
		c.X.LastBtsRcvd = uint32(len(cmd.pl))
		c.Mutex.Unlock()

		common.CountSafe("rcvd_" + cmd.cmd)
		common.CountSafeAdd("rbts_"+cmd.cmd, uint64(len(cmd.pl)))

		if cmd.cmd == "version" {
			if c.X.VersionReceived {
				L.Debug("VersionAgain from", c.ConnID, c.PeerAddr.IP(), c.Node.Agent)
				c.Misbehave("VersionAgain", 1000/10)
				break
			}
			er := c.HandleVersion(cmd.pl)
			if er != nil {
				L.Debug("version msg error:", er.Error())
				c.Disconnect("Version:" + er.Error())
				break
			}
			if common.FLAG.Log {
				f, _ := os.OpenFile("conn_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
				if f != nil {
					fmt.Fprintf(f, "%s: New connection. ID:%d  Incomming:%t  Addr:%s  Version:%d  Services:0x%x  Agent:%s\n",
						time.Now().Format("2006-01-02 15:04:05"), c.ConnID, c.X.Incomming,
						c.PeerAddr.IP(), c.Node.Version, c.Node.Services, c.Node.Agent)
					f.Close()
				}
			}
			if c.Node.DoNotRelayTxs {
				c.DoS("SPV")
				break
			}
			c.X.LastMinFeePerKByte = common.MinFeePerKB()

			if c.X.IsDuod {
				c.SendAuth()
			}

			if c.Node.Version >= 70012 {
				c.SendRawMsg("sendheaders", nil)
				if c.Node.Version >= 70013 {
					if c.X.LastMinFeePerKByte != 0 {
						c.SendFeeFilter()
					}
					if c.Node.Version >= 70014 {
						if (c.Node.Services & ServiceSegwit) == 0 {
							// if the node does not support segwit, request compact blocks
							// only if we have not achieved the segwit enforcement moment
							if common.BlockChain.Consensus.EnforceSegwit == 0 ||
								common.Last.BlockHeight() < common.BlockChain.Consensus.EnforceSegwit {
								c.SendRawMsg("sendcmpct", []byte{0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
							}
						} else {
							c.SendRawMsg("sendcmpct", []byte{0x01, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
						}
					}
				}
			}
			c.PeerAddr.Services = c.Node.Services
			c.PeerAddr.Save()

			if common.IsListenTCP() {
				c.SendOwnAddr()
			}
			continue
		}

		switch cmd.cmd {
		case "inv":
			c.ProcessInv(cmd.pl)

		case "tx":
			if common.AcceptTx() {
				c.ParseTxNet(cmd.pl)
			}

		case "addr":
			c.ParseAddr(cmd.pl)

		case "block": //block received
			netBlockReceived(c, cmd.pl)
			c.X.GetBlocksDataNow = true // try to ask for more blocks

		case "getblocks":
			c.GetBlocks(cmd.pl)

		case "getdata":
			c.ProcessGetData(cmd.pl)

		case "getaddr":
			if !c.X.GetAddrDone {
				c.SendAddr()
				c.X.GetAddrDone = true
			} else {
				c.Mutex.Lock()
				c.counters["SecondGetAddr"]++
				c.Mutex.Unlock()
				if c.Misbehave("SecondGetAddr", 1000/20) {
					break
				}
			}

		case "ping":
			re := make([]byte, len(cmd.pl))
			copy(re, cmd.pl)
			c.SendRawMsg("pong", re)

		case "pong":
			c.HandlePong(cmd.pl)

		case "getheaders":
			c.GetHeaders(cmd.pl)

		case "notfound":
			common.CountSafe("NotFound")

		case "headers":
			if c.HandleHeaders(cmd.pl) > 0 {
				c.sendGetHeaders()
			}

		case "sendheaders":
			c.Mutex.Lock()
			c.Node.SendHeaders = true
			c.Mutex.Unlock()

		case "feefilter":
			if len(cmd.pl) >= 8 {
				c.X.MinFeeSPKB = int64(binary.LittleEndian.Uint64(cmd.pl[:8]))
				L.Debug(c.PeerAddr.IP(), c.Node.Agent, "feefilter", c.X.MinFeeSPKB)
			}

		case "sendcmpct":
			if len(cmd.pl) >= 9 {
				version := binary.LittleEndian.Uint64(cmd.pl[1:9])
				c.Mutex.Lock()
				if version > c.Node.SendCmpctVer {
					L.Debug(c.ConnID, "sendcmpct", cmd.pl[0])
					c.Node.SendCmpctVer = version
					c.Node.HighBandwidth = cmd.pl[0] == 1
				} else {
					c.counters[fmt.Sprint("SendCmpctV", version)]++
				}
				c.Mutex.Unlock()
			} else {
				common.CountSafe("SendCmpctErr")
				if len(cmd.pl) != 5 {
					L.Debug(c.ConnID, c.PeerAddr.IP(), c.Node.Agent, "sendcmpct", hex.EncodeToString(cmd.pl))
				}
			}

		case "cmpctblock":
			c.ProcessCompactBlock(cmd.pl)

		case "getblocktxn":
			c.ProcessGetBlockTx(cmd.pl)
			L.Debug(c.ConnID, c.PeerAddr.IP(), c.Node.Agent, "getblocktxn", hex.EncodeToString(cmd.pl))

		case "blocktxn":
			c.ProcessBlockTx(cmd.pl)
			L.Debug(c.ConnID, c.PeerAddr.IP(), c.Node.Agent, "blocktxn", hex.EncodeToString(cmd.pl))

		case "getmp":
			if c.X.Authorized {
				c.ProcessGetMP(cmd.pl)
			}

		case "auth":
			c.AuthRvcd(cmd.pl)
			if c.X.AuthAckGot {
				c.GetMPNow()
			}

		case "authack":
			c.X.AuthAckGot = true
			c.GetMPNow()

		case "getmpdone":
			c.GetMPDone(cmd.pl)

		default:
		}
	}

	c.GetMPDone(nil)

	c.Conn.SetWriteDeadline(time.Now()) // this should cause c.Conn.Write() to terminate
	c.writingThreadDone.Wait()

	c.Mutex.Lock()
	MutexRcv.Lock()
	for k := range c.GetBlockInProgress {
		if rec, ok := BlocksToGet[k]; ok {
			rec.InProgress--
		} else {
			//println("ERROR! Block", bip.hash.String(), "in progress, but not in BlocksToGet")
		}
	}
	MutexRcv.Unlock()

	ban := c.banit
	c.Mutex.Unlock()

	if c.PeerAddr.Friend || c.X.Authorized {
		common.CountSafe(fmt.Sprint("FDisconnect-", ban))
	} else {
		if ban {
			c.PeerAddr.Ban()
			common.CountSafe("PeersBanned")
		} else if c.X.Incomming && !c.MutexGetBool(&c.X.IsSpecial) {
			HammeringMutex.Lock()
			RecentlyDisconencted[c.PeerAddr.NetAddr.IPv4] = time.Now()
			HammeringMutex.Unlock()
		}
	}
	c.Conn.Close()
}
