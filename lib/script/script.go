package script

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"runtime/debug"

	"github.com/ParallelCoinTeam/duod/lib/btc"
	"golang.org/x/crypto/ripemd160"
)

// VerifyConsensusFunction -
type VerifyConsensusFunction func(pkScr []byte, amount uint64, i int, tx *btc.Tx, verFlags uint32, result bool)

var (
	// DebugScr -
	DebugScr = false
	// DebugError -
	DebugError = true
	// VerifyConsensus -
	VerifyConsensus VerifyConsensusFunction
)

const (
	// MaxScriptSize -
	MaxScriptSize = 10000
	// VerP2sh -
	VerP2sh = 1 << 0
	// VerStrictEnc -
	VerStrictEnc = 1 << 1
	// VerDerSig -
	VerDerSig = 1 << 2
	// VerLowS -
	VerLowS = 1 << 3
	// VerNullDummy -
	VerNullDummy = 1 << 4
	// VerSigPushOnly -
	VerSigPushOnly = 1 << 5
	// VerMinData -
	VerMinData = 1 << 6
	// VerBlockOps -
	VerBlockOps = 1 << 7 // othewise known as DISCOURAGE_UPGRADABLE_NOPS
	// VerCleanStack -
	VerCleanStack = 1 << 8
	// VerCLTV -
	VerCLTV = 1 << 9
	// VerCSV -
	VerCSV = 1 << 10
	// VerWitness -
	VerWitness = 1 << 11
	// VerWitnessProg -
	VerWitnessProg = 1 << 12 // DISCOURAGE_UPGRADABLE_WITNESS_PROGRAM
	// VerMinimalIf -
	VerMinimalIf = 1 << 13
	// VerNullFail -
	VerNullFail = 1 << 14
	// VerWitnessPubKey -
	VerWitnessPubKey = 1 << 15 // WITNESS_PUBKEYTYPE
	// StandardVerifyFlags -
	StandardVerifyFlags = VerP2sh | VerStrictEnc | VerDerSig | VerLowS |
		VerNullDummy | VerMinData | VerBlockOps | VerCleanStack | VerCLTV | VerCSV |
		VerWitness | VerWitnessProg | VerMinimalIf | VerNullFail | VerWitnessPubKey
	// LockTimeThreshold -
	LockTimeThreshold = 500000000
	// SequenceLocktimeDisableFlag -
	SequenceLocktimeDisableFlag = 1 << 31
	// SequenceLocktimeTypeFlag -
	SequenceLocktimeTypeFlag = 1 << 22
	// SequenceLocktimeMask -
	SequenceLocktimeMask = 0x0000ffff
	// SigVersionBase -
	SigVersionBase = 0
	// SigVersionWitnessV0 -
	SigVersionWitnessV0 = 1
)

