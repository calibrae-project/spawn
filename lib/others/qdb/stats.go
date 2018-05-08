package qdb

import (
	"fmt"
	"sort"
	"sync"
)

var (
	counter      = make(map[string]uint64)
	counterMutex sync.Mutex
)

func cnt(k string) {
	cntadd(k, 1)
}

func cntadd(k string, val uint64) {
	counterMutex.Lock()
	counter[k] += val
	counterMutex.Unlock()
}

// GetStats -
func GetStats() (s string) {
	counterMutex.Lock()
	ck := make([]string, len(counter))
	idx := 0
	for k := range counter {
		ck[idx] = k
		idx++
	}
	sort.Strings(ck)

	for i := range ck {
		k := ck[i]
		v := counter[k]
		s += fmt.Sprintln(k, ": ", v)
	}
	counterMutex.Unlock()
	return s
}
