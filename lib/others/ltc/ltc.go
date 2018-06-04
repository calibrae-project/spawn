package ltc

import (
	"bytes"

	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/others/utils"
	"github.com/ParallelCoinTeam/duod/lib/utxo"
)

// LTCAddrVersion -
const LTCAddrVersion = 48

// HashFromMessage - LTC signing uses different seed string
func HashFromMessage(msg []byte, out []byte) {
	const MessageMagic = "Litecoin Signed Message:\n"
	b := new(bytes.Buffer)
	btc.WriteVlen(b, uint64(len(MessageMagic)))
	b.Write([]byte(MessageMagic))
	btc.WriteVlen(b, uint64(len(msg)))
	b.Write(msg)
	btc.ShaHash(b.Bytes(), out)
}

// AddrVerPubkey -
func AddrVerPubkey(testnet bool) byte {
	if !testnet {
		return LTCAddrVersion
	}
	return btc.AddrVerPubkey(testnet)
}

// NewAddrFromPkScript -
func NewAddrFromPkScript(scr []byte, testnet bool) (ad *btc.Addr) {
	ad = btc.NewAddrFromPkScript(scr, testnet)
	if ad != nil && ad.Version == btc.AddrVerPubkey(false) {
		ad.Version = LTCAddrVersion
	}
	return
}

// GetUnspent -
func GetUnspent(addr *btc.Addr) (res utxo.AllUnspentTx) {
	var er error

	res, er = utils.GetUnspentFromBlockcypher(addr, "ltc")
	if er == nil {
		return
	}
	println("GetUnspentFromBlockcypher:", er.Error())

	return
}

// GetTxFromWeb - Download testnet's raw transaction from a web server
func GetTxFromWeb(txid *btc.Uint256) (raw []byte) {
	raw = utils.GetTxFromBlockcypher(txid, "ltc")
	if raw != nil && txid.Equal(btc.NewSha2Hash(raw)) {
		//println("GetTxFromBlockcypher - OK")
		return
	}

	return
}
