package secp256k1

import (
	"fmt"
)

// XY -
type XY struct {
	X, Y     Field
	Infinity bool
}

// Print -
func (xy *XY) Print(lab string) {
	if xy.Infinity {
		fmt.Println(lab + " - Infinity")
		return
	}
	fmt.Println(lab+".X:", xy.X.String())
	fmt.Println(lab+".Y:", xy.Y.String())
}

// ParsePubkey -
func (xy *XY) ParsePubkey(pub []byte) bool {
	if len(pub) == 33 && (pub[0] == 0x02 || pub[0] == 0x03) {
		xy.X.SetB32(pub[1:33])
		xy.SetXO(&xy.X, pub[0] == 0x03)
	} else if len(pub) == 65 && (pub[0] == 0x04 || pub[0] == 0x06 || pub[0] == 0x07) {
		xy.X.SetB32(pub[1:33])
		xy.Y.SetB32(pub[33:65])
		if (pub[0] == 0x06 || pub[0] == 0x07) && xy.Y.IsOdd() != (pub[0] == 0x07) {
			return false
		}
	} else {
		return false
	}
	return true
}

// Bytes - Returns serialized key in uncompressed format "<04> <X> <Y>"
// ... or in compressed format: "<02> <X>", eventually "<03> <X>"
func (xy *XY) Bytes(compressed bool) (raw []byte) {
	if compressed {
		raw = make([]byte, 33)
		if xy.Y.IsOdd() {
			raw[0] = 0x03
		} else {
			raw[0] = 0x02
		}
		xy.X.GetB32(raw[1:])
	} else {
		raw = make([]byte, 65)
		raw[0] = 0x04
		xy.X.GetB32(raw[1:33])
		xy.Y.GetB32(raw[33:65])
	}
	return
}

// SetXY -
func (xy *XY) SetXY(X, Y *Field) {
	xy.Infinity = false
	xy.X = *X
	xy.Y = *Y
}

// IsValid -
func (xy *XY) IsValid() bool {
	if xy.Infinity {
		return false
	}
	var y2, x3, c Field
	xy.Y.Sqr(&y2)
	xy.X.Sqr(&x3)
	x3.Mul(&x3, &xy.X)
	c.SetInt(7)
	x3.SetAdd(&c)
	y2.Normalize()
	x3.Normalize()
	return y2.Equals(&x3)
}

// SetXYZ -
func (xy *XY) SetXYZ(a *XYZ) {
	var z2, z3 Field
	a.Z.InvVar(&a.Z)
	a.Z.Sqr(&z2)
	a.Z.Mul(&z3, &z2)
	a.X.Mul(&a.X, &z2)
	a.Y.Mul(&a.Y, &z3)
	a.Z.SetInt(1)
	xy.Infinity = a.Infinity
	xy.X = a.X
	xy.Y = a.Y
}

func (xy *XY) precomp(w int) (pre []XY) {
	pre = make([]XY, (1 << (uint(w) - 2)))
	pre[0] = *xy
	var X, d, tmp XYZ
	X.SetXY(xy)
	X.Double(&d)
	for i := 1; i < len(pre); i++ {
		d.AddXY(&tmp, &pre[i-1])
		pre[i].SetXYZ(&tmp)
	}
	return
}

// Neg -
func (xy *XY) Neg(r *XY) {
	r.Infinity = xy.Infinity
	r.X = xy.X
	r.Y = xy.Y
	r.Y.Normalize()
	r.Y.Negate(&r.Y, 1)
}

// SetXO -
func (xy *XY) SetXO(X *Field, odd bool) {
	var c, x2, x3 Field
	xy.X = *X
	X.Sqr(&x2)
	X.Mul(&x3, &x2)
	xy.Infinity = false
	c.SetInt(7)
	c.SetAdd(&x3)
	c.Sqrt(&xy.Y)
	if xy.Y.IsOdd() != odd {
		xy.Y.Negate(&xy.Y, 1)
	}
	xy.Y.Normalize()
}

// AddXY -
func (xy *XY) AddXY(a *XY) {
	var xyz XYZ
	xyz.SetXY(xy)
	xyz.AddXY(&xyz, a)
	xy.SetXYZ(&xyz)
}

// GetPublicKey -
func (xy *XY) GetPublicKey(out []byte) {
	xy.X.Normalize() // See GitHub issue #15
	xy.X.GetB32(out[1:33])
	if len(out) == 65 {
		out[0] = 0x04
		xy.Y.Normalize()
		xy.Y.GetB32(out[33:65])
	} else {
		if xy.Y.IsOdd() {
			out[0] = 0x03
		} else {
			out[0] = 0x02
		}
	}
}
