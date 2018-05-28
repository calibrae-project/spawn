// Package common -
package common

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ParallelCoinTeam/duod/lib/logg"
)

var (
	bwMutex      sync.Mutex
	dlLastSec    = time.Now().Unix()
	dlBytesSoFar int
	// DlBytesPrevSec -
	DlBytesPrevSec [0x10000]uint64 // this buffer takes 524288 bytes (hope it's not a problem)
	// DlBytesPrevSecIdx -
	DlBytesPrevSecIdx uint16
	dlBytesPeriod     uint64
	// DlBytesTotal -
	DlBytesTotal  uint64
	uploadLimit   uint64
	downloadLimit uint64
	ulLastSec     = time.Now().Unix()
	ulBytesSoFar  int
	// UlBytesPrevSec -
	UlBytesPrevSec [0x10000]uint64 // this buffer takes 524288 bytes (hope it's not a problem)
	// UlBytesPrevSecIdx -
	UlBytesPrevSecIdx uint16
	ulBytesPeriod     uint64
	// UlBytesTotal -
	UlBytesTotal uint64
)

// TickRecv -
func TickRecv() (ms int) {
	tn := time.Now()
	ms = tn.Nanosecond() / 1e6
	now := tn.Unix()
	if now != dlLastSec {
		for now-dlLastSec != 1 {
			DlBytesPrevSec[DlBytesPrevSecIdx] = 0
			DlBytesPrevSecIdx++
			dlLastSec++
		}
		DlBytesPrevSec[DlBytesPrevSecIdx] = dlBytesPeriod
		DlBytesPrevSecIdx++
		dlBytesPeriod = 0
		dlBytesSoFar = 0
		dlLastSec = now
	}
	return
}

// TickSent -
func TickSent() (ms int) {
	tn := time.Now()
	ms = tn.Nanosecond() / 1e6
	now := tn.Unix()
	if now != ulLastSec {
		for now-ulLastSec != 1 {
			UlBytesPrevSec[UlBytesPrevSecIdx] = 0
			UlBytesPrevSecIdx++
			ulLastSec++
		}
		UlBytesPrevSec[UlBytesPrevSecIdx] = ulBytesPeriod
		UlBytesPrevSecIdx++
		ulBytesPeriod = 0
		ulBytesSoFar = 0
		ulLastSec = now
	}
	return
}

// SockRead -
// Reads the given number of bytes, but respecting the download limit
// Returns -1 and no error if we can't read any data now, because of bw limit
func SockRead(con net.Conn, buf []byte) (n int, e error) {
	var toread int
	bwMutex.Lock()
	ms := TickRecv()
	if DownloadLimit() == 0 {
		toread = len(buf)
	} else {
		toread = ms*int(DownloadLimit())/1000 - dlBytesSoFar
		if toread > len(buf) {
			toread = len(buf)
			if toread > 4096 {
				toread = 4096
			}
		} else if toread < 0 {
			toread = 0
		}
	}
	dlBytesSoFar += toread
	bwMutex.Unlock()

	if toread > 0 {
		// Wait 10 millisecond for a data, timeout if nothing there
		con.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		n, e = con.Read(buf[:toread])
		bwMutex.Lock()
		dlBytesSoFar -= toread
		if n > 0 {
			dlBytesSoFar += n
			DlBytesTotal += uint64(n)
			dlBytesPeriod += uint64(n)
		}
		bwMutex.Unlock()
	} else {
		n = -1
	}
	return
}

// SockWrite -
// Send all the bytes, but respect the upload limit (force delays)
// Returns -1 and no error if we can't send any data now, because of bw limit
func SockWrite(con net.Conn, buf []byte) (n int, e error) {
	var tosend int
	bwMutex.Lock()
	ms := TickSent()
	if UploadLimit() == 0 {
		tosend = len(buf)
	} else {
		tosend = ms*int(UploadLimit())/1000 - ulBytesSoFar
		if tosend > len(buf) {
			tosend = len(buf)
			if tosend > 4096 {
				tosend = 4096
			}
		} else if tosend < 0 {
			tosend = 0
		}
	}
	ulBytesSoFar += tosend
	bwMutex.Unlock()
	if tosend > 0 {
		// We used to have SetWriteDeadline() here, but it was causing problems because
		// in case of a timeout returned "n" was always 0, even if some data got sent.
		n, e = con.Write(buf[:tosend])
		bwMutex.Lock()
		ulBytesSoFar -= tosend
		if n > 0 {
			ulBytesSoFar += n
			UlBytesTotal += uint64(n)
			ulBytesPeriod += uint64(n)
		}
		bwMutex.Unlock()
	} else {
		n = -1
	}
	return
}

// LockBw -
func LockBw() {
	bwMutex.Lock()
}

// UnlockBw -
func UnlockBw() {
	bwMutex.Unlock()
}

// GetAvgBW -
func GetAvgBW(arr []uint64, idx uint16, cnt int) uint64 {
	var sum uint64
	if cnt <= 0 {
		return 0
	}
	for i := 0; i < cnt; i++ {
		idx--
		sum += arr[idx]
	}
	return sum / uint64(cnt)
}

// PrintBWStats -
func PrintBWStats() {
	bwMutex.Lock()
	TickRecv()
	TickSent()
	logg.Infof("Downloading at %d/%d KB/s, %s total",
		GetAvgBW(DlBytesPrevSec[:], DlBytesPrevSecIdx, 5)>>10, DownloadLimit()>>10, BytesToString(DlBytesTotal))
	logg.Infof("  |  Uploading at %d/%d KB/s, %s total\n",
		GetAvgBW(UlBytesPrevSec[:], UlBytesPrevSecIdx, 5)>>10, UploadLimit()>>10, BytesToString(UlBytesTotal))
	bwMutex.Unlock()
	return
}

// SetDownloadLimit -
func SetDownloadLimit(val uint64) {
	atomic.StoreUint64(&downloadLimit, val)
}

// DownloadLimit -
func DownloadLimit() uint64 {
	return atomic.LoadUint64(&downloadLimit)
}

// SetUploadLimit -
func SetUploadLimit(val uint64) {
	atomic.StoreUint64(&uploadLimit, val)
}

// UploadLimit -
func UploadLimit() (res uint64) {
	return atomic.LoadUint64(&uploadLimit)
}