// VerifyTxScript -
func VerifyTxScript(pkScr []byte, amount uint64, i int, tx *btc.Tx, verFlags uint32) (result bool) {
	if VerifyConsensus != nil {
		defer func() {
			// We call CompareToConsensus inside another function to wait for final "result"
			VerifyConsensus(pkScr, amount, i, tx, verFlags, result)
		}()
	}

	sigScr := tx.TxIn[i].ScriptSig

	if (verFlags&VerSigPushOnly) != 0 && !btc.IsPushOnly(sigScr) {
		if DebugError {
			fmt.Println("Not push only")
		}
		return false
	}

	if DebugScr {
		fmt.Println("VerifyTxScript", tx.Hash.String(), i+1, "/", len(tx.TxIn))
		fmt.Println("sigScript:", hex.EncodeToString(sigScr[:]))
		fmt.Println("_pkScript:", hex.EncodeToString(pkScr))
		fmt.Printf("flagz:%x\n", verFlags)
	}

	var stack, stackCopy scrStack
	if !evalScript(sigScr, amount, &stack, tx, i, verFlags, SigVersionBase) {
		if DebugError {
			if tx != nil {
				fmt.Println("VerifyTxScript", tx.Hash.String(), i+1, "/", len(tx.TxIn))
			}
			fmt.Println("sigScript failed :", hex.EncodeToString(sigScr[:]))
			fmt.Println("pkScript:", hex.EncodeToString(pkScr[:]))
		}
		return
	}
	if DebugScr {
		fmt.Println("\nsigScr verified OK")
		//stack.print()
		fmt.Println()
	}

	if (verFlags&VerP2sh) != 0 && stack.size() > 0 {
		// copy the stack content to stackCopy
		stackCopy.copyFrom(&stack)
	}

	if !evalScript(pkScr, amount, &stack, tx, i, verFlags, SigVersionBase) {
		if DebugScr {
			fmt.Println("* pkScript failed :", hex.EncodeToString(pkScr[:]))
			fmt.Println("* VerifyTxScript", tx.Hash.String(), i+1, "/", len(tx.TxIn))
			fmt.Println("* sigScript:", hex.EncodeToString(sigScr[:]))
		}
		return
	}

	if stack.size() == 0 {
		if DebugScr {
			fmt.Println("* stack empty after executing scripts:", hex.EncodeToString(pkScr[:]))
		}
		return
	}

	if !stack.topBool(-1) {
		if DebugScr {
			fmt.Println("* FALSE on stack after executing scripts:", hex.EncodeToString(pkScr[:]))
		}
		return
	}

	// Bare witness programs
	var witnessversion int
	var witnessprogram []byte
	var hadWitness bool
	var witness witnessCtx

	if (verFlags & VerWitness) != 0 {
		if tx.SegWit != nil {
			for _, wd := range tx.SegWit[i] {
				witness.stack.push(wd)
			}
		}

		witnessversion, witnessprogram = btc.IsWitnessProgram(pkScr)
		if DebugScr {
			fmt.Println("------------witnessversion:", witnessversion, "   witnessprogram:", hex.EncodeToString(witnessprogram))
		}
		if witnessprogram != nil {
			hadWitness = true
			if len(sigScr) != 0 {
				if DebugError {
					fmt.Println("SCRIPT_ERR_WITNESS_MALLEATED")
				}
				return
			}
			if !VerifyWitnessProgram(&witness, amount, tx, i, witnessversion, witnessprogram, verFlags) {
				if DebugError {
					fmt.Println("VerifyWitnessProgram failed A")
				}
				return false
			}
			// Bypass the cleanstack check at the end. The actual stack is obviously not clean
			// for witness programs.
			stack.resize(1)
		} else {
			if DebugScr {
				fmt.Println("No witness program")
			}
		}
	} else {
		if DebugScr {
			fmt.Println("Witness flag off")
		}
	}

	// Additional validation for spend-to-script-hash transactions:
	if (verFlags&VerP2sh) != 0 && btc.IsPayToScript(pkScr) {
		if DebugScr {
			fmt.Println()
			fmt.Println()
			fmt.Println(" ******************* Looks like P2SH script ********************* ")
			stack.print()
		}

		if DebugScr {
			fmt.Println("sigScr len", len(sigScr), hex.EncodeToString(sigScr))
		}
		if !btc.IsPushOnly(sigScr) {
			if DebugError {
				fmt.Println("P2SH is not push only")
			}
			return
		}

		// Restore stack.
		stack = stackCopy

		pubKey2 := stack.pop()
		if DebugScr {
			fmt.Println("pubKey2:", hex.EncodeToString(pubKey2))
		}

		if !evalScript(pubKey2, amount, &stack, tx, i, verFlags, SigVersionBase) {
			if DebugError {
				fmt.Println("P2SH extra verification failed")
			}
			return
		}

		if stack.size() == 0 {
			if DebugScr {
				fmt.Println("* P2SH stack empty after executing script:", hex.EncodeToString(pubKey2))
			}
			return
		}

		if !stack.topBool(-1) {
			if DebugScr {
				fmt.Println("* FALSE on stack after executing P2SH script:", hex.EncodeToString(pubKey2))
			}
			return
		}

		if (verFlags & VerWitness) != 0 {
			witnessversion, witnessprogram = btc.IsWitnessProgram(pubKey2)
			if DebugScr {
				fmt.Println("============witnessversion:", witnessversion, "   witnessprogram:", hex.EncodeToString(witnessprogram))
			}
			if witnessprogram != nil {
				hadWitness = true
				bt := new(bytes.Buffer)
				btc.WritePutLen(bt, uint32(len(pubKey2)))
				bt.Write(pubKey2)
				if !bytes.Equal(sigScr, bt.Bytes()) {
					if DebugError {
						fmt.Println(hex.EncodeToString(sigScr))
						fmt.Println(hex.EncodeToString(bt.Bytes()))
						fmt.Println("SCRIPT_ERR_WITNESS_MALLEATED_P2SH")
					}
					return
				}
				if !VerifyWitnessProgram(&witness, amount, tx, i, witnessversion, witnessprogram, verFlags) {
					if DebugError {
						fmt.Println("VerifyWitnessProgram failed B")
					}
					return false
				}
				// Bypass the cleanstack check at the end. The actual stack is obviously not clean
				// for witness programs.
				stack.resize(1)
			}
		}
	}

	if (verFlags & VerCleanStack) != 0 {
		if (verFlags & VerP2sh) == 0 {
			panic("VerCleanStack without VerP2sh")
		}
		if DebugScr {
			fmt.Println("stack size", stack.size())
		}
		if stack.size() != 1 {
			if DebugError {
				fmt.Println("Stack not clean")
			}
			return
		}
	}

	if (verFlags & VerWitness) != 0 {
		// We can't check for correct unexpected witness data if P2SH was off, so require
		// that WITNESS implies P2SH. Otherwise, going from WITNESS->P2SH+WITNESS would be
		// possible, which is not a softfork.
		if (verFlags & VerP2sh) == 0 {
			panic("VerWitness must be used with P2SH")
		}
		if !hadWitness && !witness.IsNull() {
			if DebugError {
				fmt.Println("SCRIPT_ERR_WITNESS_UNEXPECTED", len(tx.SegWit))
			}
			return
		}
	}

	result = true
	return true
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func evalScript(p []byte, amount uint64, stack *scrStack, tx *btc.Tx, inp int, verFlags uint32, sigversion int) bool {
	if DebugScr {
		fmt.Println("evalScript len", len(p), "amount", amount, "inp", inp, "flagz", verFlags, "sigver", sigversion)
		stack.print()
	}

	if len(p) > MaxScriptSize {
		if DebugError {
			fmt.Println("script too long", len(p))
		}
		return false
	}

	defer func() {
		if r := recover(); r != nil {
			if DebugError {
				err, ok := r.(error)
				if !ok {
					err = fmt.Errorf("pkg: %v", r)
				}
				fmt.Println("evalScript panic:", err.Error())
				fmt.Println(string(debug.Stack()))
			}
		}
	}()

	var exestack scrStack
	var altstack scrStack
	sta, idx, opcnt := 0, 0, 0
	checkMinVals := (verFlags & VerMinData) != 0
	for idx < len(p) {
		inexec := exestack.nofalse()

		// Read instruction
		opcode, pushval, n, e := btc.GetOpcode(p[idx:])
		if e != nil {
			//fmt.Println(e.Error())
			//fmt.Println("A", idx, hex.EncodeToString(p))
			return false
		}
		idx += n

		if DebugScr {
			fmt.Printf("\nExecuting opcode 0x%02x  n=%d  inexec:%t  push:%s..\n",
				opcode, n, inexec, hex.EncodeToString(pushval))
			stack.print()
		}

		if pushval != nil && len(pushval) > btc.MaxScriptElementSize {
			if DebugError {
				fmt.Println("pushval too long", len(pushval))
			}
			return false
		}

		if opcode > 0x60 {
			opcnt++
			if opcnt > 201 {
				if DebugError {
					fmt.Println("evalScript: too many opcodes A")
				}
				return false
			}
		}

		if opcode == 0x7e /*OP_CAT*/ ||
			opcode == 0x7f /*OP_SUBSTR*/ ||
			opcode == 0x80 /*OP_LEFT*/ ||
			opcode == 0x81 /*OP_RIGHT*/ ||
			opcode == 0x83 /*OP_INVERT*/ ||
			opcode == 0x84 /*OP_AND*/ ||
			opcode == 0x85 /*OP_OR*/ ||
			opcode == 0x86 /*OP_XOR*/ ||
			opcode == 0x8d /*OP_2MUL*/ ||
			opcode == 0x8e /*OP_2DIV*/ ||
			opcode == 0x95 /*OP_MUL*/ ||
			opcode == 0x96 /*OP_DIV*/ ||
			opcode == 0x97 /*OP_MOD*/ ||
			opcode == 0x98 /*OP_LSHIFT*/ ||
			opcode == 0x99 /*OP_RSHIFT*/ {
			if DebugError {
				fmt.Println("Unsupported opcode", opcode)
			}
			return false
		}

		if inexec && 0 <= opcode && opcode <= btc.OP_PUSHDATA4 {
			if checkMinVals && !checkMinimalPush(pushval, opcode) {
				if DebugError {
					fmt.Println("Push value not in a minimal format", hex.EncodeToString(pushval))
				}
				return false
			}
			stack.push(pushval)
			if DebugScr {
				fmt.Println("pushed", len(pushval), "bytes")
			}
		} else if inexec || (0x63 /*OP_IF*/ <= opcode && opcode <= 0x68 /*OP_ENDIF*/) {
			switch {
			case opcode == 0x4f: // OP_1NEGATE
				stack.pushInt(-1)

			case opcode >= 0x51 && opcode <= 0x60: // OP_1-OP_16
				stack.pushInt(int64(opcode - 0x50))

			case opcode == 0x61: // OP_NOP
				// Do nothing

			/* - not handled
			OP_VER = 0x62
			*/

			case opcode == 0x63 || opcode == 0x64: //OP_IF || OP_NOTIF
				// <expression> if [statements] [else [statements]] endif
				val := false
				if inexec {
					if stack.size() < 1 {
						if DebugError {
							fmt.Println("Stack too short for", opcode)
						}
						return false
					}
					vch := stack.pop()
					if sigversion == SigVersionWitnessV0 && (verFlags&VerMinimalIf) != 0 {
						if len(vch) > 1 {
							if DebugError {
								fmt.Println("SCRIPT_ERR_MINIMALIF-1")
							}
							return false
						}
						if len(vch) == 1 && vch[0] != 1 {
							if DebugError {
								fmt.Println("SCRIPT_ERR_MINIMALIF-2")
							}
							return false
						}
					}
					val = bts2bool(vch)
					if opcode == 0x64 /*OP_NOTIF*/ {
						val = !val
					}
				}
				if DebugScr {
					fmt.Println(inexec, "if pushing", val, "...")
				}
				exestack.pushBool(val)

			/* - not handled
			   OP_VERIF = 0x65,
			   OP_VERNOTIF = 0x66,
			*/
			case opcode == 0x67: //OP_ELSE
				if exestack.size() == 0 {
					if DebugError {
						fmt.Println("exestack empty in OP_ELSE")
					}
				}
				exestack.pushBool(!exestack.popBool())

			case opcode == 0x68: //OP_ENDIF
				if exestack.size() == 0 {
					if DebugError {
						fmt.Println("exestack empty in OP_ENDIF")
					}
				}
				exestack.pop()

			case opcode == 0x69: //OP_VERIFY
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				if !stack.topBool(-1) {
					return false
				}
				stack.pop()

			case opcode == 0x6b: //OP_TOALTSTACK
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				altstack.push(stack.pop())

			case opcode == 0x6c: //OP_FROMALTSTACK
				if altstack.size() < 1 {
					if DebugError {
						fmt.Println("AltStack too short for opcode", opcode)
					}
					return false
				}
				stack.push(altstack.pop())

			case opcode == 0x6d: //OP_2DROP
				if stack.size() < 2 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				stack.pop()
				stack.pop()

			case opcode == 0x6e: //OP_2DUP
				if stack.size() < 2 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				x1 := stack.top(-1)
				x2 := stack.top(-2)
				stack.push(x2)
				stack.push(x1)

			case opcode == 0x6f: //OP_3DUP
				if stack.size() < 3 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				x1 := stack.top(-3)
				x2 := stack.top(-2)
				x3 := stack.top(-1)
				stack.push(x1)
				stack.push(x2)
				stack.push(x3)

			case opcode == 0x70: //OP_2OVER
				if stack.size() < 4 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				x1 := stack.top(-4)
				x2 := stack.top(-3)
				stack.push(x1)
				stack.push(x2)

			case opcode == 0x71: //OP_2ROT
				// (x1 x2 x3 x4 x5 x6 -- x3 x4 x5 x6 x1 x2)
				if stack.size() < 6 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				x6 := stack.pop()
				x5 := stack.pop()
				x4 := stack.pop()
				x3 := stack.pop()
				x2 := stack.pop()
				x1 := stack.pop()
				stack.push(x3)
				stack.push(x4)
				stack.push(x5)
				stack.push(x6)
				stack.push(x1)
				stack.push(x2)

			case opcode == 0x72: //OP_2SWAP
				// (x1 x2 x3 x4 -- x3 x4 x1 x2)
				if stack.size() < 4 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				x4 := stack.pop()
				x3 := stack.pop()
				x2 := stack.pop()
				x1 := stack.pop()
				stack.push(x3)
				stack.push(x4)
				stack.push(x1)
				stack.push(x2)

			case opcode == 0x73: //OP_IFDUP
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				if stack.topBool(-1) {
					stack.push(stack.top(-1))
				}

			case opcode == 0x74: //OP_DEPTH
				stack.pushInt(int64(stack.size()))

			case opcode == 0x75: //OP_DROP
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				stack.pop()

			case opcode == 0x76: //OP_DUP
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				el := stack.pop()
				stack.push(el)
				stack.push(el)

			case opcode == 0x77: //OP_NIP
				if stack.size() < 2 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				x := stack.pop()
				stack.pop()
				stack.push(x)

			case opcode == 0x78: //OP_OVER
				if stack.size() < 2 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				stack.push(stack.top(-2))

			case opcode == 0x79 || opcode == 0x7a: //OP_PICK || OP_ROLL
				if stack.size() < 2 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				n := stack.popInt(checkMinVals)
				if n < 0 || n >= int64(stack.size()) {
					if DebugError {
						fmt.Println("Wrong n for opcode", opcode)
					}
					return false
				}
				if opcode == 0x79 /*OP_PICK*/ {
					stack.push(stack.top(int(-1 - n)))
				} else if n > 0 {
					tmp := make([][]byte, n)
					for i := range tmp {
						tmp[i] = stack.pop()
					}
					xn := stack.pop()
					for i := len(tmp) - 1; i >= 0; i-- {
						stack.push(tmp[i])
					}
					stack.push(xn)
				}

			case opcode == 0x7b: //OP_ROT
				if stack.size() < 3 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				x3 := stack.pop()
				x2 := stack.pop()
				x1 := stack.pop()
				stack.push(x2)
				stack.push(x3)
				stack.push(x1)

			case opcode == 0x7c: //OP_SWAP
				if stack.size() < 2 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				x1 := stack.pop()
				x2 := stack.pop()
				stack.push(x1)
				stack.push(x2)

			case opcode == 0x7d: //OP_TUCK
				if stack.size() < 2 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				x1 := stack.pop()
				x2 := stack.pop()
				stack.push(x1)
				stack.push(x2)
				stack.push(x1)

			case opcode == 0x82: //OP_SIZE
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				stack.pushInt(int64(len(stack.top(-1))))

			case opcode == 0x87 || opcode == 0x88: //OP_EQUAL || OP_EQUALVERIFY
				if stack.size() < 2 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				a := stack.pop()
				b := stack.pop()
				if opcode == 0x88 { //OP_EQUALVERIFY
					if !bytes.Equal(a, b) {
						return false
					}
				} else {
					stack.pushBool(bytes.Equal(a, b))
				}

			/* - not handled
			OP_RESERVED1 = 0x89,
			OP_RESERVED2 = 0x8a,
			*/

			case opcode == 0x8b: //OP_1ADD
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				stack.pushInt(stack.popInt(checkMinVals) + 1)

			case opcode == 0x8c: //OP_1SUB
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				stack.pushInt(stack.popInt(checkMinVals) - 1)

			case opcode == 0x8f: //OP_NEGATE
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				stack.pushInt(-stack.popInt(checkMinVals))

			case opcode == 0x90: //OP_ABS
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				a := stack.popInt(checkMinVals)
				if a < 0 {
					stack.pushInt(-a)
				} else {
					stack.pushInt(a)
				}

			case opcode == 0x91: //OP_NOT
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				stack.pushBool(stack.popInt(checkMinVals) == 0)

			case opcode == 0x92: //OP_0NOTEQUAL
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				d := stack.pop()
				if checkMinVals && len(d) > 1 {
					if DebugError {
						fmt.Println("Not minimal bool value", hex.EncodeToString(d))
					}
					return false
				}
				stack.pushBool(bts2bool(d))

			case opcode == 0x93 || //OP_ADD
				opcode == 0x94 || //OP_SUB
				opcode == 0x9a || //OP_BOOLAND
				opcode == 0x9b || //OP_BOOLOR
				opcode == 0x9c || opcode == 0x9d || //OP_NUMEQUAL || OP_NUMEQUALVERIFY
				opcode == 0x9e || //OP_NUMNOTEQUAL
				opcode == 0x9f || //OP_LESSTHAN
				opcode == 0xa0 || //OP_GREATERTHAN
				opcode == 0xa1 || //OP_LESSTHANOREQUAL
				opcode == 0xa2 || //OP_GREATERTHANOREQUAL
				opcode == 0xa3 || //OP_MIN
				opcode == 0xa4: //OP_MAX
				if stack.size() < 2 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				bn2 := stack.popInt(checkMinVals)
				bn1 := stack.popInt(checkMinVals)
				var bn int64
				switch opcode {
				case 0x93:
					bn = bn1 + bn2 // OP_ADD
				case 0x94:
					bn = bn1 - bn2 // OP_SUB
				case 0x9a:
					bn = b2i(bn1 != 0 && bn2 != 0) // OP_BOOLAND
				case 0x9b:
					bn = b2i(bn1 != 0 || bn2 != 0) // OP_BOOLOR
				case 0x9c:
					bn = b2i(bn1 == bn2) // OP_NUMEQUAL
				case 0x9d:
					bn = b2i(bn1 == bn2) // OP_NUMEQUALVERIFY
				case 0x9e:
					bn = b2i(bn1 != bn2) // OP_NUMNOTEQUAL
				case 0x9f:
					bn = b2i(bn1 < bn2) // OP_LESSTHAN
				case 0xa0:
					bn = b2i(bn1 > bn2) // OP_GREATERTHAN
				case 0xa1:
					bn = b2i(bn1 <= bn2) // OP_LESSTHANOREQUAL
				case 0xa2:
					bn = b2i(bn1 >= bn2) // OP_GREATERTHANOREQUAL
				case 0xa3: // OP_MIN
					if bn1 < bn2 {
						bn = bn1
					} else {
						bn = bn2
					}
				case 0xa4: // OP_MAX
					if bn1 > bn2 {
						bn = bn1
					} else {
						bn = bn2
					}
				default:
					panic("invalid opcode")
				}
				if opcode == 0x9d { //OP_NUMEQUALVERIFY
					if bn == 0 {
						return false
					}
				} else {
					stack.pushInt(bn)
				}

			case opcode == 0xa5: //OP_WITHIN
				if stack.size() < 3 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				bn3 := stack.popInt(checkMinVals)
				bn2 := stack.popInt(checkMinVals)
				bn1 := stack.popInt(checkMinVals)
				stack.pushBool(bn2 <= bn1 && bn1 < bn3)

			case opcode == 0xa6: //OP_RIPEMD160
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				rim := ripemd160.New()
				rim.Write(stack.pop()[:])
				stack.push(rim.Sum(nil)[:])

			case opcode == 0xa7: //OP_SHA1
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				sha := sha1.New()
				sha.Write(stack.pop()[:])
				stack.push(sha.Sum(nil)[:])

			case opcode == 0xa8: //OP_SHA256
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				sha := sha256.New()
				sha.Write(stack.pop()[:])
				stack.push(sha.Sum(nil)[:])

			case opcode == 0xa9: //OP_HASH160
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				rim160 := btc.Rimp160AfterSha256(stack.pop())
				stack.push(rim160[:])

			case opcode == 0xaa: //OP_HASH256
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				h := btc.Sha2Sum(stack.pop())
				stack.push(h[:])

			case opcode == 0xab: // OP_CODESEPARATOR
				sta = idx

			case opcode == 0xac || opcode == 0xad: // OP_CHECKSIG || OP_CHECKSIGVERIFY

				if stack.size() < 2 {
					if DebugError {
						fmt.Println("Stack too short for opcode", opcode)
					}
					return false
				}
				var fSuccess bool
				vchSig := stack.top(-2)
				vchPubKey := stack.top(-1)

				// BIP-0066
				if !CheckSignatureEncoding(vchSig, verFlags) || !CheckPubKeyEncoding(vchPubKey, verFlags, sigversion) {
					if DebugError {
						fmt.Println("Invalid Signature Encoding A")
					}
					return false
				}

				if len(vchSig) > 0 {
					var sh []byte
					if sigversion == SigVersionWitnessV0 {
						if DebugScr {
							fmt.Println("getting WitnessSigHash for inp", inp, "and htype", int32(vchSig[len(vchSig)-1]))
						}
						sh = tx.WitnessSigHash(p[sta:], amount, inp, int32(vchSig[len(vchSig)-1]))
					} else {
						sh = tx.SignatureHash(delSig(p[sta:], vchSig), inp, int32(vchSig[len(vchSig)-1]))
					}
					if DebugScr {
						fmt.Println("EcdsaVerify", hex.EncodeToString(sh))
						fmt.Println(" key:", hex.EncodeToString(vchPubKey))
						fmt.Println(" sig:", hex.EncodeToString(vchSig))
					}
					fSuccess = btc.EcdsaVerify(vchPubKey, vchSig, sh)
					if DebugScr {
						fmt.Println(" ->", fSuccess)
					}
				}
				if !fSuccess && DebugScr {
					fmt.Println("EcdsaVerify fail 1", tx.Hash.String())
				}

				if !fSuccess && (verFlags&VerNullFail) != 0 && len(vchSig) > 0 {
					if DebugError {
						fmt.Println("SCRIPT_ERR_SIG_NULLFAIL-1")
					}
					return false
				}

				stack.pop()
				stack.pop()

				if DebugScr {
					fmt.Println("ver:", fSuccess)
				}
				if opcode == 0xad {
					if !fSuccess { // OP_CHECKSIGVERIFY
						return false
					}
				} else { // OP_CHECKSIG
					stack.pushBool(fSuccess)
				}

			case opcode == 0xae || opcode == 0xaf: //OP_CHECKMULTISIG || OP_CHECKMULTISIGVERIFY
				//fmt.Println("OP_CHECKMULTISIG ...")
				//stack.print()
				if stack.size() < 1 {
					if DebugError {
						fmt.Println("OP_CHECKMULTISIG: Stack too short A")
					}
					return false
				}
				i := 1
				keyscnt := stack.topInt(-i, checkMinVals)
				if keyscnt < 0 || keyscnt > 20 {
					if DebugError {
						fmt.Println("OP_CHECKMULTISIG: Wrong number of keys")
					}
					return false
				}
				opcnt += int(keyscnt)
				if opcnt > 201 {
					if DebugError {
						fmt.Println("evalScript: too many opcodes B")
					}
					return false
				}
				i++
				ikey := i
				// ikey2 is the position of last non-signature item in the stack. Top stack item = 1.
				// With SCRIPT_VERIFY_NULLFAIL, this is used for cleanup if operation fails.
				ikey2 := keyscnt + 2
				i += int(keyscnt)
				if stack.size() < i {
					if DebugError {
						fmt.Println("OP_CHECKMULTISIG: Stack too short B")
					}
					return false
				}
				sigscnt := stack.topInt(-i, checkMinVals)
				if sigscnt < 0 || sigscnt > keyscnt {
					if DebugError {
						fmt.Println("OP_CHECKMULTISIG: sigscnt error")
					}
					return false
				}
				i++
				isig := i
				i += int(sigscnt)
				if stack.size() < i {
					if DebugError {
						fmt.Println("OP_CHECKMULTISIG: Stack too short C")
					}
					return false
				}

				xxx := p[sta:]
				if sigversion != SigVersionWitnessV0 {
					for k := 0; k < int(sigscnt); k++ {
						xxx = delSig(xxx, stack.top(-isig-k))
					}
				}

				success := true
				for sigscnt > 0 {
					vchPubKey := stack.top(-ikey)
					vchSig := stack.top(-isig)

					// BIP-0066
					if !CheckSignatureEncoding(vchSig, verFlags) || !CheckPubKeyEncoding(vchPubKey, verFlags, sigversion) {
						if DebugError {
							fmt.Println("Invalid Signature Encoding B")
						}
						return false
					}

					if len(vchSig) > 0 {
						var sh []byte

						if sigversion == SigVersionWitnessV0 {
							sh = tx.WitnessSigHash(xxx, amount, inp, int32(vchSig[len(vchSig)-1]))
						} else {
							sh = tx.SignatureHash(xxx, inp, int32(vchSig[len(vchSig)-1]))
						}
						if btc.EcdsaVerify(vchPubKey, vchSig, sh) {
							isig++
							sigscnt--
						}
					}

					ikey++
					keyscnt--

					// If there are more signatures left than keys left,
					// then too many signatures have failed
					if sigscnt > keyscnt {
						success = false
						break
					}
				}

				// Clean up stack of actual arguments
				for i > 1 {
					i--

					if !success && (verFlags&VerNullFail) != 0 && ikey2 == 0 && len(stack.top(-1)) > 0 {
						if DebugError {
							fmt.Println("SCRIPT_ERR_SIG_NULLFAIL-2")
						}
						return false
					}
					if ikey2 > 0 {
						ikey2--
					}

					stack.pop()
				}

				if stack.size() < 1 {
					if DebugError {
						fmt.Println("OP_CHECKMULTISIG: Dummy element missing")
					}
					return false
				}
				if (verFlags&VerNullDummy) != 0 && len(stack.top(-1)) != 0 {
					if DebugError {
						fmt.Println("OP_CHECKMULTISIG: NULLDUMMY verification failed")
					}
					return false
				}
				stack.pop()

				if opcode == 0xaf {
					if !success { // OP_CHECKMULTISIGVERIFY
						return false
					}
				} else {
					stack.pushBool(success)
				}

			case opcode == 0xb1: //OP_NOP2 or OP_CHECKLOCKTIMEVERIFY
				if (verFlags & VerCLTV) == 0 {
					if (verFlags & VerBlockOps) != 0 {
						return false
					}
					break // Just do NOP2
				}

				if DebugScr {
					fmt.Println("OP_CHECKLOCKTIMEVERIFY...")
				}

				if stack.size() < 1 {
					if DebugError {
						fmt.Println("OP_CHECKLOCKTIMEVERIFY: Stack too short")
					}
					return false
				}

				d := stack.top(-1)
				if len(d) > 5 {
					if DebugError {
						fmt.Println("OP_CHECKLOCKTIMEVERIFY: locktime field too long", len(d))
					}
					return false
				}

				if DebugScr {
					fmt.Println("val from stack", hex.EncodeToString(d))
				}

				locktime := bts2intExt(d, 5, checkMinVals)
				if locktime < 0 {
					if DebugError {
						fmt.Println("OP_CHECKLOCKTIMEVERIFY: negative locktime")
					}
					return false
				}

				if !((tx.LockTime < LockTimeThreshold && locktime < LockTimeThreshold) ||
					(tx.LockTime >= LockTimeThreshold && locktime >= LockTimeThreshold)) {
					if DebugError {
						fmt.Println("OP_CHECKLOCKTIMEVERIFY: broken lock value")
					}
					return false
				}

				if DebugScr {
					fmt.Println("locktime > int64(tx.LockTime)", locktime, int64(tx.LockTime))
					fmt.Println(" ... seq", len(tx.TxIn), inp, tx.TxIn[inp].Sequence)
				}

				// Actually compare the specified lock time with the transaction.
				if locktime > int64(tx.LockTime) {
					if DebugError {
						fmt.Println("OP_CHECKLOCKTIMEVERIFY: Locktime requirement not satisfied")
					}
					return false
				}

				if tx.TxIn[inp].Sequence == 0xffffffff {
					if DebugError {
						fmt.Println("OP_CHECKLOCKTIMEVERIFY: TxIn final")
					}
					return false
				}

				// OP_CHECKLOCKTIMEVERIFY passed successfully

			case opcode == 0xb2: //OP_NOP3 or OP_CHECKSEQUENCEVERIFY
				if (verFlags & VerCSV) == 0 {
					if (verFlags & VerBlockOps) != 0 {
						return false
					}
					break // Just do NOP3
				}

				if DebugScr {
					fmt.Println("OP_CHECKSEQUENCEVERIFY...")
				}

				if stack.size() < 1 {
					if DebugError {
						fmt.Println("OP_CHECKSEQUENCEVERIFY: Stack too short")
					}
					return false
				}

				d := stack.top(-1)
				if len(d) > 5 {
					if DebugError {
						fmt.Println("OP_CHECKSEQUENCEVERIFY: sequence field too long", len(d))
					}
					return false
				}

				if DebugScr {
					fmt.Println("seq from stack", hex.EncodeToString(d))
				}

				sequence := bts2intExt(d, 5, checkMinVals)
				if sequence < 0 {
					if DebugError {
						fmt.Println("OP_CHECKSEQUENCEVERIFY: negative sequence")
					}
					return false
				}

				if (sequence & SequenceLocktimeDisableFlag) != 0 {
					break
				}

				if !CheckSequence(tx, inp, sequence) {
					if DebugError {
						fmt.Println("OP_CHECKSEQUENCEVERIFY: CheckSequence failed")
					}
					return false
				}

			case opcode == 0xb0 || opcode >= 0xb3 && opcode <= 0xb9: //OP_NOP1 || OP_NOP4..OP_NOP10
				if (verFlags & VerBlockOps) != 0 {
					return false
				}
				// just do nothing

			default:
				if DebugError {
					fmt.Printf("Unhandled opcode 0x%02x - a handler must be implemented\n", opcode)
					stack.print()
					fmt.Println("Rest of the script:", hex.EncodeToString(p[idx:]))
				}
				return false
			}
		}

		if DebugScr {
			fmt.Printf("Finished Executing opcode 0x%02x\n", opcode)
			stack.print()
		}
		if stack.size()+altstack.size() > 1000 {
			if DebugError {
				fmt.Println("Stack too big")
			}
			return false
		}
	}

	if DebugScr {
		fmt.Println("END OF SCRIPT")
		stack.print()
	}

	if exestack.size() > 0 {
		if DebugError {
			fmt.Println("Unfinished if..")
		}
		return false
	}

	return true
}

func delSig(where, sig []byte) (res []byte) {
	// recover the standard length
	bb := new(bytes.Buffer)
	if len(sig) < btc.OP_PUSHDATA1 {
		bb.Write([]byte{byte(len(sig))})
	} else if len(sig) <= 0xff {
		bb.Write([]byte{btc.OP_PUSHDATA1})
		bb.Write([]byte{byte(len(sig))})
	} else if len(sig) <= 0xffff {
		bb.Write([]byte{btc.OP_PUSHDATA2})
		binary.Write(bb, binary.LittleEndian, uint16(len(sig)))
	} else {
		bb.Write([]byte{btc.OP_PUSHDATA4})
		binary.Write(bb, binary.LittleEndian, uint32(len(sig)))
	}
	bb.Write(sig)
	sig = bb.Bytes()
	var idx int
	for idx < len(where) {
		_, _, n, e := btc.GetOpcode(where[idx:])
		if e != nil {
			fmt.Println(e.Error())
			fmt.Println("B", idx, hex.EncodeToString(where))
			return
		}
		if !bytes.Equal(where[idx:idx+n], sig) {
			res = append(res, where[idx:idx+n]...)
		}
		idx += n
	}
	return
}

// IsValidSignatureEncoding -
func IsValidSignatureEncoding(sig []byte) bool {
	// Minimum and maximum size constraints.
	if len(sig) < 9 {
		return false
	}
	if len(sig) > 73 {
		return false
	}

	// A signature is of type 0x30 (compound).
	if sig[0] != 0x30 {
		return false
	}

	// Make sure the length covers the entire signature.
	if int(sig[1]) != len(sig)-3 {
		return false
	}

	// Extract the length of the R element.
	lenR := uint(sig[3])

	// Make sure the length of the S element is still inside the signature.
	if 5+lenR >= uint(len(sig)) {
		return false
	}

	// Extract the length of the S element.
	lenS := uint(sig[5+lenR])

	// Verify that the length of the signature matches the sum of the length
	// of the elements.
	if lenR+lenS+7 != uint(len(sig)) {
		return false
	}

	// Check whether the R element is an integer.
	if sig[2] != 0x02 {
		return false
	}

	// Zero-length integers are not allowed for R.
	if lenR == 0 {
		return false
	}

	// Negative numbers are not allowed for R.
	if (sig[4] & 0x80) != 0 {
		return false
	}

	// Null bytes at the start of R are not allowed, unless R would
	// otherwise be interpreted as a negative number.
	if lenR > 1 && sig[4] == 0x00 && (sig[5]&0x80) == 0 {
		return false
	}

	// Check whether the S element is an integer.
	if sig[lenR+4] != 0x02 {
		return false
	}

	// Zero-length integers are not allowed for S.
	if lenS == 0 {
		return false
	}

	// Negative numbers are not allowed for S.
	if (sig[lenR+6] & 0x80) != 0 {
		return false
	}

	// Null bytes at the start of S are not allowed, unless S would otherwise be
	// interpreted as a negative number.
	if lenS > 1 && (sig[lenR+6] == 0x00) && (sig[lenR+7]&0x80) == 0 {
		return false
	}

	return true
}

// IsDefinedHashtypeSignature -
func IsDefinedHashtypeSignature(sig []byte) bool {
	if len(sig) == 0 {
		return false
	}
	htype := sig[len(sig)-1] & (btc.SigHashAnyoneCanPay ^ 0xff)
	if htype < btc.SigHashAll || htype > btc.SigHashSingle {
		return false
	}
	return true
}

// IsLowS -
func IsLowS(sig []byte) bool {
	if !IsValidSignatureEncoding(sig) {
		return false
	}

	ss, e := btc.NewSignature(sig)
	if e != nil {
		return false
	}

	return ss.IsLowS()
}

// CheckSignatureEncoding -
func CheckSignatureEncoding(sig []byte, flags uint32) bool {
	if len(sig) == 0 {
		return true
	}
	if (flags&(VerDerSig|VerStrictEnc)) != 0 && !IsValidSignatureEncoding(sig) {
		return false
	} else if (flags&VerLowS) != 0 && !IsLowS(sig) {
		return false
	} else if (flags&VerStrictEnc) != 0 && !IsDefinedHashtypeSignature(sig) {
		return false
	}
	return true
}

// IsCompressedOrUncompressedPubKey -
func IsCompressedOrUncompressedPubKey(pk []byte) bool {
	if len(pk) < 33 {
		return false
	}
	if pk[0] == 0x04 {
		if len(pk) != 65 {
			return false
		}
	} else if pk[0] == 0x02 || pk[0] == 0x03 {
		if len(pk) != 33 {
			return false
		}
	} else {
		return false
	}
	return true
}

// IsCompressedPubKey -
func IsCompressedPubKey(pk []byte) bool {
	if len(pk) != 33 {
		return false
	}
	if pk[0] == 0x02 || pk[0] == 0x03 {
		return true
	}
	return false
}

// CheckPubKeyEncoding -
func CheckPubKeyEncoding(pk []byte, flags uint32, sigversion int) bool {
	if (flags&VerStrictEnc) != 0 && !IsCompressedOrUncompressedPubKey(pk) {
		return false
	}
	// Only compressed keys are accepted in segwit
	if (flags&VerWitnessPubKey) != 0 && sigversion == SigVersionWitnessV0 && !IsCompressedPubKey(pk) {
		return false
	}
	return true
}

// https://bitcointalk.org/index.php?topic=1240385.0
func checkMinimalPush(d []byte, opcode int) bool {
	if DebugScr {
		fmt.Printf("checkMinimalPush %02x %s\n", opcode, hex.EncodeToString(d))
	}
	if len(d) == 0 {
		// Could have used OP_0.
		if DebugScr {
			fmt.Println("Could have used OP_0.")
		}
		return opcode == 0x00
	} else if len(d) == 1 && d[0] >= 1 && d[0] <= 16 {
		// Could have used OP_1 .. OP_16.
		if DebugScr {
			fmt.Println("Could have used OP_1 .. OP_16.", 0x01+int(d[0]-1), 0x01, int(d[0]-1))
		}
		return opcode == 0x51+int(d[0])-1
	} else if len(d) == 1 && d[0] == 0x81 {
		// Could have used OP_1NEGATE.
		if DebugScr {
			fmt.Println("Could have used OP_1NEGATE.")
		}
		return opcode == 0x4f
	} else if len(d) <= 75 {
		// Could have used a direct push (opcode indicating number of bytes pushed + those bytes).
		if DebugScr {
			fmt.Println("Could have used a direct push (opcode indicating number of bytes pushed + those bytes).")
		}
		return opcode == len(d)
	} else if len(d) <= 255 {
		// Could have used OP_PUSHDATA.
		if DebugScr {
			fmt.Println("Could have used OP_PUSHDATA.")
		}
		return opcode == 0x4c
	} else if len(d) <= 65535 {
		// Could have used OP_PUSHDATA2.
		if DebugScr {
			fmt.Println("Could have used OP_PUSHDATA2.")
		}
		return opcode == 0x4d
	}
	fmt.Println("All checks passed")
	return true
}

// CheckSequence -
func CheckSequence(tx *btc.Tx, inp int, seq int64) bool {
	if tx.Version < 2 {
		return false
	}

	toseq := int64(tx.TxIn[inp].Sequence)

	if (toseq & SequenceLocktimeDisableFlag) != 0 {
		return false
	}

	// Mask off any bits that do not have consensus-enforced meaning
	// before doing the integer comparisons
	const nLockTimeMask = SequenceLocktimeTypeFlag | SequenceLocktimeMask
	txToSequenceMasked := toseq & nLockTimeMask
	nSequenceMasked := seq & nLockTimeMask

	if !((txToSequenceMasked < SequenceLocktimeTypeFlag && nSequenceMasked < SequenceLocktimeTypeFlag) ||
		(txToSequenceMasked >= SequenceLocktimeTypeFlag && nSequenceMasked >= SequenceLocktimeTypeFlag)) {
		return false
	}

	// Now that we know we're comparing apples-to-apples, the
	// comparison is a simple numeric one.
	if nSequenceMasked > txToSequenceMasked {
		return false
	}

	return true
}
