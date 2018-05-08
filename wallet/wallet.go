package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/calibrae-project/spawn/lib/btc"
	"github.com/calibrae-project/spawn/lib/others/sys"
)

var (
	type2Secret    []byte // used to type-2 wallets
	firstDetermIdx int
	// set in makeWallet():
	keys   []*btc.PrivateAddr
	segwit []*btc.BtcAddr
	curFee uint64
)

// load private keys fo .others file
func loadOthers() {
	f, e := os.Open(RawKeysFilename)
	if e == nil {
		defer f.Close()
		td := bufio.NewReader(f)
		for {
			li, _, _ := td.ReadLine()
			if li == nil {
				break
			}
			if len(li) == 0 {
				continue
			}
			pk := strings.SplitN(strings.Trim(string(li), " "), " ", 2)
			if pk[0][0] == '#' {
				continue // Just a comment-line
			}

			rec, er := btc.DecodePrivateAddr(pk[0])
			if er != nil {
				println("DecodePrivateAddr error:", er.Error())
				if *verbose {
					println(pk[0])
				}
				continue
			}
			if rec.Version != verSecret() {
				println(pk[0][:6], "has version", rec.Version, "while we expect", verSecret())
				fmt.Println("You may want to play with -t or -ltc switch")
			}
			if len(pk) > 1 {
				rec.BtcAddr.Extra.Label = pk[1]
			} else {
				rec.BtcAddr.Extra.Label = fmt.Sprint("Other ", len(keys))
			}
			keys = append(keys, rec)
		}
		if *verbose {
			fmt.Println(len(keys), "keys imported from", RawKeysFilename)
		}
	} else {
		if *verbose {
			fmt.Println("You can also have some dumped (b58 encoded) Key keys in file", RawKeysFilename)
		}
	}
}

// Get the secret seed and generate "keycnt" key pairs (both private and public)
func makeWallet() {
	var lab string

	loadOthers()

	var seedKey []byte
	var hdwal *btc.HDWallet

	defer func() {
		sys.ClearBuffer(seedKey)
		if hdwal != nil {
			sys.ClearBuffer(hdwal.Key)
			sys.ClearBuffer(hdwal.ChCode)
		}
	}()

	pass := getpass()
	if pass == nil {
		cleanExit(0)
	}

	if waltype >= 1 && waltype <= 3 {
		seedKey = make([]byte, 32)
		btc.ShaHash(pass, seedKey)
		sys.ClearBuffer(pass)
		lab = fmt.Sprintf("Typ%c", 'A'+waltype-1)
		if waltype == 1 {
			println("WARNING: Wallet Type 1 is obsolete")
		} else if waltype == 2 {
			if type2sec != "" {
				d, e := hex.DecodeString(type2sec)
				if e != nil {
					println("t2sec error:", e.Error())
					cleanExit(1)
				}
				type2Secret = d
			} else {
				type2Secret = make([]byte, 20)
				btc.RimpHash(seedKey, type2Secret)
			}
		}
	} else if waltype == 4 {
		lab = "TypHD"
		hdwal = btc.MasterKey(pass, testnet)
		sys.ClearBuffer(pass)
	} else {
		sys.ClearBuffer(pass)
		println("ERROR: Unsupported wallet type", waltype)
		cleanExit(1)
	}

	if *verbose {
		fmt.Println("Generating", keycnt, "keys, version", verPubkey(), "...")
	}

	firstDetermIdx = len(keys)
	for i := uint(0); i < keycnt; {
		privKey := make([]byte, 32)
		if waltype == 3 {
			btc.ShaHash(seedKey, privKey)
			seedKey = append(seedKey, byte(i))
		} else if waltype == 2 {
			seedKey = btc.DeriveNextPrivate(seedKey, type2Secret)
			copy(privKey, seedKey)
		} else if waltype == 1 {
			btc.ShaHash(seedKey, privKey)
			copy(seedKey, privKey)
		} else /*if waltype==4*/ {
			// HD wallet
			_hd := hdwal.Child(uint32(0x80000000 | i))
			copy(privKey, _hd.Key[1:])
			sys.ClearBuffer(_hd.Key)
			sys.ClearBuffer(_hd.ChCode)
		}

		rec := btc.NewPrivateAddr(privKey, verSecret(), !uncompressed)

		if *pubkey != "" && *pubkey == rec.BtcAddr.String() {
			fmt.Println("Public address:", rec.BtcAddr.String())
			fmt.Println("Public hexdump:", hex.EncodeToString(rec.BtcAddr.Pubkey))
			return
		}

		rec.BtcAddr.Extra.Label = fmt.Sprint(lab, " ", i+1)
		keys = append(keys, rec)
		i++
	}
	if *verbose {
		fmt.Println("Private keys re-generated")
	}

	// Calculate SegWit addresses
	segwit = make([]*btc.BtcAddr, len(keys))
	for i, pk := range keys {
		if len(pk.Pubkey) != 33 {
			continue
		}
		if *bech32Mode {
			segwit[i] = btc.NewAddrFromPkScript(append([]byte{0, 20}, pk.Hash160[:]...), testnet)
		} else {
			h160 := btc.Rimp160AfterSha256(append([]byte{0, 20}, pk.Hash160[:]...))
			segwit[i] = btc.NewAddrFromHash160(h160[:], btc.AddrVerScript(testnet))
		}
	}
}

