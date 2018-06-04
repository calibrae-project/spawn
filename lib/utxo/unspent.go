package utxo

import (
	"encoding/binary"
	"fmt"

	"github.com/ParallelCoinTeam/duod/lib/btc"
)

// AllUnspentTx -
type AllUnspentTx []*OneUnspentTx

// OneUnspentTx - Returned by GetUnspentFromPkScr
type OneUnspentTx struct {
	btc.TxPrevOut
	Value   uint64
	MinedAt uint32
	*btc.Addr
	destString string
	Coinbase   bool
	Message    []byte
}

func (x AllUnspentTx) Len() int {
	return len(x)
}

func (x AllUnspentTx) Less(i, j int) bool {
	if x[i].MinedAt == x[j].MinedAt {
		if x[i].TxPrevOut.Hash == x[j].TxPrevOut.Hash {
			return x[i].TxPrevOut.Vout < x[j].TxPrevOut.Vout
		}
		return binary.LittleEndian.Uint64(x[i].TxPrevOut.Hash[24:32]) <
			binary.LittleEndian.Uint64(x[j].TxPrevOut.Hash[24:32])
	}
	return x[i].MinedAt < x[j].MinedAt
}

func (x AllUnspentTx) Swap(i, j int) {
	x[i], x[j] = x[j], x[i]
}

func (ou *OneUnspentTx) String() (s string) {
	s = fmt.Sprintf("%15s BTC %s", btc.UintToBtc(ou.Value), ou.TxPrevOut.String())
	if ou.Addr != nil {
		s += " " + ou.DestAddr() + ou.Addr.Label()
	}
	if ou.MinedAt != 0 {
		s += fmt.Sprint(" ", ou.MinedAt)
	}
	if ou.Coinbase {
		s += fmt.Sprint(" Coinbase")
	}
	if ou.Message != nil {
		s += "  "
		for _, c := range ou.Message {
			if c < ' ' || c > 127 {
				s += fmt.Sprintf("\\x%02x", c)
			} else {
				s += string(c)
			}
		}
	}
	return
}

// FixDestString -
func (ou *OneUnspentTx) FixDestString() {
	ou.destString = ou.Addr.String()
}

// UnspentTextLine -
func (ou *OneUnspentTx) UnspentTextLine() (s string) {
	s = fmt.Sprintf("%s # %.8f BTC @ %s%s, block %d", ou.TxPrevOut.String(),
		float64(ou.Value)/1e8, ou.DestAddr(), ou.Addr.Label(), ou.MinedAt)
	return
}

// DestAddr -
func (ou *OneUnspentTx) DestAddr() string {
	if ou.destString == "" {
		return ou.Addr.String()
	}
	return ou.destString
}
