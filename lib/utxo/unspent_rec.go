package utxo

import (
	//"encoding/binary"
	"github.com/calibrae-project/spawn/lib/btc"
)

/*
Each unspent key is 8 bytes long - thats firt 8 bytes of TXID
Eech value is variable length:
  [0:24] - remainig 24 bytes of TxID
  var_int: BlochHeight
  var_int: 2*out_cnt + is_coibase
  And now set of records:
   var_int: Output index
   var_int: Value
   var_int: PKscrpt_length
   PKscript
  ...
*/

const (
	// UtxoIdxLen -
	UtxoIdxLen = 8
)

// KeyType -
type KeyType [UtxoIdxLen]byte

// Rec -
type Rec struct {
	TxID     [32]byte
	Coinbase bool
	InBlock  uint32
	Outs     []*TxOut
}

// TxOut -
type TxOut struct {
	Value uint64
	PKScr []byte
}

// FullUtxoRec -
func FullUtxoRec(dat []byte) *Rec {
	var key KeyType
	copy(key[:], dat[:UtxoIdxLen])
	return NewUtxoRec(key, dat[UtxoIdxLen:])
}

var (
	staRec  Rec
	recOuts = make([]*TxOut, 30001)
	recPool = make([]TxOut, 30001)
)

// NewUtxoRecStatic -
func NewUtxoRecStatic(key KeyType, dat []byte) *Rec {
	var off, n, i, recIdx int
	var u64, idx uint64

	off = 32 - UtxoIdxLen
	copy(staRec.TxID[:UtxoIdxLen], key[:])
	copy(staRec.TxID[UtxoIdxLen:], dat[:off])

	u64, n = btc.VULe(dat[off:])
	off += n
	staRec.InBlock = uint32(u64)

	u64, n = btc.VULe(dat[off:])
	off += n

	staRec.Coinbase = (u64 & 1) != 0
	u64 >>= 1
	if len(recOuts) < int(u64) {
		recOuts = make([]*TxOut, u64)
		recPool = make([]TxOut, u64)
	}
	staRec.Outs = recOuts[:u64]
	for i := range staRec.Outs {
		staRec.Outs[i] = nil
	}

	for off < len(dat) {
		idx, n = btc.VULe(dat[off:])
		off += n

		staRec.Outs[idx] = &recPool[recIdx]
		recIdx++

		u64, n = btc.VULe(dat[off:])
		off += n
		staRec.Outs[idx].Value = uint64(u64)

		i, n = btc.VLen(dat[off:])
		off += n

		staRec.Outs[idx].PKScr = dat[off : off+i]
		off += i
	}

	return &staRec
}

// NewUtxoRec -
func NewUtxoRec(key KeyType, dat []byte) *Rec {
	var off, n, i int
	var u64, idx uint64
	var rec Rec

	off = 32 - UtxoIdxLen
	copy(rec.TxID[:UtxoIdxLen], key[:])
	copy(rec.TxID[UtxoIdxLen:], dat[:off])

	u64, n = btc.VULe(dat[off:])
	off += n
	rec.InBlock = uint32(u64)

	u64, n = btc.VULe(dat[off:])
	off += n

	rec.Coinbase = (u64 & 1) != 0
	rec.Outs = make([]*TxOut, u64>>1)

	for off < len(dat) {
		idx, n = btc.VULe(dat[off:])
		off += n
		rec.Outs[idx] = new(TxOut)

		u64, n = btc.VULe(dat[off:])
		off += n
		rec.Outs[idx].Value = uint64(u64)

		i, n = btc.VLen(dat[off:])
		off += n

		rec.Outs[idx].PKScr = dat[off : off+i]
		off += i
	}
	return &rec
}

