package btc

import (
	"encoding/binary"
	"fmt"
)

// NetAddr -
type NetAddr struct {
	Services uint64
	IPv6     [12]byte
	IPv4     [4]byte
	Port     uint16
}

// NewNetAddr -
func NewNetAddr(b []byte) (na *NetAddr) {
	if len(b) != 26 {
		println("Incorrect input data length", len(b))
		return
	}
	na = new(NetAddr)
	na.Services = binary.LittleEndian.Uint64(b[0:8])
	copy(na.IPv6[:], b[8:20])
	copy(na.IPv4[:], b[20:24])
	na.Port = binary.BigEndian.Uint16(b[24:26])
	return
}

// Bytes -
func (a *NetAddr) Bytes() (res []byte) {
	res = make([]byte, 26)
	binary.LittleEndian.PutUint64(res[0:8], a.Services)
	copy(res[8:20], a.IPv6[:])
	copy(res[20:24], a.IPv4[:])
	binary.BigEndian.PutUint16(res[24:26], a.Port)
	return
}

// String -
func (a *NetAddr) String() string {
	return fmt.Sprintf("%d.%d.%d.%d:%d", a.IPv4[0], a.IPv4[1], a.IPv4[2], a.IPv4[3], a.Port)
}
