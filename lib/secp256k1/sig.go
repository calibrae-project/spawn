package secp256k1

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

// Signature -
type Signature struct {
	R, S Number
}

// Print -
func (Sig *Signature) Print(lab string) {
	fmt.Println(lab+".R:", hex.EncodeToString(Sig.R.Bytes()))
	fmt.Println(lab+".S:", hex.EncodeToString(Sig.S.Bytes()))
}

// ParseBytes -
func (Sig *Signature) ParseBytes(sig []byte) int {
	if len(sig) < 5 || sig[0] != 0x30 {
		return -1
	}

	lenr := int(sig[3])
	if lenr == 0 || 5+lenr >= len(sig) || sig[lenr+4] != 0x02 {
		return -1
	}

	lens := int(sig[lenr+5])
	if lens == 0 || int(sig[1]) != lenr+lens+4 || lenr+lens+6 > len(sig) || sig[2] != 0x02 {
		return -1
	}

	Sig.R.SetBytes(sig[4 : 4+lenr])
	Sig.S.SetBytes(sig[6+lenr : 6+lenr+lens])
	return 6 + lenr + lens
}

// Verify -
func (Sig *Signature) Verify(pubkey *XY, message *Number) (ret bool) {
	var r2 Number
	ret = Sig.recompute(&r2, pubkey, message) && Sig.R.Cmp(&r2.Int) == 0
	return
}

func (Sig *Signature) recompute(r2 *Number, pubkey *XY, message *Number) (ret bool) {
	var sn, u1, u2 Number

	sn.modInv(&Sig.S, &TheCurve.Order)
	u1.modMul(&sn, message, &TheCurve.Order)
	u2.modMul(&sn, &Sig.R, &TheCurve.Order)

	var pr, pubkeyj XYZ
	pubkeyj.SetXY(pubkey)

	pubkeyj.ECmult(&pr, &u2, &u1)
	if !pr.IsInfinity() {
		var xr Field
		pr.getX(&xr)
		xr.Normalize()
		var xrb [32]byte
		xr.GetB32(xrb[:])
		r2.SetBytes(xrb[:])
		r2.Mod(&r2.Int, &TheCurve.Order.Int)
		ret = true
	}

	return
}

func (Sig *Signature) recover(pubkey *XY, m *Number, recid int) (ret bool) {
	var rx, rn, u1, u2 Number
	var fx Field
	var X XY
	var xj, qj XYZ

	rx.Set(&Sig.R.Int)
	if (recid & 2) != 0 {
		rx.Add(&rx.Int, &TheCurve.Order.Int)
		if rx.Cmp(&TheCurve.p.Int) >= 0 {
			return false
		}
	}

	fx.SetB32(rx.getBin(32))

	X.SetXO(&fx, (recid&1) != 0)
	if !X.IsValid() {
		return false
	}

	xj.SetXY(&X)
	rn.modInv(&Sig.R, &TheCurve.Order)
	u1.modMul(&rn, m, &TheCurve.Order)
	u1.Sub(&TheCurve.Order.Int, &u1.Int)
	u2.modMul(&rn, &Sig.S, &TheCurve.Order)
	xj.ECmult(&qj, &u2, &u1)
	pubkey.SetXYZ(&qj)

	return true
}

// Sign -
func (Sig *Signature) Sign(seckey, message, nonce *Number, recid *int) int {
	var r XY
	var rp XYZ
	var n Number
	var b [32]byte

	ECmultGen(&rp, nonce)
	r.SetXYZ(&rp)
	r.X.Normalize()
	r.Y.Normalize()
	r.X.GetB32(b[:])
	Sig.R.SetBytes(b[:])
	if recid != nil {
		*recid = 0
		if Sig.R.Cmp(&TheCurve.Order.Int) >= 0 {
			*recid |= 2
		}
		if r.Y.IsOdd() {
			*recid |= 1
		}
	}
	Sig.R.mod(&TheCurve.Order)
	n.modMul(&Sig.R, seckey, &TheCurve.Order)
	n.Add(&n.Int, &message.Int)
	n.mod(&TheCurve.Order)
	Sig.S.modInv(nonce, &TheCurve.Order)
	Sig.S.modMul(&Sig.S, &n, &TheCurve.Order)
	if Sig.S.Sign() == 0 {
		return 0
	}
	if Sig.S.IsOdd() {
		Sig.S.Sub(&TheCurve.Order.Int, &Sig.S.Int)
		if recid != nil {
			*recid ^= 1
		}
	}

	if ForceLowS && Sig.S.Cmp(&TheCurve.HalfOrder.Int) == 1 {
		Sig.S.Sub(&TheCurve.Order.Int, &Sig.S.Int)
		if recid != nil {
			*recid ^= 1
		}
	}

	return 1
}

// Bytes -
func (Sig *Signature) Bytes() []byte {
	r := Sig.R.Bytes()
	if r[0] >= 0x80 {
		r = append([]byte{0}, r...)
	}
	s := Sig.S.Bytes()
	if s[0] >= 0x80 {
		s = append([]byte{0}, s...)
	}
	res := new(bytes.Buffer)
	res.WriteByte(0x30)
	res.WriteByte(byte(4 + len(r) + len(s)))
	res.WriteByte(0x02)
	res.WriteByte(byte(len(r)))
	res.Write(r)
	res.WriteByte(0x02)
	res.WriteByte(byte(len(s)))
	res.Write(s)
	return res.Bytes()
}
