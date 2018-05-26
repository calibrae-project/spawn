package webui

import (
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/client/network"
	"github.com/ParallelCoinTeam/duod/lib/btc"
)

func pNet(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	netPage := loadTemplate("net.html")

	network.MutexNet.Lock()
	netPage = strings.Replace(netPage, "{LISTEN_TCP}", fmt.Sprint(common.IsListenTCP(), network.TCPServerStarted), 1)
	netPage = strings.Replace(netPage, "{EXTERNAL_ADDR}", btc.NewNetAddr(network.BestExternalAddr()).String(), 1)

	network.MutexNet.Unlock()

	d, _ := ioutil.ReadFile(common.SpawnHomeDir + "friends.txt")
	netPage = strings.Replace(netPage, "{FRIENDS_TXT}", html.EscapeString(string(d)), 1)

	writeHTMLHead(w, r)
	w.Write([]byte(netPage))
	writeHTMLTail(w)
}

func jsonNetCon(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("pkg: %v", r)
			}
			fmt.Println("jsonNetCon recovered:", err.Error())
			fmt.Println(string(debug.Stack()))
		}
	}()

	network.MutexNet.Lock()
	defer network.MutexNet.Unlock()

	netCons := make([]network.ConnInfo, len(network.OpenCons))
	tmp, _, _ := network.GetSortedConnections()
	i := len(netCons)
	for _, v := range tmp {
		i--
		v.Conn.GetStats(&netCons[i])
		netCons[i].HasImmunity = v.MinutesOnline < network.OnlineImmunityMinutes
	}

	bx, er := json.Marshal(netCons)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}

}

func jsonPeersT(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if len(r.Form["id"]) == 0 {
		return
	}

	conid, e := strconv.ParseUint(r.Form["id"][0], 10, 32)
	if e != nil {
		return
	}

	var res *network.ConnInfo

	network.MutexNet.Lock()
	for _, v := range network.OpenCons {
		if uint32(conid) == v.ConnID {
			res = new(network.ConnInfo)
			v.GetStats(res)
			break
		}
	}
	network.MutexNet.Unlock()

	if res != nil {
		bx, er := json.Marshal(&res)
		if er == nil {
			w.Header()["Content-Type"] = []string{"application/json"}
			w.Write(bx)
		} else {
			println(er.Error())
		}
	}
}

func jsonBandwidth(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	type oneExtIP struct {
		IP               string
		Count, Timestamp uint
	}

	var out struct {
		OpenConnsTotal  int
		OpenConnsOut    uint32
		OpenConnsIn     uint32
		DLSpeedNow      uint64
		DLSpeedMax      uint64
		DLTotal         uint64
		ULSpeedNow      uint64
		ULSpeedMax      uint64
		ULTotal         uint64
		ExternalIP      []oneExtIP
		GetMPInProgress bool
	}

	common.LockBw()
	common.TickRecv()
	common.TickSent()
	out.DLSpeedNow = common.GetAvgBW(common.DlBytesPrevSec[:], common.DlBytesPrevSecIdx, 5)
	out.DLSpeedMax = common.DownloadLimit()
	out.DLTotal = common.DlBytesTotal
	out.ULSpeedNow = common.GetAvgBW(common.UlBytesPrevSec[:], common.UlBytesPrevSecIdx, 5)
	out.ULSpeedMax = common.UploadLimit()
	out.ULTotal = common.UlBytesTotal
	common.UnlockBw()

	network.MutexNet.Lock()
	out.OpenConnsTotal = len(network.OpenCons)
	out.OpenConnsOut = network.OutConsActive
	out.OpenConnsIn = network.InConsActive
	network.MutexNet.Unlock()

	arr := network.GetExternalIPs()
	for _, rec := range arr {
		out.ExternalIP = append(out.ExternalIP, oneExtIP{
			IP:    fmt.Sprintf("%d.%d.%d.%d", byte(rec.IP>>24), byte(rec.IP>>16), byte(rec.IP>>8), byte(rec.IP)),
			Count: rec.Cnt, Timestamp: rec.Tim})
	}

	out.GetMPInProgress = len(network.GetMPInProgressTicket) != 0

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}

// jsonBWChar -
func jsonBWChar(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var cnt uint64

	if len(r.Form["seconds"]) > 0 {
		cnt, _ = strconv.ParseUint(r.Form["seconds"][0], 10, 32)
	}
	if cnt < 1 {
		cnt = 1
	} else if cnt > 300 {
		cnt = 300
	}

	var out struct {
		DL           [200]uint64 // max 200 records (from 200 seconds to ~16.7 hours)
		UL           [200]uint64
		MaxDL, MaxUL uint64
	}

	common.LockBw()
	common.TickRecv()
	common.TickSent()

	idx := uint16(common.DlBytesPrevSecIdx)
	for i := range out.DL {
		var sum uint64
		for c := 0; c < int(cnt); c++ {
			idx--
			sum += common.DlBytesPrevSec[idx]
			if common.DlBytesPrevSec[idx] > out.MaxDL {
				out.MaxDL = common.DlBytesPrevSec[idx]
			}
		}
		out.DL[i] = sum / cnt
	}

	idx = uint16(common.UlBytesPrevSecIdx)
	for i := range out.UL {
		var sum uint64
		for c := 0; c < int(cnt); c++ {
			idx--
			sum += common.UlBytesPrevSec[idx]
			if common.UlBytesPrevSec[idx] > out.MaxUL {
				out.MaxUL = common.UlBytesPrevSec[idx]
			}
		}
		out.UL[i] = sum / cnt
	}

	common.UnlockBw()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
