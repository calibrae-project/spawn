package script

import (
	//"os"
	//"fmt"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/calibrae-project/spawn/lib/btc"
)

type oneTestVector struct {
	sigscr, pkscr []byte
	flags         uint32
	expRes        bool
	desc          string

	witness [][]byte
	value   uint64
}

func TestScritps(t *testing.T) {
	var str interface{}
	var vecs []*oneTestVector

	DBG_ERR = false
	dat, er := ioutil.ReadFile("../test/script_tests.json")
	if er != nil {
		t.Error(er.Error())
		return
	}
	er = json.Unmarshal(dat, &str)
	if er != nil {
		t.Error(er.Error())
		return
	}

	m := str.([]interface{})
	for i := range m {
		switch mm := m[i].(type) {
		case []interface{}:
			if len(mm) < 4 {
				continue
			}

			var skip bool
			var bfield int
			var e error
			var allGood bool

			vec := new(oneTestVector)
			for ii := range mm {
				switch segwitdata := mm[ii].(type) {
				case []interface{}:
					for iii := range segwitdata {
						switch segwitdata[iii].(type) {
						case string:
							var by []byte
							s := segwitdata[iii].(string)
							by, e = hex.DecodeString(s)
							if e != nil {
								t.Error("error parsing serwit script", s)
								skip = true
								break
							}
							vec.witness = append(vec.witness, by)

						case float64:
							vec.value = uint64(1e8 * segwitdata[iii].(float64))
						}
					}

				case string:
					s := mm[ii].(string)
					if bfield == 0 {
						vec.sigscr, e = btc.DecodeScript(s)
						if e != nil {
							t.Error("error parsing script", s)
							skip = true
							break
						}
					} else if bfield == 1 {
						vec.pkscr, e = btc.DecodeScript(s)
						if e != nil {
							skip = true
							break
						}
					} else if bfield == 2 {
						vec.flags, e = decodeFlags(s)
						if e != nil {
							println("error parsing flag", e.Error())
							skip = true
							break
						}
					} else if bfield == 3 {
						vec.expRes = s == "OK"
						allGood = true
					} else if bfield == 4 {
						vec.desc = s
						skip = true
						break
					}
					bfield++

				default:
					panic("Enexpected test vector")
					// skip = true
				}
				if skip {
					break
				}
			}
			if allGood {
				vecs = append(vecs, vec)
			}
		}
	}

	tot := 0
	for _, v := range vecs {
		tot++

		/*
			if tot==114400 {
				DBG_SCR = true
				DBG_ERR = true
			}*/

		flags := v.flags
		if (flags & VER_CLEANSTACK) != 0 {
			flags |= VER_P2SH
			flags |= VER_WITNESS
		}

		creditTx := mkCreditTx(v.pkscr, v.value)
		spendTx := mkSpendTx(creditTx, v.sigscr, v.witness)

		if DBG_SCR {
			println("desc:", v, tot, v.desc)
			println("pkscr:", hex.EncodeToString(v.pkscr))
			println("sigscr:", hex.EncodeToString(v.sigscr))
			println("credit:", hex.EncodeToString(creditTx.Serialize()))
			println("spend:", hex.EncodeToString(spendTx.Serialize()))
			println("------------------------------ testing vector", tot, len(v.witness), v.value)
		}
		res := VerifyTxScript(v.pkscr, v.value, 0, spendTx, flags)

		if res != v.expRes {
			t.Error(tot, "TestScritps failed. Got:", res, "   exp:", v.expRes, v.desc)
			return
		} else {
			if DBG_SCR {
				println(tot, "ok:", res, v.desc)
			}
		}

		if tot == 114400 {
			return
		}
	}
}

func decodeFlags(s string) (fl uint32, e error) {
	ss := strings.Split(s, ",")
	for i := range ss {
		switch ss[i] {
		case "": // ignore
		case "NONE": // ignore
			break
		case "P2SH":
			fl |= VER_P2SH
		case "STRICTENC":
			fl |= VER_STRICTENC
		case "DERSIG":
			fl |= VER_DERSIG
		case "LOW_S":
			fl |= VER_LOW_S
		case "NULLDUMMY":
			fl |= VER_NULLDUMMY
		case "SIGPUSHONLY":
			fl |= VER_SIGPUSHONLY
		case "MINIMALDATA":
			fl |= VER_MINDATA
		case "DISCOURAGE_UPGRADABLE_NOPS":
			fl |= VER_BLOCK_OPS
		case "CLEANSTACK":
			fl |= VER_CLEANSTACK
		case "CHECKLOCKTIMEVERIFY":
			fl |= VER_CLTV
		case "CHECKSEQUENCEVERIFY":
			fl |= VER_CSV
		case "WITNESS":
			fl |= VER_WITNESS
		case "DISCOURAGE_UPGRADABLE_WITNESS_PROGRAM":
			fl |= VER_WITNESS_PROG
		case "MINIMALIF":
			fl |= VER_MINIMALIF
		case "NULLFAIL":
			fl |= VER_NULLFAIL
		case "WITNESS_PUBKEYTYPE":
			fl |= VER_WITNESS_PUBKEY
		default:
			e = errors.New("Unsupported flag " + ss[i])
			return
		}
	}
	return
}

func mkCreditTx(pkScr []byte, value uint64) (inputTx *btc.Tx) {
	// We build inputTx only to calculate it's hash for outputTx
	inputTx = new(btc.Tx)
	inputTx.Version = 1
	inputTx.TxIn = []*btc.TxIn{&btc.TxIn{Input: btc.TxPrevOut{Vout: 0xffffffff},
		ScriptSig: []byte{0, 0}, Sequence: 0xffffffff}}
	inputTx.TxOut = []*btc.TxOut{&btc.TxOut{Pk_script: pkScr, Value: value}}
	// Lock_time = 0
	inputTx.SetHash(inputTx.Serialize())
	return
}

func mkSpendTx(inputTx *btc.Tx, sigScr []byte, witness [][]byte) (outputTx *btc.Tx) {
	outputTx = new(btc.Tx)
	outputTx.Version = 1
	outputTx.TxIn = []*btc.TxIn{&btc.TxIn{Input: btc.TxPrevOut{Hash: btc.Sha2Sum(inputTx.Serialize()), Vout: 0},
		ScriptSig: sigScr, Sequence: 0xffffffff}}
	outputTx.TxOut = []*btc.TxOut{&btc.TxOut{Value: inputTx.TxOut[0].Value}}
	// Lock_time = 0

	if len(witness) > 0 {
		outputTx.SegWit = make([][][]byte, 1)
		outputTx.SegWit[0] = witness
		if DBG_SCR {
			println("tx has", len(witness), "ws")
			for xx := range witness {
				println("", xx, hex.EncodeToString(witness[xx]))
			}
		}
	}
	outputTx.SetHash(outputTx.Serialize())
	return
}
