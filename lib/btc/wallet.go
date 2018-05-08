package btc

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/calibrae-project/spawn/lib/secp256k1"
)

// PublicFromPrivate - Get ECDSA public key in bitcoin protocol format, from the give private key
func PublicFromPrivate(privKey []byte, compressed bool) (res []byte) {
	if compressed {
		res = make([]byte, 33)
	} else {
		res = make([]byte, 65)
	}

	if !secp256k1.BaseMultiply(privKey, res) {
		res = nil
	}
	return
}

// VerifyKeyPair - Verify the secret key's range and if a test message signed with it verifies OK
// Returns nil if everything looks OK
func VerifyKeyPair(priv []byte, publ []byte) error {
	var sig Signature

	const TestMessage = "Just some test message..."
	hash := Sha2Sum([]byte(TestMessage))

	D := new(big.Int).SetBytes(priv)

	if D.Cmp(big.NewInt(0)) == 0 {
		return errors.New("pubkey value is zero")
	}

	if D.Cmp(&secp256k1.TheCurve.Order.Int) != -1 {
		return errors.New("pubkey value is too big")
	}

	r, s, e := EcdsaSign(priv, hash[:])
	if e != nil {
		return errors.New("EcdsaSign failed: " + e.Error())
	}

	sig.R.Set(r)
	sig.S.Set(s)
	ok := EcdsaVerify(publ, sig.Bytes(), hash[:])
	if !ok {
		return errors.New("EcdsaVerify failed")
	}
	return nil
}

// DeriveNextPrivate - B_privateKey = ( A_privateKey + secret ) % N
// Used for implementing Type-2 determinitic keys
func DeriveNextPrivate(p, s []byte) (toreturn []byte) {
	var prv, secret big.Int
	prv.SetBytes(p)
	secret.SetBytes(s)
	res := new(big.Int).Mod(new(big.Int).Add(&prv, &secret), &secp256k1.TheCurve.Order.Int).Bytes()
	toreturn = make([]byte, 32)
	copy(toreturn[32-len(res):], res)
	return
}

// DeriveNextPublic - B_publicKey = G * secret + A_publicKey
// Used for implementing Type-2 determinitic keys
func DeriveNextPublic(public, secret []byte) (out []byte) {
	out = make([]byte, len(public))
	secp256k1.BaseMultiplyAdd(public, secret, out)
	return
}

// NewSpendOutputs - returns one TxOut record
func NewSpendOutputs(addr *Addr, amount uint64, testnet bool) ([]*TxOut, error) {
	out := new(TxOut)
	out.Value = amount
	out.PkScript = addr.OutScript()
	return []*TxOut{out}, nil
}

// PrivateAddr - Base58 encoded private address with checksum and it's corresponding public key/address
type PrivateAddr struct {
	Version byte
	Key     []byte
	*Addr
}

// NewPrivateAddr -
func NewPrivateAddr(key []byte, ver byte, compr bool) (ad *PrivateAddr) {
	ad = new(PrivateAddr)
	ad.Version = ver
	ad.Key = key
	pub := PublicFromPrivate(key, compr)
	if pub == nil {
		panic("PublicFromPrivate error")
	}
	ad.Addr = NewAddrFromPubkey(pub, ver-0x80)
	return
}

// DecodePrivateAddr -
func DecodePrivateAddr(s string) (*PrivateAddr, error) {
	pkb := DecodeBase58(s)

	if pkb == nil {
		return nil, errors.New("DecodeBase58 failed")
	}

	if len(pkb) < 37 {
		return nil, errors.New("Decoded data too short")
	}

	if len(pkb) > 38 {
		return nil, errors.New("Decoded data too long")
	}

	var sh [32]byte
	ShaHash(pkb[:len(pkb)-4], sh[:])
	if !bytes.Equal(sh[:4], pkb[len(pkb)-4:]) {
		return nil, errors.New("Checksum error")
	}

	return NewPrivateAddr(pkb[1:33], pkb[0], len(pkb) == 38 && pkb[33] == 1), nil
}

// Returns base58 encoded private key (with checksum)
func (ad *PrivateAddr) String() string {
	var ha [32]byte
	buf := new(bytes.Buffer)
	buf.WriteByte(ad.Version)
	buf.Write(ad.Key)
	if ad.Addr.IsCompressed() {
		buf.WriteByte(1)
	}
	ShaHash(buf.Bytes(), ha[:])
	buf.Write(ha[:4])
	return EncodeBase58(buf.Bytes())
}
