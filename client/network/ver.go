// Package network -
package network

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/calibrae-project/spawn/client/common"
	"github.com/calibrae-project/spawn/lib/btc"
	"github.com/calibrae-project/spawn/lib/others/sys"
)

// IgnoreExternalIPFrom -
var IgnoreExternalIPFrom = []string{}

// SendVersion -
func (c *OneConnection) SendVersion() {
	b := bytes.NewBuffer([]byte{})

	binary.Write(b, binary.LittleEndian, uint32(common.Version))
	binary.Write(b, binary.LittleEndian, uint64(common.Services))
	binary.Write(b, binary.LittleEndian, uint64(time.Now().Unix()))

	b.Write(c.PeerAddr.NetAddr.Bytes())
	if ExternalAddrLen() > 0 {
		b.Write(BestExternalAddr())
	} else {
		b.Write(bytes.Repeat([]byte{0}, 26))
	}

	b.Write(nonce[:])

	common.LockCfg()
	btc.WriteVlen(b, uint64(len(common.UserAgent)))
	b.Write([]byte(common.UserAgent))
	common.UnlockCfg()

	binary.Write(b, binary.LittleEndian, uint32(common.Last.BlockHeight()))
	if !common.GetBool(&common.CFG.TXPool.Enabled) {
		b.WriteByte(0) // don't notify me about txs
	}

	c.SendRawMsg("version", b.Bytes())
}

// IsSpawn -
func (c *OneConnection) IsSpawn() bool {
	return strings.HasPrefix(c.Node.Agent, "/Spawn:")
}

// HandleVersion -
func (c *OneConnection) HandleVersion(pl []byte) error {
	if len(pl) >= 80 /*Up to, includiong, the nonce */ {
		if bytes.Equal(pl[72:80], nonce[:]) {
			common.CountSafe("VerNonceUs")
			return errors.New("Connecting to ourselves")
		}

		// check if we don't have this nonce yet
		MutexNet.Lock()
		for _, v := range OpenCons {
			if v != c {
				v.Mutex.Lock()
				yes := v.X.VersionReceived && bytes.Equal(v.Node.Nonce[:], pl[72:80])
				v.Mutex.Unlock()
				if yes {
					MutexNet.Unlock()
					v.Mutex.Lock()
					/*println("Peer with nonce", hex.EncodeToString(pl[72:80]), "from", c.PeerAddr.IP(),
					"already connected as ", v.ConnID, "from ", v.PeerAddr.IP(), v.Node.Agent)*/
					v.Mutex.Unlock()
					common.CountSafe("VerNonceSame")
					return errors.New("Peer already connected")
				}
			}
		}
		MutexNet.Unlock()

		c.Mutex.Lock()
		c.Node.Version = binary.LittleEndian.Uint32(pl[0:4])
		if c.Node.Version < MinProtoVersion {
			c.Mutex.Unlock()
			return errors.New("Client version too low")
		}

		copy(c.Node.Nonce[:], pl[72:80])
		c.Node.Services = binary.LittleEndian.Uint64(pl[4:12])
		c.Node.Timestamp = binary.LittleEndian.Uint64(pl[12:20])
		c.Node.ReportedIPv4 = binary.BigEndian.Uint32(pl[40:44])

		useThisIP := sys.ValidIp4(pl[40:44])

		if len(pl) >= 86 {
			le, of := btc.VLen(pl[80:])
			of += 80
			c.Node.Agent = string(pl[of : of+le])
			of += le
			if len(pl) >= of+4 {
				c.Node.Height = binary.LittleEndian.Uint32(pl[of : of+4])
				c.X.GetBlocksDataNow = true
				of += 4
				if len(pl) > of && pl[of] == 0 {
					c.Node.DoNotRelayTxs = true
				}
			}
			c.X.IsSpawn = strings.HasPrefix(c.Node.Agent, "/Spawn:")
		}
		c.X.VersionReceived = true
		c.Mutex.Unlock()

		if useThisIP {
			if bytes.Equal(pl[40:44], c.PeerAddr.IPv4[:]) {
				if common.FLAG.Log {
					ExternalIPmutex.Lock()
					f, _ := os.OpenFile("badip_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
					if f != nil {
						fmt.Fprintf(f, "%s: OWN IP from %s @ %s - %d\n",
							time.Now().Format("2006-01-02 15:04:05"),
							c.Node.Agent, c.PeerAddr.IP(), c.ConnID)
						f.Close()
					}
					ExternalIPmutex.Unlock()
				}
				common.CountSafe("IgnoreExtIP-O")
				useThisIP = false
			} else if len(pl) >= 86 && binary.BigEndian.Uint32(pl[66:70]) != 0 &&
				!bytes.Equal(pl[66:70], c.PeerAddr.IPv4[:]) {
				if common.FLAG.Log {
					ExternalIPmutex.Lock()
					f, _ := os.OpenFile("badip_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
					if f != nil {
						fmt.Fprintf(f, "%s: BAD IP=%d.%d.%d.%d from %s @ %s - %d\n",
							time.Now().Format("2006-01-02 15:04:05"),
							pl[66], pl[67], pl[68], pl[69], c.Node.Agent, c.PeerAddr.IP(), c.ConnID)
						f.Close()
					}
					ExternalIPmutex.Unlock()
				}
				common.CountSafe("IgnoreExtIP-B")
				useThisIP = false
			}
		}

		if useThisIP {
			ExternalIPmutex.Lock()
			if _, known := ExternalIP4[c.Node.ReportedIPv4]; !known { // New IP
				useThisIP = true
				for x, v := range IgnoreExternalIPFrom {
					if c.Node.Agent == v {
						useThisIP = false
						common.CountSafe(fmt.Sprint("IgnoreExtIP", x))
						break
					}
				}
				if useThisIP && common.IsListenTCP() {
					fmt.Printf("New external IP %d.%d.%d.%d from ConnID=%d\n> ",
						pl[40], pl[41], pl[42], pl[43], c.ConnID)
				}
			}
			if useThisIP {
				ExternalIP4[c.Node.ReportedIPv4] = [2]uint{ExternalIP4[c.Node.ReportedIPv4][0] + 1,
					uint(time.Now().Unix())}
			}
			ExternalIPmutex.Unlock()
		}

	} else {
		return errors.New("version message too short")
	}
	c.SendRawMsg("verack", []byte{})
	return nil
}
