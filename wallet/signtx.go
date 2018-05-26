package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/ParallelCoinTeam/duod/lib/btc"
)

// prepare a signed transaction
func signTx(tx *btc.Tx) (allSigned bool) {
	var multisigDone bool
	allSigned = true

	// go through each input
	for in := range tx.TxIn {
		if ms, _ := btc.NewMultiSigFromScript(tx.TxIn[in].ScriptSig); ms != nil {
			hash := tx.SignatureHash(ms.P2SH(), in, btc.SigHashAll)
			for ki := range ms.PublicKeys {
				k := publicToKey(ms.PublicKeys[ki])
				if k != nil {
					r, s, e := btc.EcdsaSign(k.Key, hash)
					if e != nil {
						println("ERROR in signTx:", e.Error())
						allSigned = false
					} else {
						btcsig := &btc.Signature{HashType: 0x01}
						btcsig.R.Set(r)
						btcsig.S.Set(s)

						ms.Signatures = append(ms.Signatures, btcsig)
						tx.TxIn[in].ScriptSig = ms.Bytes()
						multisigDone = true
					}
				}
			}
		} else {
			uo := getUO(&tx.TxIn[in].Input)
			if uo == nil {
				println("ERROR: Unkown input:", tx.TxIn[in].Input.String(), "- missing balance folder?")
				allSigned = false
				continue
			}
			adr := addrFromPkscr(uo.PkScript)
			if adr == nil {
				fmt.Println("WARNING: Don't know how to sign input number", in)
				fmt.Println(" PkScript:", hex.EncodeToString(uo.PkScript))
				allSigned = false
				continue
			}

			ver, segwitProg := btc.IsWitnessProgram(uo.PkScript)
			if len(segwitProg) == 20 && ver == 0 {
				copy(adr.Hash160[:], segwitProg) // native segwith P2WPKH output
			}

			keyIdx := hashToKeyIdx(adr.Hash160[:])
			if keyIdx < 0 {
				fmt.Println("WARNING: You do not have key for", adr.String(), "at input", in)
				allSigned = false
				continue
			}
			var er error
			k := keys[keyIdx]
			if segwitProg != nil {
				er = tx.SignWitness(in, k.Addr.OutScript(), uo.Value, btc.SigHashAll, k.Addr.Pubkey, k.Key)
			} else if adr.String() == segwit[keyIdx].String() {
				tx.TxIn[in].ScriptSig = append([]byte{22, 0, 20}, k.Addr.Hash160[:]...)
				er = tx.SignWitness(in, k.Addr.OutScript(), uo.Value, btc.SigHashAll, k.Addr.Pubkey, k.Key)
			} else {
				er = tx.Sign(in, uo.PkScript, btc.SigHashAll, k.Addr.Pubkey, k.Key)
			}
			if er != nil {
				fmt.Println("ERROR: Sign failed for input number", in, er.Error())
				allSigned = false
			}
		}
	}

	// reorder signatures if we signed any multisig inputs
	if multisigDone && !multisigReorder(tx) {
		allSigned = false
	}

	if !allSigned {
		fmt.Println("WARNING: Not all the inputs have been signed")
	}

	return
}

func writeTxFile(tx *btc.Tx) {
	var signedrawtx []byte
	if tx.SegWit != nil {
		signedrawtx = tx.SerializeNew()
	} else {
		signedrawtx = tx.Serialize()
	}
	tx.SetHash(signedrawtx)

	hs := tx.Hash.String()
	fmt.Println("TxID", hs)

	var fn string

	if txfilename == "" {
		fn = hs[:8] + ".txt"
	} else {
		fn = txfilename
	}

	f, _ := os.Create(fn)
	if f != nil {
		f.Write([]byte(hex.EncodeToString(signedrawtx)))
		f.Close()
		fmt.Println("Transaction data stored in", fn)
	}
}

// prepare a signed transaction
func makeSignedTx() {
	// Make an empty transaction
	tx := new(btc.Tx)
	tx.Version = 1
	tx.LockTime = 0

	// Select as many inputs as we need to pay the full amount (with the fee)
	var btcsofar uint64
	for i := range unspentOuts {
		if unspentOuts[i].key == nil {
			continue
		}
		uo := getUO(&unspentOuts[i].TxPrevOut)
		// add the input to our transaction:
		tin := new(btc.TxIn)
		tin.Input = unspentOuts[i].TxPrevOut
		tin.Sequence = uint32(*sequence)
		tx.TxIn = append(tx.TxIn, tin)

		btcsofar += uo.Value
		unspentOuts[i].spent = true
		if !*useallinputs && (btcsofar >= spendBtc+feeBtc) {
			break
		}
	}
	if btcsofar < (spendBtc + feeBtc) {
		fmt.Println("ERROR: You have", btc.UintToBtc(btcsofar), "BTC, but you need",
			btc.UintToBtc(spendBtc+feeBtc), "BTC for the transaction")
		cleanExit(1)
	}
	changeBtc = btcsofar - (spendBtc + feeBtc)
	if *verbose {
		fmt.Printf("Spending %d out of %d outputs...\n", len(tx.TxIn), len(unspentOuts))
	}

	// Build transaction outputs:
	for o := range sendTo {
		outs, er := btc.NewSpendOutputs(sendTo[o].addr, sendTo[o].amount, testnet)
		if er != nil {
			fmt.Println("ERROR:", er.Error())
			cleanExit(1)
		}
		tx.TxOut = append(tx.TxOut, outs...)
	}

	if changeBtc > 0 {
		// Add one more output (with the change)
		chad := getChangeAddr()
		if *verbose {
			fmt.Println("Sending change", changeBtc, "to", chad.String())
		}
		outs, er := btc.NewSpendOutputs(chad, changeBtc, testnet)
		if er != nil {
			fmt.Println("ERROR:", er.Error())
			cleanExit(1)
		}
		tx.TxOut = append(tx.TxOut, outs...)
	}

	if *message != "" {
		// Add NULL output with an arbitrary message
		scr := new(bytes.Buffer)
		scr.WriteByte(0x6a) // OP_RETURN
		btc.WritePutLen(scr, uint32(len(*message)))
		scr.Write([]byte(*message))
		tx.TxOut = append(tx.TxOut, &btc.TxOut{Value: 0, PkScript: scr.Bytes()})
	}

	signed := signTx(tx)
	writeTxFile(tx)

	if apply2bal && signed {
		applyToBalance(tx)
	}
}

// sign raw transaction with all the keys we have
func processRawTx() {
	tx := rawTxFromFile(*rawtx)
	if tx == nil {
		fmt.Println("ERROR: Cannot decode the raw transaction")
		return
	}

	signTx(tx)
	writeTxFile(tx)
}
