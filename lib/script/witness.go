package script

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/ParallelCoinTeam/duod/lib/btc"
)

type witnessCtx struct {
	stack scrStack
}

func (w *witnessCtx) IsNull() bool {
	return w.stack.size() == 0
}

// VerifyWitnessProgram -
func VerifyWitnessProgram(witness *witnessCtx, amount uint64, tx *btc.Tx, inp int, witversion int, program []byte, flags uint32) bool {
	var stack scrStack
	var scriptPubKey []byte

	if DebugScr {
		fmt.Println("*****************VerifyWitnessProgram", len(tx.SegWit), witversion, flags, witness.stack.size(), len(program))
	}

	if witversion == 0 {
		if len(program) == 32 {
			// Version 0 segregated witness program: SHA256(CScript) inside the program, CScript + inputs in witness
			if witness.stack.size() == 0 {
				if DebugError {
					fmt.Println("SCRIPT_ERR_WITNESS_PROGRAM_WITNESS_EMPTY")
				}
				return false
			}
			scriptPubKey = witness.stack.pop()
			sha := sha256.New()
			sha.Write(scriptPubKey)
			sum := sha.Sum(nil)
			if !bytes.Equal(program, sum) {
				if DebugError {
					fmt.Println("32-SCRIPT_ERR_WITNESS_PROGRAM_MISMATCH")
					fmt.Println(hex.EncodeToString(program))
					fmt.Println(hex.EncodeToString(sum))
					fmt.Println(hex.EncodeToString(scriptPubKey))
				}
				return false
			}
			stack.copyFrom(&witness.stack)
			witness.stack.push(scriptPubKey)
		} else if len(program) == 20 {
			// Special case for pay-to-pubkeyhash; signature + pubkey in witness
			if witness.stack.size() != 2 {
				if DebugError {
					fmt.Println("20-SCRIPT_ERR_WITNESS_PROGRAM_MISMATCH", tx.Hash.String())
				}
				return false
			}

			scriptPubKey = make([]byte, 25)
			scriptPubKey[0] = 0x76
			scriptPubKey[1] = 0xa9
			scriptPubKey[2] = 0x14
			copy(scriptPubKey[3:23], program)
			scriptPubKey[23] = 0x88
			scriptPubKey[24] = 0xac
			stack.copyFrom(&witness.stack)
		} else {
			if DebugError {
				fmt.Println("SCRIPT_ERR_WITNESS_PROGRAM_WRONG_LENGTH")
			}
			return false
		}
	} else if (flags & VerWitnessProg) != 0 {
		if DebugError {
			fmt.Println("SCRIPT_ERR_DISCOURAGE_UPGRADABLE_WITNESS_PROGRAM")
		}
		return false
	} else {
		// Higher version witness scripts return true for future softfork compatibility
		return true
	}

	if DebugScr {
		fmt.Println("*****************", stack.size())
	}
	// Disallow stack item size > MaxScriptElementSize in witness stack
	for i := 0; i < stack.size(); i++ {
		if len(stack.at(i)) > btc.MaxScriptElementSize {
			if DebugError {
				fmt.Println("SCRIPT_ERR_PUSH_SIZE")
			}
			return false
		}
	}

	if !evalScript(scriptPubKey, amount, &stack, tx, inp, flags, SigVersionWitnessV0) {
		return false
	}

	// Scripts inside witness implicitly require cleanstack behaviour
	if stack.size() != 1 {
		if DebugError {
			fmt.Println("SCRIPT_ERR_EVAL_FALSE")
		}
		return false
	}

	if !stack.topBool(-1) {
		if DebugError {
			fmt.Println("SCRIPT_ERR_EVAL_FALSE")
		}
		return false
	}
	return true
}
