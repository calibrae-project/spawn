package secp256k1

import (
	"encoding/hex"
	"fmt"
	"math/big"
)

// BigInt1 -
var BigInt1 = new(big.Int).SetInt64(1)

// Number -
type Number struct {
	big.Int
}

// Print -
func (N *Number) Print(label string) {
	fmt.Println(label, hex.EncodeToString(N.Bytes()))
}

func (N *Number) modMul(a, b, m *Number) {
	N.Mul(&a.Int, &b.Int)
	N.Mod(&N.Int, &m.Int)
	return
}

func (N *Number) modInv(a, b *Number) {
	N.ModInverse(&a.Int, &b.Int)
	return
}

func (N *Number) mod(a *Number) {
	N.Mod(&N.Int, &a.Int)
	return
}

// SetHex -
func (N *Number) SetHex(s string) {
	N.SetString(s, 16)
}

func (N *Number) maskBits(bits uint) {
	mask := new(big.Int).Lsh(BigInt1, bits)
	mask.Sub(mask, BigInt1)
	N.Int.And(&N.Int, mask)
}

func (N *Number) splitExp(r1, r2 *Number) {
	var bnc1, bnc2, bnn2, bnt1, bnt2 Number

	bnn2.Int.Rsh(&TheCurve.Order.Int, 1)

	bnc1.Mul(&N.Int, &TheCurve.a1b2.Int)
	bnc1.Add(&bnc1.Int, &bnn2.Int)
	bnc1.Div(&bnc1.Int, &TheCurve.Order.Int)

	bnc2.Mul(&N.Int, &TheCurve.b1.Int)
	bnc2.Add(&bnc2.Int, &bnn2.Int)
	bnc2.Div(&bnc2.Int, &TheCurve.Order.Int)

	bnt1.Mul(&bnc1.Int, &TheCurve.a1b2.Int)
	bnt2.Mul(&bnc2.Int, &TheCurve.a2.Int)
	bnt1.Add(&bnt1.Int, &bnt2.Int)
	r1.Sub(&N.Int, &bnt1.Int)

	bnt1.Mul(&bnc1.Int, &TheCurve.b1.Int)
	bnt2.Mul(&bnc2.Int, &TheCurve.a1b2.Int)
	r2.Sub(&bnt1.Int, &bnt2.Int)
}

func (N *Number) split(rl, rh *Number, bits uint) {
	rl.Int.Set(&N.Int)
	rh.Int.Rsh(&rl.Int, bits)
	rl.maskBits(bits)
}

func (N *Number) rsh(bits uint) {
	N.Rsh(&N.Int, bits)
}

func (N *Number) inc() {
	N.Add(&N.Int, BigInt1)
}

func (N *Number) rshX(bits uint) (res int) {
	res = int(new(big.Int).And(&N.Int, new(big.Int).SetUint64((1<<bits)-1)).Uint64())
	N.Rsh(&N.Int, bits)
	return
}

// IsOdd -
func (N *Number) IsOdd() bool {
	return N.Bit(0) != 0
}

func (N *Number) getBin(le int) []byte {
	bts := N.Bytes()
	if len(bts) > le {
		panic("buffer too small")
	}
	if len(bts) == le {
		return bts
	}
	return append(make([]byte, le-len(bts)), bts...)
}
