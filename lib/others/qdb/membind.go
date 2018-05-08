package qdb

import (
	"os"
	"reflect"
	"sync/atomic"
	"unsafe"
)

var (
	membind_use_wrapper bool
	_heap_alloc         func(le uint32) data_ptr_t
	_heap_free          func(ptr data_ptr_t)
	_heap_store         func(v []byte) data_ptr_t
)

type data_ptr_t unsafe.Pointer

func (idx *oneIdx) FreeData() {
	if idx.data == nil {
		return
	}
	if membind_use_wrapper {
		_heap_free(idx.data)
		atomic.AddInt64(&ExtraMemoryConsumed, -int64(idx.datlen))
		atomic.AddInt64(&ExtraMemoryAllocCnt, -1)
	}
	idx.data = nil
}

func (idx *oneIdx) Slice() (res []byte) {
	if membind_use_wrapper {
		res = *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data: uintptr(idx.data), Len: int(idx.datlen), Cap: int(idx.datlen)}))
	} else {
		res = *(*[]byte)(idx.data)
	}
	return
}

func newIdx(v []byte, f uint32) (r *oneIdx) {
	r = new(oneIdx)
	r.datlen = uint32(len(v))
	r.SetData(v)
	r.flags = f
	return
}

// SetData -
func (idx *oneIdx) SetData(v []byte) {
	if membind_use_wrapper {
		idx.data = _heap_store(v)
		atomic.AddInt64(&ExtraMemoryConsumed, int64(idx.datlen))
		atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
	} else {
		idx.data = data_ptr_t(&v)
	}
}

func (idx *oneIdx) LoadData(f *os.File) {
	if membind_use_wrapper {
		idx.data = _heap_alloc(idx.datlen)
		atomic.AddInt64(&ExtraMemoryConsumed, int64(idx.datlen))
		atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
		f.Seek(int64(idx.datpos), os.SEEK_SET)
		f.Read(*(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data: uintptr(idx.data), Len: int(idx.datlen), Cap: int(idx.datlen)})))
	} else {
		ptr := make([]byte, int(idx.datlen))
		idx.data = data_ptr_t(&ptr)
		f.Seek(int64(idx.datpos), os.SEEK_SET)
		f.Read(ptr)
	}
}
