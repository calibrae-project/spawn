package sys

import (
	"fmt"
	"sync/atomic"
)

// SyncBool -
type SyncBool struct {
	val int32
}

// Get -
func (b *SyncBool) Get() bool {
	return atomic.LoadInt32(&b.val) != 0
}

// Set -
func (b *SyncBool) Set() {
	atomic.StoreInt32(&b.val, 1)
}

// Clr -
func (b *SyncBool) Clr() {
	atomic.StoreInt32(&b.val, 0)
}

// MarshalText -
func (b *SyncBool) MarshalText() (text []byte, err error) {
	return []byte(fmt.Sprint(b.Get())), nil
}

// Store -
func (b *SyncBool) Store(val bool) {
	if val {
		b.Set()
	} else {
		b.Clr()
	}
}

// SyncInt -
type SyncInt struct {
	val int64
}

// Get -
func (b *SyncInt) Get() int {
	return int(atomic.LoadInt64(&b.val))
}

// Store -
func (b *SyncInt) Store(val int) {
	atomic.StoreInt64(&b.val, int64(val))
}

// MarshalText -
func (b *SyncInt) MarshalText() (text []byte, err error) {
	return []byte(fmt.Sprint(b.Get())), nil
}
