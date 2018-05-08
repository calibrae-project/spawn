package bech32

import (
	"bytes"
)

// Return nil on error
func convertBits(outbits uint, in []byte, inbits uint, pad bool) []byte {
	var val uint32
	var bits uint
	maxv := uint32(1<<outbits) - 1
	out := new(bytes.Buffer)
	for inx := range in {
		val = (val << inbits) | uint32(in[inx])
		bits += inbits
		for bits >= outbits {
			bits -= outbits
			out.WriteByte(byte((val >> bits) & maxv))
		}
	}
	if pad {
		if bits != 0 {
			out.WriteByte(byte((val << (outbits - bits)) & maxv))
		}
	} else if ((val<<(outbits-bits))&maxv) != 0 || bits >= inbits {
		return nil
	}
	return out.Bytes()
}

// SegwitEncode - Returns empty string on error
func SegwitEncode(hrp string, witver int, witprog []byte) string {
	if witver > 16 {
		return ""
	}
	if witver == 0 && len(witprog) != 20 && len(witprog) != 32 {
		return ""
	}
	if len(witprog) < 2 || len(witprog) > 40 {
		return ""
	}
	return Encode(hrp, append([]byte{byte(witver)}, convertBits(5, witprog, 8, true)...))
}

// SegwitDecode - returns (0, nil) on error
func SegwitDecode(hrp, addr string) (witver int, witdata []byte) {
	hrpActual, data := Decode(addr)
	if hrpActual == "" || len(data) == 0 || len(data) > 65 {
		return
	}
	if hrp != hrpActual {
		return
	}
	if data[0] > 16 {
		return
	}
	witdata = convertBits(8, data[1:], 5, false)
	if witdata == nil {
		return
	}
	if len(witdata) < 2 || len(witdata) > 40 {
		witdata = nil
		return
	}
	if data[0] == 0 && len(witdata) != 20 && len(witdata) != 32 {
		witdata = nil
		return
	}
	witver = int(data[0])
	return
}