// OneUtxoRec -
func OneUtxoRec(key KeyType, dat []byte, vout uint32) *btc.TxOut {
	var off, n, i int
	var u64, idx uint64
	var res btc.TxOut

	off = 32 - UtxoIdxLen

	u64, n = btc.VULe(dat[off:])
	off += n
	res.BlockHeight = uint32(u64)

	u64, n = btc.VULe(dat[off:])
	off += n

	res.VoutCount = uint32(u64 >> 1)
	if res.VoutCount <= vout {
		return nil
	}
	res.WasCoinbase = (u64 & 1) != 0

	for off < len(dat) {
		idx, n = btc.VULe(dat[off:])
		if uint32(idx) > vout {
			return nil
		}
		off += n

		u64, n = btc.VULe(dat[off:])
		off += n

		i, n = btc.VLen(dat[off:])
		off += n

		if uint32(idx) == vout {
			res.Value = uint64(u64)
			res.PkScript = dat[off : off+i]
			return &res
		}
		off += i
	}
	return nil
}

func vlen2size(uvl uint64) int {
	if uvl < 0xfd {
		return 1
	} else if uvl < 0x10000 {
		return 3
	} else if uvl < 0x100000000 {
		return 5
	}
	return 9
}

// Serialize -
func (rec *Rec) Serialize(full bool) (buf []byte) {
	var le, of int
	var anyOut bool

	outcnt := uint64(len(rec.Outs) << 1)
	if rec.Coinbase {
		outcnt |= 1
	}

	if full {
		le = 32
	} else {
		le = 32 - UtxoIdxLen
	}

	le += vlen2size(uint64(rec.InBlock)) // block length
	le += vlen2size(outcnt)              // out count

	for i := range rec.Outs {
		if rec.Outs[i] != nil {
			le += vlen2size(uint64(i))
			le += vlen2size(rec.Outs[i].Value)
			le += vlen2size(uint64(len(rec.Outs[i].PKScr)))
			le += len(rec.Outs[i].PKScr)
			anyOut = true
		}
	}
	if !anyOut {
		return
	}

	buf = make([]byte, le)
	if full {
		copy(buf[:32], rec.TxID[:])
		of = 32
	} else {
		of = 32 - UtxoIdxLen
		copy(buf[:of], rec.TxID[UtxoIdxLen:])
	}

	of += btc.PutULe(buf[of:], uint64(rec.InBlock))
	of += btc.PutULe(buf[of:], outcnt)
	for i := range rec.Outs {
		if rec.Outs[i] != nil {
			of += btc.PutULe(buf[of:], uint64(i))
			of += btc.PutULe(buf[of:], rec.Outs[i].Value)
			of += btc.PutULe(buf[of:], uint64(len(rec.Outs[i].PKScr)))
			copy(buf[of:], rec.Outs[i].PKScr)
			of += len(rec.Outs[i].PKScr)
		}
	}
	return
}

// Bytes -
func (rec *Rec) Bytes() []byte {
	return rec.Serialize(false)
}

// ToUnspent -
func (rec *Rec) ToUnspent(idx uint32, ad *btc.Addr) (nr *OneUnspentTx) {
	nr = new(OneUnspentTx)
	nr.TxPrevOut.Hash = rec.TxID
	nr.TxPrevOut.Vout = idx
	nr.Value = rec.Outs[idx].Value
	nr.Coinbase = rec.Coinbase
	nr.MinedAt = rec.InBlock
	nr.Addr = ad
	nr.destString = ad.String()
	return
}

// IsP2KH -
func (out *TxOut) IsP2KH() bool {
	return len(out.PKScr) == 25 && out.PKScr[0] == 0x76 && out.PKScr[1] == 0xa9 &&
		out.PKScr[2] == 0x14 && out.PKScr[23] == 0x88 && out.PKScr[24] == 0xac
}

// IsP2SH -
func (out *TxOut) IsP2SH() bool {
	return len(out.PKScr) == 23 && out.PKScr[0] == 0xa9 && out.PKScr[1] == 0x14 && out.PKScr[22] == 0x87
}

// IsP2WPKH -
func (out *TxOut) IsP2WPKH() bool {
	return len(out.PKScr) == 22 && out.PKScr[0] == 0 && out.PKScr[1] == 20
}

// IsP2WSH -
func (out *TxOut) IsP2WSH() bool {
	return len(out.PKScr) == 34 && out.PKScr[0] == 0 && out.PKScr[1] == 32
}