// Print all the public addresses
func dumpAddrs() {
	f, _ := os.Create("wallet.txt")

	fmt.Fprintln(f, "# Deterministic Walet Type", waltype)
	if type2Secret != nil {
		fmt.Fprintln(f, "#", hex.EncodeToString(keys[firstDetermIdx].BtcAddr.Pubkey))
		fmt.Fprintln(f, "#", hex.EncodeToString(type2Secret))
	}
	for i := range keys {
		if !*noverify {
			if er := btc.VerifyKeyPair(keys[i].Key, keys[i].BtcAddr.Pubkey); er != nil {
				println("Something wrong with key at index", i, " - abort!", er.Error())
				cleanExit(1)
			}
		}
		var pubaddr string
		if *segwitMode {
			if segwit[i] == nil {
				pubaddr = "-=CompressedKey=-"
			} else {
				pubaddr = segwit[i].String()
			}
		} else {
			pubaddr = keys[i].BtcAddr.String()
		}
		fmt.Println(pubaddr, keys[i].BtcAddr.Extra.Label)
		if f != nil {
			fmt.Fprintln(f, pubaddr, keys[i].BtcAddr.Extra.Label)
		}
	}
	if f != nil {
		f.Close()
		fmt.Println("You can find all the addresses in wallet.txt file")
	}
}

func publicToKey(pubkey []byte) *btc.PrivateAddr {
	for i := range keys {
		if bytes.Equal(pubkey, keys[i].BtcAddr.Pubkey) {
			return keys[i]
		}
	}
	return nil
}

func hashToKeyIdx(h160 []byte) (res int) {
	for i := range keys {
		if bytes.Equal(keys[i].BtcAddr.Hash160[:], h160) {
			return i
		}
		if segwit[i] != nil && bytes.Equal(segwit[i].Hash160[:], h160) {
			return i
		}
	}
	return -1
}

func hashToKey(h160 []byte) *btc.PrivateAddr {
	if i := hashToKeyIdx(h160); i >= 0 {
		return keys[i]
	}
	return nil
}

func addressToKey(addr string) *btc.PrivateAddr {
	a, e := btc.NewAddrFromString(addr)
	if e != nil {
		println("Cannot Decode address", addr)
		println(e.Error())
		cleanExit(1)
	}
	return hashToKey(a.Hash160[:])
}

// suuports only P2KH scripts
func pkscrToKey(scr []byte) *btc.PrivateAddr {
	if len(scr) == 25 && scr[0] == 0x76 && scr[1] == 0xa9 && scr[2] == 0x14 && scr[23] == 0x88 && scr[24] == 0xac {
		return hashToKey(scr[3:23])
	}
	// P2SH(WPKH)
	if len(scr) == 23 && scr[0] == 0xa9 && scr[22] == 0x87 {
		return hashToKey(scr[2:22])
	}
	// P2WPKH
	if len(scr) == 22 && scr[0] == 0x00 && scr[1] == 0x14 {
		return hashToKey(scr[2:])
	}
	return nil
}

func dumpPrivKey() {
	if *dumppriv == "*" {
		// Dump all private keys
		for i := range keys {
			fmt.Println(keys[i].String(), keys[i].BtcAddr.String(), keys[i].BtcAddr.Extra.Label)
		}
	} else {
		// single key
		k := addressToKey(*dumppriv)
		if k != nil {
			fmt.Println("Public address:", k.BtcAddr.String(), k.BtcAddr.Extra.Label)
			fmt.Println("Public hexdump:", hex.EncodeToString(k.BtcAddr.Pubkey))
			fmt.Println("Public compressed:", k.BtcAddr.IsCompressed())
			fmt.Println("Private encoded:", k.String())
			fmt.Println("Private hexdump:", hex.EncodeToString(k.Key))
		} else {
			println("Dump Private Key:", *dumppriv, "not found it the wallet")
		}
	}
}
