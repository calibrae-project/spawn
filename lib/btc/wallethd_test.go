/*
This code is taken from:
 * https://github.com/WeMeetAgain/go-hdwallet
*/

package btc

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// implements https://github.com/bitcoin/bips/blob/master/bip-0032.mediawiki#test-vectors

var (
	masterhex1                      = "000102030405060708090a0b0c0d0e0f"
	mPub1                           = "xpub661MyMwAqRbcFtXgS5sYJABqqG9YLmC4Q1Rdap9gSE8NqtwybGhePY2gZ29ESFjqJoCu1Rupje8YtGqsefD265TMg7usUDFdp6W1EGMcet8"
	mPriv1                          = "xprv9s21ZrQH143K3QTDL4LXw2F7HEK3wJUD2nW2nRk4stbPy6cq3jPPqjiChkVvvNKmPGJxWUtg6LnF5kejMRNNU3TGtRBeJgk33yuGBxrMPHi"
	m0pPub1                         = "xpub68Gmy5EdvgibQVfPdqkBBCHxA5htiqg55crXYuXoQRKfDBFA1WEjWgP6LHhwBZeNK1VTsfTFUHCdrfp1bgwQ9xv5ski8PX9rL2dZXvgGDnw"
	m0pPriv1                        = "xprv9uHRZZhk6KAJC1avXpDAp4MDc3sQKNxDiPvvkX8Br5ngLNv1TxvUxt4cV1rGL5hj6KCesnDYUhd7oWgT11eZG7XnxHrnYeSvkzY7d2bhkJ7"
	m0p1Pub1                        = "xpub6ASuArnXKPbfEwhqN6e3mwBcDTgzisQN1wXN9BJcM47sSikHjJf3UFHKkNAWbWMiGj7Wf5uMash7SyYq527Hqck2AxYysAA7xmALppuCkwQ"
	m0p1Priv1                       = "xprv9wTYmMFdV23N2TdNG573QoEsfRrWKQgWeibmLntzniatZvR9BmLnvSxqu53Kw1UmYPxLgboyZQaXwTCg8MSY3H2EU4pWcQDnRnrVA1xe8fs"
	m0p12pPub1                      = "xpub6D4BDPcP2GT577Vvch3R8wDkScZWzQzMMUm3PWbmWvVJrZwQY4VUNgqFJPMM3No2dFDFGTsxxpG5uJh7n7epu4trkrX7x7DogT5Uv6fcLW5"
	m0p12pPriv1                     = "xprv9z4pot5VBttmtdRTWfWQmoH1taj2axGVzFqSb8C9xaxKymcFzXBDptWmT7FwuEzG3ryjH4ktypQSAewRiNMjANTtpgP4mLTj34bhnZX7UiM"
	m0p12p2Pub1                     = "xpub6FHa3pjLCk84BayeJxFW2SP4XRrFd1JYnxeLeU8EqN3vDfZmbqBqaGJAyiLjTAwm6ZLRQUMv1ZACTj37sR62cfN7fe5JnJ7dh8zL4fiyLHV"
	m0p12p2Priv1                    = "xprvA2JDeKCSNNZky6uBCviVfJSKyQ1mDYahRjijr5idH2WwLsEd4Hsb2Tyh8RfQMuPh7f7RtyzTtdrbdqqsunu5Mm3wDvUAKRHSC34sJ7in334"
	m0p12p21000000000Pub1           = "xpub6H1LXWLaKsWFhvm6RVpEL9P4KfRZSW7abD2ttkWP3SSQvnyA8FSVqNTEcYFgJS2UaFcxupHiYkro49S8yGasTvXEYBVPamhGW6cFJodrTHy"
	m0p12p21000000000Priv1          = "xprvA41z7zogVVwxVSgdKUHDy1SKmdb533PjDz7J6N6mV6uS3ze1ai8FHa8kmHScGpWmj4WggLyQjgPie1rFSruoUihUZREPSL39UNdE3BBDu76"
	masterhex2                      = "fffcf9f6f3f0edeae7e4e1dedbd8d5d2cfccc9c6c3c0bdbab7b4b1aeaba8a5a29f9c999693908d8a8784817e7b7875726f6c696663605d5a5754514e4b484542"
	mPub2                           = "xpub661MyMwAqRbcFW31YEwpkMuc5THy2PSt5bDMsktWQcFF8syAmRUapSCGu8ED9W6oDMSgv6Zz8idoc4a6mr8BDzTJY47LJhkJ8UB7WEGuduB"
	mPriv2                          = "xprv9s21ZrQH143K31xYSDQpPDxsXRTUcvj2iNHm5NUtrGiGG5e2DtALGdso3pGz6ssrdK4PFmM8NSpSBHNqPqm55Qn3LqFtT2emdEXVYsCzC2U"
	m0Pub2                          = "xpub69H7F5d8KSRgmmdJg2KhpAK8SR3DjMwAdkxj3ZuxV27CprR9LgpeyGmXUbC6wb7ERfvrnKZjXoUmmDznezpbZb7ap6r1D3tgFxHmwMkQTPH"
	m0Priv2                         = "xprv9vHkqa6EV4sPZHYqZznhT2NPtPCjKuDKGY38FBWLvgaDx45zo9WQRUT3dKYnjwih2yJD9mkrocEZXo1ex8G81dwSM1fwqWpWkeS3v86pgKt"
	m02147483647pPub2               = "xpub6ASAVgeehLbnwdqV6UKMHVzgqAG8Gr6riv3Fxxpj8ksbH9ebxaEyBLZ85ySDhKiLDBrQSARLq1uNRts8RuJiHjaDMBU4Zn9h8LZNnBC5y4a"
	m02147483647pPriv2              = "xprv9wSp6B7kry3Vj9m1zSnLvN3xH8RdsPP1Mh7fAaR7aRLcQMKTR2vidYEeEg2mUCTAwCd6vnxVrcjfy2kRgVsFawNzmjuHc2YmYRmagcEPdU9"
	m02147483647p1Pub2              = "xpub6DF8uhdarytz3FWdA8TvFSvvAh8dP3283MY7p2V4SeE2wyWmG5mg5EwVvmdMVCQcoNJxGoWaU9DCWh89LojfZ537wTfunKau47EL2dhHKon"
	m02147483647p1Priv2             = "xprv9zFnWC6h2cLgpmSA46vutJzBcfJ8yaJGg8cX1e5StJh45BBciYTRXSd25UEPVuesF9yog62tGAQtHjXajPPdbRCHuWS6T8XA2ECKADdw4Ef"
	m02147483647p12147483646pPub2   = "xpub6ERApfZwUNrhLCkDtcHTcxd75RbzS1ed54G1LkBUHQVHQKqhMkhgbmJbZRkrgZw4koxb5JaHWkY4ALHY2grBGRjaDMzQLcgJvLJuZZvRcEL"
	m02147483647p12147483646pPriv2  = "xprvA1RpRA33e1JQ7ifknakTFpgNXPmW2YvmhqLQYMmrj4xJXXWYpDPS3xz7iAxn8L39njGVyuoseXzU6rcxFLJ8HFsTjSyQbLYnMpCqE2VbFWc"
	m02147483647p12147483646p2Pub2  = "xpub6FnCn6nSzZAw5Tw7cgR9bi15UV96gLZhjDstkXXxvCLsUXBGXPdSnLFbdpq8p9HmGsApME5hQTZ3emM2rnY5agb9rXpVGyy3bdW6EEgAtqt"
	m02147483647p12147483646p2Priv2 = "xprvA2nrNbFZABcdryreWet9Ea4LvTJcGsqrMzxHx98MMrotbir7yrKCEXw7nadnHM8Dq38EGfSh6dqA9QWTyefMLEcBYJUuekgW4BYPJcr9E7j"
)

