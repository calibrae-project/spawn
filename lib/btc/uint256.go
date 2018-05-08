package btc

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
)

// Uint256IdxLen -
const Uint256IdxLen = 8 // The bigger it is, the more memory is needed, but lower chance of a collision
// Uint256 -
type Uint256 struct {
	Hash [32]byte
}

// NewUint256 -
func NewUint256(h []byte) (res *Uint256) {
	res = new(Uint256)
	copy(res.Hash[:], h)
	return
}

// NewUint256FromString - Get from MSB hexstring
func NewUint256FromString(s string) (res *Uint256) {
	d, e := hex.DecodeString(s)
	if e != nil {
		//println("NewUint256FromString", s, e.Error())
		return
	}
	if len(d) != 32 {
		//println("NewUint256FromString", s, "not 32 bytes long")
		return
	}
	res = new(Uint256)
	for i := 0; i < 32; i++ {
		res.Hash[31-i] = d[i]
	}
	return
}

// NewSha2Hash -
func NewSha2Hash(data []byte) (res *Uint256) {
	res = new(Uint256)
	ShaHash(data, res.Hash[:])
	return
}

// Bytes -
func (u *Uint256) Bytes() []byte {
	return u.Hash[:]
}

func (u *Uint256) String() (s string) {
	for i := 0; i < 32; i++ {
		s += fmt.Sprintf("%02x", u.Hash[31-i])
	}
	return
}

// Equal -
func (u *Uint256) Equal(o *Uint256) bool {
	return bytes.Equal(u.Hash[:], o.Hash[:])
}

// Calc -
func (u *Uint256) Calc(data []byte) {
	ShaHash(data, u.Hash[:])
}

// BIdx -
func BIdx(hash []byte) (o [Uint256IdxLen]byte) {
	copy(o[:], hash[:Uint256IdxLen])
	return
}

// BIdx -
func (u *Uint256) BIdx() (o [Uint256IdxLen]byte) {
	o = BIdx(u.Hash[:])
	return
}

// BigInt -
func (u *Uint256) BigInt() *big.Int {
	var buf [32]byte
	for i := range buf {
		buf[i] = u.Hash[31-i]
	}
	return new(big.Int).SetBytes(buf[:])
}
