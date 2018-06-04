package main

/*
  This is a ECVerify speedup that works only with Windows

  Use secp256k1.dll from Duod/tools/sipa_dll
  or build one yourself.

*/

import (
	"encoding/hex"
	"os"
	"syscall"
	"unsafe"

	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/L"
)

var (
	dll = syscall.NewLazyDLL("secp256k1.dll")
	// DLLECVerify -
	DLLECVerify = dll.NewProc("ECVerify")
)

// ECVerify -
func ECVerify(pkey, sign, hash []byte) bool {
	r1, _, _ := syscall.Syscall6(DLLECVerify.Addr(), 6,
		uintptr(unsafe.Pointer(&hash[0])), uintptr(32),
		uintptr(unsafe.Pointer(&sign[0])), uintptr(len(sign)),
		uintptr(unsafe.Pointer(&pkey[0])), uintptr(len(pkey)))
	return r1 == 1
}

func verify() bool {
	key, _ := hex.DecodeString("020eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d")
	sig, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	has, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	return ECVerify(key, sig, has)
}

func init() {
	if verify() {
		L.Debug("Using secp256k1.dll by sipa for ECVerify")
		btc.ECVerify = ECVerify
	} else {
		L.Debug("ERROR: Could not initiate secp256k1.dll")
		os.Exit(1)
	}
}