func testChild(t *testing.T, key, refKey string, i uint32) {
	childKey := StringChild(key, i)
	if childKey != refKey {
		t.Errorf("\n%s\nsupposed to be\n%s", childKey, refKey)
	}
}

func testMasterKey(t *testing.T, seed []byte, refKey string) {
	masterprv := MasterKey(seed, false).String()
	if masterprv != refKey {
		t.Errorf("\n%s\nsupposed to be\n%s", masterprv, refKey)
	}
}

func testPub(t *testing.T, prv, refPub string) {
	w, err := StringWallet(prv)
	if err != nil {
		t.Errorf("%s should have been nil", err.Error())
	}
	pub := w.Pub().String()
	if pub != refPub {
		t.Errorf("\n%s\nsupposed to be\n%s", pub, refPub)
	}
}

func TestVector1(t *testing.T) {
	seed, _ := hex.DecodeString(masterhex1)
	t.Logf("master key")
	testMasterKey(t, seed, mPriv1)
	t.Logf("master key -> pub")
	testPub(t, mPriv1, mPub1)
	var i uint32
	i = 0x80000000
	t.Logf("first child")
	testChild(t, mPriv1, m0pPriv1, i)
	t.Logf("first child -> pub")
	testPub(t, m0pPriv1, m0pPub1)
	t.Logf("second child")
	testChild(t, m0pPriv1, m0p1Priv1, 1)
	t.Logf("second child -> pub")
	testPub(t, m0p1Priv1, m0p1Pub1)
	t.Logf("third child")
	i = 0x80000002
	testChild(t, m0p1Priv1, m0p12pPriv1, i)
	t.Logf("third child -> pub")
	testPub(t, m0p12pPriv1, m0p12pPub1)
	t.Logf("fourth child")
	testChild(t, m0p12pPriv1, m0p12p2Priv1, 2)
	t.Logf("fourth child -> pub")
	testPub(t, m0p12p2Priv1, m0p12p2Pub1)
	t.Logf("fifth child")
	i = 1000000000 % 0x80000000
	testChild(t, m0p12p2Priv1, m0p12p21000000000Priv1, i)
	t.Logf("fifth child -> pub")
	testPub(t, m0p12p21000000000Priv1, m0p12p21000000000Pub1)
}

