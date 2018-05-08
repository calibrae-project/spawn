package spiadll

import (
	"syscall"
	"unsafe"
)

var (
	advapi32 = syscall.NewLazyDLL("secp256k1.dll")
	// DLLECVerify -
	DLLECVerify = advapi32.NewProc("ECVerify")
)

// ECVerify -
func ECVerify(pkey, sign, hash []byte) int32 {
	r1, _, _ := syscall.Syscall6(DLLECVerify.Addr(), 6,
		uintptr(unsafe.Pointer(&hash[0])), uintptr(32),
		uintptr(unsafe.Pointer(&sign[0])), uintptr(len(sign)),
		uintptr(unsafe.Pointer(&pkey[0])), uintptr(len(pkey)))
	return int32(r1)
}
