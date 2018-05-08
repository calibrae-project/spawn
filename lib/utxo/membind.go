package utxo

import (
	"sync/atomic"
)

var (
	malloc = func(le uint32) []byte {
		return make([]byte, int(le))
	}

	free = func(v []byte) {
	}

	mallocAndCopy = func(v []byte) []byte {
		return v
	}
	// MembindInit -
	MembindInit = func() {}
)

var (
	extraMemoryConsumed int64 // if we are using the glibc memory manager
	extraMemoryAllocCnt int64 // if we are using the glibc memory manager
)

// ExtraMemoryConsumed -
func ExtraMemoryConsumed() int64 {
	return atomic.LoadInt64(&extraMemoryConsumed)
}

// ExtraMemoryAllocCnt -
func ExtraMemoryAllocCnt() int64 {
	return atomic.LoadInt64(&extraMemoryAllocCnt)
}