func TestVector2(t *testing.T) {
	seed, _ := hex.DecodeString(masterhex2)
	t.Logf("master key")
	testMasterKey(t, seed, mPriv2)
	t.Logf("master key -> pub")
	testPub(t, mPriv2, mPub2)
	var i uint32
	t.Logf("first child")
	testChild(t, mPriv2, m0Priv2, 0)
	t.Logf("first child -> pub")
	testPub(t, m0Priv2, m0Pub2)
	i = 2147483647 + 0x80000000
	t.Logf("second child")
	testChild(t, m0Priv2, m02147483647pPriv2, i)
	t.Logf("second child -> pub")
	testPub(t, m02147483647pPriv2, m02147483647pPub2)
	t.Logf("third child")
	testChild(t, m02147483647pPriv2, m02147483647p1Priv2, 1)
	t.Logf("third child -> pub")
	testPub(t, m02147483647p1Priv2, m02147483647p1Pub2)
	i = 2147483646 + 0x80000000
	t.Logf("fourth child")
	testChild(t, m02147483647p1Priv2, m02147483647p12147483646pPriv2, i)
	t.Logf("fourth child -> pub")
	testPub(t, m02147483647p12147483646pPriv2, m02147483647p12147483646pPub2)
	t.Logf("fifth child")
	testChild(t, m02147483647p12147483646pPriv2, m02147483647p12147483646p2Priv2, 2)
	t.Logf("fifth child -> pub")
	testPub(t, m02147483647p12147483646p2Priv2, m02147483647p12147483646p2Pub2)
}

