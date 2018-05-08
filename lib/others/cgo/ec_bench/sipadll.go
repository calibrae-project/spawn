package main

import (
	"encoding/hex"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	secp256k1 = syscall.NewLazyDLL("secp256k1.dll")
	// DLLECVerify -
	DLLECVerify = secp256k1.NewProc("ECVerify")
)

// ECVerify -
func ECVerify(pkey, sign, hash []byte) int32 {
	r1, _, _ := syscall.Syscall6(DLLECVerify.Addr(), 6,
		uintptr(unsafe.Pointer(&hash[0])), uintptr(32),
		uintptr(unsafe.Pointer(&sign[0])), uintptr(len(sign)),
		uintptr(unsafe.Pointer(&pkey[0])), uintptr(len(pkey)))
	return int32(r1)
}

// CNT -
var CNT = 100e3

func main() {
	key, _ := hex.DecodeString("040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	sig, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	msg, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	var wg sync.WaitGroup
	sta := time.Now()
	for i := 0; i < CNT; i++ {
		wg.Add(1)
		go func() {
			if ECVerify(key, sig, msg) != 1 {
				println("Verify error")
			}
			wg.Done()
		}()
	}
	wg.Wait()
	sto := time.Now()
	println((sto.UnixNano()-sta.UnixNano())/int64(CNT*1000), "us per ECDSA_Verify")
}
