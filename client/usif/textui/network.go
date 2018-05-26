package textui

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/client/network"
	"github.com/ParallelCoinTeam/duod/lib/others/peersdb"
)

// SortedKeys -
type SortedKeys []struct {
	Key    uint64
	ConnID uint32
}

// Len -
func (sk SortedKeys) Len() int {
	return len(sk)
}

// Less -
func (sk SortedKeys) Less(a, b int) bool {
	return sk[a].ConnID < sk[b].ConnID
}

// Swap -
func (sk SortedKeys) Swap(a, b int) {
	sk[a], sk[b] = sk[b], sk[a]
}

func netDrop(par string) {
	conid, e := strconv.ParseUint(par, 10, 32)
	if e != nil {
		println(e.Error())
		return
	}
	network.DropPeer(uint32(conid))
}

func nodeInfo(par string) {
	conid, e := strconv.ParseUint(par, 10, 32)
	if e != nil {
		return
	}

	var r *network.ConnInfo

	network.MutexNet.Lock()

	for _, v := range network.OpenCons {
		if uint32(conid) == v.ConnID {
			r = new(network.ConnInfo)
			v.GetStats(r)
			break
		}
	}
	network.MutexNet.Unlock()

	if r == nil {
		return
	}

	fmt.Printf("Connection ID %d:\n", r.ID)
	if r.Incomming {
		fmt.Println("Comming from", r.PeerIP)
	} else {
		fmt.Println("Going to", r.PeerIP)
	}
	if !r.ConnectedAt.IsZero() {
		fmt.Println("Connected at", r.ConnectedAt.Format("2006-01-02 15:04:05"))
		if r.Version != 0 {
			fmt.Println("Node Version:", r.Version, "/ Services:", fmt.Sprintf("0x%x", r.Services))
			fmt.Println("User Agent:", r.Agent)
			fmt.Println("Chain Height:", r.Height)
			fmt.Printf("Reported IP: %d.%d.%d.%d\n", byte(r.ReportedIPv4>>24), byte(r.ReportedIPv4>>16),
				byte(r.ReportedIPv4>>8), byte(r.ReportedIPv4))
			fmt.Println("SendHeaders:", r.SendHeaders)
		}
		fmt.Println("Invs Done:", r.InvsDone)
		fmt.Println("Last data got:", time.Now().Sub(r.LastDataGot).String())
		fmt.Println("Last data sent:", time.Now().Sub(r.LastSent).String())
		fmt.Println("Last command received:", r.LastCmdRcvd, " ", r.LastBtsRcvd, "bytes")
		fmt.Println("Last command sent:", r.LastCmdSent, " ", r.LastBtsSent, "bytes")
		fmt.Print("Invs  Recieved:", r.InvsRecieved, "  Pending:", r.InvsToSend, "\n")
		fmt.Print("Bytes to send:", r.BytesToSend, " (", r.MaxSentBufSize, " max)\n")
		fmt.Print("BlockInProgress:", r.BlocksInProgress, "  GetHeadersInProgress:", r.GetHeadersInProgress, "\n")
		fmt.Println("GetBlocksDataNow:", r.GetBlocksDataNow)
		fmt.Println("AllHeadersReceived:", r.AllHeadersReceived)
		fmt.Println("Total Received:", r.BytesReceived, " /  Sent:", r.BytesSent)
		for k, v := range r.Counters {
			fmt.Println(k, ":", v)
		}
	} else {
		fmt.Println("Not yet connected")
	}
}

func netConn(par string) {
	ad, er := peersdb.NewAddrFromString(par, false)
	if er != nil {
		fmt.Println(par, er.Error())
		return
	}
	fmt.Println("Connecting to", ad.IP())
	ad.Manual = true
	network.DoNetwork(ad)
}

func netStats(par string) {
	if par == "bw" {
		common.PrintBWStats()
		return
	} else if par != "" {
		nodeInfo(par)
		return
	}

	network.MutexNet.Lock()
	fmt.Printf("%d active net connections, %d outgoing\n", len(network.OpenCons), network.OutConsActive)
	srt := make(SortedKeys, len(network.OpenCons))
	cnt := 0
	for k, v := range network.OpenCons {
		srt[cnt].Key = k
		srt[cnt].ConnID = v.ConnID
		cnt++
	}
	sort.Sort(srt)
	for idx := range srt {
		v := network.OpenCons[srt[idx].Key]
		v.Mutex.Lock()
		fmt.Printf("%8d) ", v.ConnID)

		if v.X.Incomming {
			fmt.Print("<- ")
		} else {
			fmt.Print(" ->")
		}
		fmt.Printf(" %21s %5dms %7d : %-16s %7d : %-16s", v.PeerAddr.IP(),
			v.GetAveragePing(), v.X.LastBtsRcvd, v.X.LastCmdRcvd, v.X.LastBtsSent, v.X.LastCmdSent)
		fmt.Printf("%9s %9s", common.BytesToString(v.X.Counters["BytesReceived"]), common.BytesToString(v.X.Counters["BytesSent"]))
		fmt.Print("  ", v.Node.Agent)

		if b2s := v.BytesToSent(); b2s > 0 {
			fmt.Print("  ", b2s)
		}
		v.Mutex.Unlock()
		fmt.Println()
	}

	if network.ExternalAddrLen() > 0 {
		fmt.Print("External addresses:")
		network.ExternalIPmutex.Lock()
		for ip, cnt := range network.ExternalIP4 {
			fmt.Printf(" %d.%d.%d.%d(%d)", byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip), cnt)
		}
		network.ExternalIPmutex.Unlock()
		fmt.Println()
	} else {
		fmt.Println("No known external address")
	}

	network.MutexNet.Unlock()

	fmt.Print("RecentlyDisconencted:")
	network.HammeringMutex.Lock()
	for ip, ti := range network.RecentlyDisconencted {
		fmt.Printf(" %d.%d.%d.%d-%s", ip[0], ip[1], ip[2], ip[3], time.Now().Sub(ti).String())
	}
	network.HammeringMutex.Unlock()
	fmt.Println()

	fmt.Println("GetMPInProgress:", len(network.GetMPInProgressTicket) != 0)

	common.PrintBWStats()
}

func init() {
	newUI("net n", false, netStats, "Show network statistics. Specify ID to see its details.")
	newUI("drop", false, netDrop, "Disconenct from node with a given IP")
	newUI("conn", false, netConn, "Connect to the given node (specify IP and optionally a port)")
}
