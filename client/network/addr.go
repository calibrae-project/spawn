// Package network -
package network

import (
	"bytes"
	"encoding/binary"
	"sort"
	"sync"
	"time"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/L"
	"github.com/ParallelCoinTeam/duod/lib/others/peersdb"
	"github.com/ParallelCoinTeam/duod/lib/others/qdb"
	"github.com/ParallelCoinTeam/duod/lib/others/sys"
)

var (
	// ExternalIP4 - [0]-count, [1]-timestamp
	ExternalIP4 = make(map[uint32][2]uint)
	// ExternalIPmutex -
	ExternalIPmutex sync.Mutex
	// ExternalIPexpireTicker -
	ExternalIPexpireTicker int
)

// ExternalAddrLen -
func ExternalAddrLen() (res int) {
	ExternalIPmutex.Lock()
	res = len(ExternalIP4)
	ExternalIPmutex.Unlock()
	return
}

// ExternalIPrec -
type ExternalIPrec struct {
	IP  uint32
	Cnt uint
	Tim uint
}

// GetExternalIPs - Returns the list sorted by "freshness"
func GetExternalIPs() (arr []ExternalIPrec) {
	L.Debug("Getting external IPs")
	ExternalIPmutex.Lock()
	defer ExternalIPmutex.Unlock()
	if len(ExternalIP4) > 0 {
		arr = make([]ExternalIPrec, len(ExternalIP4))
		var idx int
		for ip, rec := range ExternalIP4 {
			arr[idx].IP = ip
			arr[idx].Cnt = rec[0]
			arr[idx].Tim = rec[1]
			idx++
		}
		sort.Slice(arr, func(i, j int) bool {
			if arr[i].Cnt > 3 && arr[j].Cnt > 3 || arr[i].Cnt == arr[j].Cnt {
				return arr[i].Tim > arr[j].Tim
			}
			return arr[i].Cnt > arr[j].Cnt
		})
	}
	return
}

// BestExternalAddr -
func BestExternalAddr() []byte {
	L.Debug("Getting best external address")
	arr := GetExternalIPs()

	// Expire any extra IP if it has been stale for more than an hour
	if len(arr) > 1 {
		L.Debug("Expiring stale IPs")
		worst := &arr[len(arr)-1]

		if uint(time.Now().Unix())-worst.Tim > 3600 {
			common.CountSafe("ExternalIPExpire")
			ExternalIPmutex.Lock()
			if ExternalIP4[worst.IP][0] == worst.Cnt {
				delete(ExternalIP4, worst.IP)
			}
			ExternalIPmutex.Unlock()
		}
	}

	res := make([]byte, 26)
	binary.LittleEndian.PutUint64(res[0:8], common.Services)
	// leave ip6 filled with zeros, except for the last 2 bytes:
	res[18], res[19] = 0xff, 0xff
	if len(arr) > 0 {
		binary.BigEndian.PutUint32(res[20:24], arr[0].IP)
	}
	binary.BigEndian.PutUint16(res[24:26], common.DefaultTCPport())
	return res
}

// SendAddr -
func (c *OneConnection) SendAddr() {
	L.Debug("Send addresses")
	pers := peersdb.GetBestPeers(MaxAddrsPerMessage, nil)
	maxtime := uint32(time.Now().Unix() + 3600)
	if len(pers) > 0 {
		buf := new(bytes.Buffer)
		btc.WriteVlen(buf, uint64(len(pers)))
		for i := range pers {
			if pers[i].Time > maxtime {
				L.Debug("addr", i, "time in future", pers[i].Time, maxtime, "should not happen")
				pers[i].Time = maxtime - 7200
			}
			binary.Write(buf, binary.LittleEndian, pers[i].Time)
			buf.Write(pers[i].NetAddr.Bytes())
		}
		c.SendRawMsg("addr", buf.Bytes())
	}
}

// SendOwnAddr -
func (c *OneConnection) SendOwnAddr() {
	L.Debug("Sending own address")
	if ExternalAddrLen() > 0 {
		buf := new(bytes.Buffer)
		btc.WriteVlen(buf, uint64(1))
		binary.Write(buf, binary.LittleEndian, uint32(time.Now().Unix()))
		buf.Write(BestExternalAddr())
		c.SendRawMsg("addr", buf.Bytes())
	}
}

// ParseAddr - Parse network's "addr" message
func (c *OneConnection) ParseAddr(pl []byte) {
	L.Debug("Parsing address")
	b := bytes.NewBuffer(pl)
	cnt, _ := btc.ReadVLen(b)
	for i := 0; i < int(cnt); i++ {
		var buf [30]byte
		n, e := b.Read(buf[:])
		if n != len(buf) || e != nil {
			common.CountSafe("AddrError")
			c.DoS("AddrError")
			L.Debug("ParseAddr:", n, e)
			break
		}
		a := peersdb.NewPeer(buf[:])
		if !sys.ValidIPv4(a.IPv4[:]) {
			L.Debug("Address Invalid")
			common.CountSafe("AddrInvalid")
			/*if c.Misbehave("AddrLocal", 1) {
				break
			}*/
			//print(c.PeerAddr.IP(), " ", c.Node.Agent, " ", c.Node.Version, " addr local ", a.String(), "\n> ")
		} else if time.Unix(int64(a.Time), 0).Before(time.Now().Add(time.Hour)) {
			if time.Now().Before(time.Unix(int64(a.Time), 0).Add(peersdb.ExpirePeerAfter)) {
				k := qdb.KeyType(a.UniqID())
				v := peersdb.PeerDB.Get(k)
				if v != nil {
					a.Banned = peersdb.NewPeer(v[:]).Banned
				}
				a.Time = uint32(time.Now().Add(-5 * time.Minute).Unix()) // add new peers as not just alive
				if a.Time > uint32(time.Now().Unix()) {
					println("wtf", a.Time, time.Now().Unix())
				}
				peersdb.PeerDB.Put(k, a.Bytes())
			} else {
				common.CountSafe("AddrStale")
			}
		} else {
			if c.Misbehave("AddrFuture", 50) {
				break
			}
		}
	}
}