func TestChildPub(t *testing.T) {
	testChild(t, mPub2, m0Pub2, 0)
}

func TestChildPrv(t *testing.T) {
	testChild(t, mPriv2, m0Priv2, 0)
}

func TestSerialize(t *testing.T) {
	w, err := StringWallet(mPriv2)
	if err != nil {
		t.Errorf("%s should have been nil", err.Error())
	}
	if mPriv2 != w.String() {
		t.Errorf("private key not de/reserializing properly")
	}
	w, err = StringWallet(mPub2)
	if err != nil {
		t.Errorf("%s should have been nil", err.Error())
	}
	if mPub2 != w.String() {
		t.Errorf("public key not de/reserializing properly")
	}
}

// Used this site to create test http://gobittest.appspot.com/Address
// Public key: 04CBCAA9C98C877A26977D00825C956A238E8DDDFBD322CCE4F74B0B5BD6ACE4A77BD3305D363C26F82C1E41C667E4B3561C06C60A2104D2B548E6DD059056AA51
// Expected address: 1AEg9dFEw29kMgaN4BNHALu7AzX5XUfzSU
func TestAddress(t *testing.T) {
	addr, err := StringAddress(mPub2)
	if err != nil {
		t.Errorf("%s should have been nil", err.Error())
	}
	expectedAddr := "1JEoxevbLLG8cVqeoGKQiAwoWbNYSUyYjg"
	if addr != expectedAddr {
		t.Errorf("\n%s\nshould be\n%s", addr, expectedAddr)
	}
}

func TestStringCheck(t *testing.T) {
	if err := StringCheck(mPub2); err != nil {
		t.Errorf("%s should have been nil", err.Error())
	}
	if err := StringCheck(mPriv2); err != nil {
		t.Errorf("%s should have been nil", err.Error())
	}
}

func TestChildren(t *testing.T) {
	hdwal := MasterKey([]byte("Random seed"), false)
	hdpub := hdwal.Pub()

	for i := 0; i < 1000; i++ {
		prv := hdwal.Child(uint32(i | 0x80000000))
		if len(prv.Key) != 33 || prv.Key[0] != 0 {
			t.Error("Bad private derivated key", i)
		}

		prv = hdwal.Child(uint32(i))
		pub := hdpub.Child(uint32(i))
		if len(prv.Key) != 33 || prv.Key[0] != 0 {
			t.Error("Bad private key", i)
		}
		if len(pub.Key) != 33 || (pub.Key[0] != 2 && pub.Key[0] != 3) {
			t.Error("Bad public key", i)
		}
		pu2 := PublicFromPrivate(prv.Key[1:], true)
		if !bytes.Equal(pub.Key, pu2) {
			t.Error("Private/public mismatch on Child", i)
		}

		var p [32]byte
		copy(p[:], prv.Key[1:])
		pu2 = PublicFromPrivate(p[:], true)
		if !bytes.Equal(pub.Key, pu2) {
			t.Error("Private/public other mismatch on Child", i)
		}
	}
}

// benchmarks

func BenchmarkStringChildPub(b *testing.B) {
	for i := 0; i < b.N; i++ {
		StringChild(mPub2, 0)
	}
}

func BenchmarkStringChildPrv(b *testing.B) {
	var a uint32
	a = 0x80000000
	for i := 0; i < b.N; i++ {
		StringChild(mPriv1, a)
	}
}

func BenchmarkStringPubString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		w, _ := StringWallet(mPriv2)
		_ = w.Pub().String()
	}
}

func BenchmarkStringAddress(b *testing.B) {
	for i := 0; i < b.N; i++ {
		StringAddress(mPub2)
	}
}

func BenchmarkStringCheck(b *testing.B) {
	for i := 0; i < b.N; i++ {
		StringCheck(mPub2)
	}
}
