package utils

import (
	"encoding/binary"
	"hash/crc64"

	"github.com/ParallelCoinTeam/duod/lib/btc"
)

// OnePeer -
type OnePeer struct {
	btc.NetAddr
	Time   uint32 // When seen last time
	Banned uint32 // time when this address baned or zero if never
}

var crctab = crc64.MakeTable(crc64.ISO)

/*
Serialized peer record (all values are LSB unless specified otherwise):
 [0:4] - Unix timestamp of when last the peer was seen
 [4:12] - Services
 [12:24] - IPv6 (network order)
 [24:28] - IPv4 (network order)
 [28:30] - TCP port (big endian)
 [30:34] - OPTIONAL: if present, unix timestamp of when the peer was banned
*/

// NewPeer -
func NewPeer(v []byte) (p *OnePeer) {
	if len(v) < 30 {
		println("NewPeer: unexpected length", len(v))
		return
	}
	p = new(OnePeer)
	p.Time = binary.LittleEndian.Uint32(v[0:4])
	p.Services = binary.LittleEndian.Uint64(v[4:12])
	copy(p.IPv6[:], v[12:24])
	copy(p.IPv4[:], v[24:28])
	p.Port = binary.BigEndian.Uint16(v[28:30])
	if len(v) >= 34 {
		p.Banned = binary.LittleEndian.Uint32(v[30:34])
	}
	return
}

// Bytes -
func (p *OnePeer) Bytes() (res []byte) {
	if p.Banned != 0 {
		res = make([]byte, 34)
		binary.LittleEndian.PutUint32(res[30:34], p.Banned)
	} else {
		res = make([]byte, 30)
	}
	binary.LittleEndian.PutUint32(res[0:4], p.Time)
	binary.LittleEndian.PutUint64(res[4:12], p.Services)
	copy(res[12:24], p.IPv6[:])
	copy(res[24:28], p.IPv4[:])
	binary.BigEndian.PutUint16(res[28:30], p.Port)
	return
}

// UniqID -
func (p *OnePeer) UniqID() uint64 {
	h := crc64.New(crctab)
	h.Write(p.IPv6[:])
	h.Write(p.IPv4[:])
	h.Write([]byte{byte(p.Port >> 8), byte(p.Port)})
	return h.Sum64()
}
