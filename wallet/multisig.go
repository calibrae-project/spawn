package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"

	"github.com/calibrae-project/spawn/lib/btc"
)

// MultiToSignOut -
const MultiToSignOut = "multi2sign.txt"

// add P2SH pre-signing data into a raw tx
func makeP2sh() {
	tx := rawTxFromFile(*rawtx)
	if tx == nil {
		fmt.Println("ERROR: Cannot decode the raw transaction")
		return
	}

	d, er := hex.DecodeString(*p2sh)
	if er != nil {
		println("P2SH hex data:", er.Error())
		return
	}

	ms, er := btc.NewMultiSigFromP2SH(d)
	if er != nil {
		println("Decode P2SH:", er.Error())
		return
	}

	fmt.Println("The P2SH data points to address", ms.Addr(testnet).String())

	sd := ms.Bytes()

	for i := range tx.TxIn {
		if *input < 0 || i == *input {
			tx.TxIn[i].ScriptSig = sd
			fmt.Println("Input number", i, " - hash to sign:", hex.EncodeToString(tx.SignatureHash(d, i, btc.SIGHASH_ALL)))
		}
	}
	ioutil.WriteFile(MultiToSignOut, []byte(hex.EncodeToString(tx.Serialize())), 0666)
	fmt.Println("Transaction with", len(tx.TxIn), "inputs ready for multi-signing, stored in", MultiToSignOut)
}

// reorder signatures to meet order of the keys
// remove signatuers made by the same keys
// remove exessive signatures (keeps transaction size down)
func multisigReorder(tx *btc.Tx) (allSigned bool) {
	allSigned = true
	for i := range tx.TxIn {
		ms, _ := btc.NewMultiSigFromScript(tx.TxIn[i].ScriptSig)
		if ms == nil {
			continue
		}
		hash := tx.SignatureHash(ms.P2SH(), i, btc.SIGHASH_ALL)

		var sigs []*btc.Signature
		for ki := range ms.PublicKeys {
			var sig *btc.Signature
			for si := range ms.Signatures {
				if btc.EcdsaVerify(ms.PublicKeys[ki], ms.Signatures[si].Bytes(), hash) {
					//fmt.Println("Key number", ki, "has signature number", si)
					sig = ms.Signatures[si]
					break
				}
			}
			if sig != nil {
				sigs = append(sigs, sig)
			} else if *verbose {
				fmt.Println("WARNING: Key number", ki, "has no matching signature")
			}

			if !*allowextramsigns && uint(len(sigs)) >= ms.SigsNeeded {
				break
			}
		}

		if *verbose {
			if len(ms.Signatures) > len(sigs) {
				fmt.Println("WARNING: Some signatures are obsolete and will be removed", len(ms.Signatures), "=>", len(sigs))
			} else if len(ms.Signatures) < len(sigs) {
				fmt.Println("It appears that same key is re-used.", len(sigs)-len(ms.Signatures), "more signatures were added")
			}
		}

		ms.Signatures = sigs
		tx.TxIn[i].ScriptSig = ms.Bytes()

		if len(sigs) < int(ms.SigsNeeded) {
			allSigned = false
		}
	}
	return
}

// sign a multisig transaction with a specific key
func multisigSign() {
	tx := rawTxFromFile(*rawtx)
	if tx == nil {
		println("ERROR: Cannot decode the raw multisig transaction")
		println("Always use -msign <addr> along with -raw multi2sign.txt")
		return
	}

	k := addressToKey(*multisign)
	if k == nil {
		println("You do not know a key for address", *multisign)
		return
	}

	for i := range tx.TxIn {
		ms, er := btc.NewMultiSigFromScript(tx.TxIn[i].ScriptSig)
		if er != nil {
			println("WARNING: Input", i, "- not multisig:", er.Error())
			continue
		}
		hash := tx.SignatureHash(ms.P2SH(), i, btc.SIGHASH_ALL)
		//fmt.Println("Input number", i, len(ms.Signatures), " - hash to sign:", hex.EncodeToString(hash))

		r, s, e := btc.EcdsaSign(k.Key, hash)
		if e != nil {
			println(e.Error())
			return
		}
		btcsig := &btc.Signature{HashType: 0x01}
		btcsig.R.Set(r)
		btcsig.S.Set(s)

		ms.Signatures = append(ms.Signatures, btcsig)
		tx.TxIn[i].ScriptSig = ms.Bytes()
	}

	// Now re-order the signatures as they shall be:
	multisigReorder(tx)

	writeTxFile(tx)
}
