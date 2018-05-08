package utxo

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/calibrae-project/spawn/lib/btc"
	"github.com/calibrae-project/spawn/lib/others/sys"
)

const (
	// UXTORecordsPrealloc -
	UXTORecordsPrealloc = 25e6
)

var (
	// UXTOWritingTimeTarget -
	UXTOWritingTimeTarget = 5 * time.Minute // Take it easy with flushing UTXO.db onto disk
)

// FunctionWalkUnspent -
type FunctionWalkUnspent func(*Rec)

// CallbackFunctions -
type CallbackFunctions struct {
	// If NotifyTx is set, it will be called each time a new unspent
	// output is being added or removed. When being removed, btc.TxOut is nil.
	NotifyTxAdd func(*Rec)
	NotifyTxDel func(*Rec, []bool)
}

// BlockChanges - Used to pass block's changes to UnspentDB
type BlockChanges struct {
	Height          uint32
	LastKnownHeight uint32 // put here zero to disable this feature
	AddList         []*Rec
	DeledTxs        map[[32]byte][]bool
	UndoData        map[[32]byte]*Rec
}

// UnspentDB -
type UnspentDB struct {
	HashMap      map[KeyType][]byte
	sync.RWMutex // used to access HashMap

	LastBlockHash    []byte
	LastBlockHeight  uint32
	dirUXTO, dirUndo string
	volatimemode     bool
	UnwindBufLen     uint32
	DirtyDB          sys.SyncBool
	sync.Mutex

	abortwritingnow   chan bool
	WritingInProgress sys.SyncBool
	writingDone       sync.WaitGroup
	lastFileClosed    sync.WaitGroup

	CurrentHeightOnDisk uint32
	hurryup             chan bool
	DoNotWriteUndoFiles bool
	CB                  CallbackFunctions
}

// NewUnspentOpts -
type NewUnspentOpts struct {
	Dir             string
	Rescan          bool
	VolatimeMode    bool
	UnwindBufferLen uint32
	CB              CallbackFunctions
	AbortNow        *bool
}

// NewUnspentDB -
func NewUnspentDB(opts *NewUnspentOpts) (db *UnspentDB) {
	//var maxbl_fn string
	db = new(UnspentDB)
	db.dirUXTO = opts.Dir
	db.dirUndo = db.dirUXTO + "undo" + string(os.PathSeparator)
	db.volatimemode = opts.VolatimeMode
	db.UnwindBufLen = 256
	db.CB = opts.CB
	db.abortwritingnow = make(chan bool, 1)
	db.hurryup = make(chan bool, 1)

	os.MkdirAll(db.dirUndo, 0770)

	os.Remove(db.dirUndo + "tmp")
	os.Remove(db.dirUXTO + "UTXO.db.tmp")

	if opts.Rescan {
		db.HashMap = make(map[KeyType][]byte, UXTORecordsPrealloc)
		return
	}

	// Load data form disk
	var k KeyType
	var countDown, countDownFrom, perc int
	var le uint64
	var u64, totRecs uint64
	var info string
	var rd *bufio.Reader
	var of *os.File

	fname := "UTXO.db"

redo:
	of, er := os.Open(db.dirUXTO + fname)
	if er != nil {
		goto fatal_error
	}

	rd = bufio.NewReaderSize(of, 0x100000)

	er = binary.Read(rd, binary.LittleEndian, &u64)
	if er != nil {
		goto fatal_error
	}
	db.LastBlockHeight = uint32(u64)

	db.LastBlockHash = make([]byte, 32)
	_, er = rd.Read(db.LastBlockHash)
	if er != nil {
		goto fatal_error
	}
	er = binary.Read(rd, binary.LittleEndian, &u64)
	if er != nil {
		goto fatal_error
	}

	//fmt.Println("Last block height", db.LastBlockHeight, "   Number of records", u64)
	countDownFrom = int(u64 / 100)
	perc = 0

	db.HashMap = make(map[KeyType][]byte, int(u64))
	info = fmt.Sprint("\rLoading ", u64, " transactions from ", fname, " - ")

	for totRecs = 0; totRecs < u64; totRecs++ {
		if opts.AbortNow != nil && *opts.AbortNow {
			break
		}
		le, er = btc.ReadVLen(rd)
		if er != nil {
			goto fatal_error
		}

		er = btc.ReadAll(rd, k[:])
		if er != nil {
			goto fatal_error
		}

		b := malloc(uint32(int(le) - UtxoIdxLen))
		er = btc.ReadAll(rd, b)
		if er != nil {
			goto fatal_error
		}

		// we don't lock RWMutex here as this code is only used during init phase, when no other routines are running
		db.HashMap[k] = b

		if countDown == 0 {
			fmt.Print(info, perc, "% complete ... ")
			perc++
			countDown = countDownFrom
		} else {
			countDown--
		}
	}
	of.Close()

	fmt.Print("\r                                                              \r")

	db.CurrentHeightOnDisk = db.LastBlockHeight

	return

fatal_error:
	if of != nil {
		of.Close()
	}

	println(er.Error())
	if fname != "UTXO.old" {
		fname = "UTXO.old"
		goto redo
	}
	db.LastBlockHeight = 0
	db.LastBlockHash = nil
	db.HashMap = make(map[KeyType][]byte, UXTORecordsPrealloc)

	return
}

func (db *UnspentDB) save() {
	//var countDown, countDownFrom, perc int
	var abort, hurryup, checkTime bool
	var totalRecords, currentRecords, dataProgress, timeProgress int64

	os.Rename(db.dirUXTO+"UTXO.db", db.dirUXTO+"UTXO.old")
	dataChannel := make(chan []byte, 100)
	exitChannel := make(chan bool, 1)

	startTime := time.Now()

	db.RWMutex.RLock()

	totalRecords = int64(len(db.HashMap))

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint64(db.LastBlockHeight))
	buf.Write(db.LastBlockHash)
	binary.Write(buf, binary.LittleEndian, uint64(totalRecords))

	// The data is written in a separate process
	// so we can abort without waiting for disk.
	db.lastFileClosed.Add(1)
	go func(fname string) {
		of, er := os.Create(fname)
		if er != nil {
			println("Create file:", er.Error())
			return
		}

		var dat []byte
		var abort, exit bool

		for !exit || len(dataChannel) > 0 {
			select {

			case dat = <-dataChannel:
				if len(exitChannel) > 0 {
					if abort = <-exitChannel; abort {
						goto exit
					} else {
						exit = true
					}
				}
				of.Write(dat)

			case abort = <-exitChannel:
				if abort {
					goto exit
				} else {
					exit = true
				}
			}
		}
	exit:
		if abort {
			of.Close() // abort
			os.Remove(fname)
		} else {
			of.Close()
			os.Rename(fname, db.dirUXTO+"UTXO.db")
		}
		db.lastFileClosed.Done()
	}(db.dirUXTO + btc.NewUint256(db.LastBlockHash).String() + ".db.tmp")

	for k, v := range db.HashMap {
		if checkTime {
			checkTime = false
			dataProgress = int64((currentRecords << 20) / totalRecords)
			timeProgress = int64((time.Now().Sub(startTime) << 20) / UXTOWritingTimeTarget)
			if dataProgress > timeProgress {
				select {
				case <-db.abortwritingnow:
					abort = true
					goto finito
				case <-db.hurryup:
					hurryup = true
				case <-time.After(time.Millisecond):
				}
			}
		}

		for len(dataChannel) >= cap(dataChannel) {
			select {
			case <-db.abortwritingnow:
				abort = true
				goto finito
			case <-db.hurryup:
				hurryup = true
			case <-time.After(time.Millisecond):
			}
		}

		btc.WriteVlen(buf, uint64(UtxoIdxLen+len(v)))
		buf.Write(k[:])
		buf.Write(v)
		if buf.Len() > 0x10000 {
			dataChannel <- buf.Bytes()
			buf = new(bytes.Buffer)
		}

		if !hurryup {
			currentRecords++
			if (currentRecords & 0x3f) == 0 {
				checkTime = true
			}
		}
	}
finito:
	db.RWMutex.RUnlock()

	if !abort && buf.Len() > 0 {
		dataChannel <- buf.Bytes()
	}
	exitChannel <- abort

	if !abort {
		db.DirtyDB.Clr()
		//println("utxo written OK in", time.Now().Sub(startTime).String(), timewaits)
		db.CurrentHeightOnDisk = db.LastBlockHeight
	}
	db.WritingInProgress.Clr()
	db.writingDone.Done()
}

// CommitBlockTxs - Commit the given add/del transactions to UTXO and Unwind DBs
func (db *UnspentDB) CommitBlockTxs(changes *BlockChanges, blhash []byte) (e error) {
	undoFn := fmt.Sprint(db.dirUndo, changes.Height)

	db.Mutex.Lock()
	defer db.Mutex.Unlock()
	db.abortWriting()

	if changes.UndoData != nil {
		bu := new(bytes.Buffer)
		bu.Write(blhash)
		if changes.UndoData != nil {
			for _, xx := range changes.UndoData {
				bin := xx.Serialize(true)
				btc.WriteVlen(bu, uint64(len(bin)))
				bu.Write(bin)
			}
		}
		ioutil.WriteFile(db.dirUndo+"tmp", bu.Bytes(), 0666)
		os.Rename(db.dirUndo+"tmp", undoFn)
	}

	db.commit(changes)

	if db.LastBlockHash == nil {
		db.LastBlockHash = make([]byte, 32)
	}
	copy(db.LastBlockHash, blhash)
	db.LastBlockHeight = changes.Height

	if changes.Height > db.UnwindBufLen {
		os.Remove(fmt.Sprint(db.dirUndo, changes.Height-db.UnwindBufLen))
	}

	db.DirtyDB.Set()
	return
}

// UndoBlockTxs -
func (db *UnspentDB) UndoBlockTxs(bl *btc.Block, newhash []byte) {
	db.Mutex.Lock()
	defer db.Mutex.Unlock()
	db.abortWriting()

	for _, tx := range bl.Txs {
		lst := make([]bool, len(tx.TxOut))
		for i := range lst {
			lst[i] = true
		}
		db.del(tx.Hash.Hash[:], lst)
	}

	fn := fmt.Sprint(db.dirUndo, db.LastBlockHeight)
	var addback []*Rec

	if _, er := os.Stat(fn); er != nil {
		fn += ".tmp"
	}

	dat, er := ioutil.ReadFile(fn)
	if er != nil {
		panic(er.Error())
	}

	off := 32 // ship the block hash
	for off < len(dat) {
		le, n := btc.VLen(dat[off:])
		off += n
		qr := FullUtxoRec(dat[off : off+le])
		off += le
		addback = append(addback, qr)
	}

	for _, tx := range addback {
		if db.CB.NotifyTxAdd != nil {
			db.CB.NotifyTxAdd(tx)
		}

		var ind KeyType
		copy(ind[:], tx.TxID[:])
		db.RWMutex.RLock()
		v := db.HashMap[ind]
		db.RWMutex.RUnlock()
		if v != nil {
			oldrec := NewUtxoRec(ind, v)
			for a := range tx.Outs {
				if tx.Outs[a] == nil {
					tx.Outs[a] = oldrec.Outs[a]
				}
			}
		}
		db.RWMutex.Lock()
		db.HashMap[ind] = mallocAndCopy(tx.Bytes())
		db.RWMutex.Unlock()
	}

	os.Remove(fn)
	db.LastBlockHeight--
	copy(db.LastBlockHash, newhash)
	db.DirtyDB.Set()
}

// Idle - Call it when the main thread is idle
func (db *UnspentDB) Idle() bool {
	if db.volatimemode {
		return false
	}

	db.Mutex.Lock()
	defer db.Mutex.Unlock()

	if db.DirtyDB.Get() && !db.WritingInProgress.Get() {
		db.WritingInProgress.Set()
		db.writingDone.Add(1)
		go db.save() // this one will call db.writingDone.Done()
		return true
	}

	return false
}

// HurryUp -
func (db *UnspentDB) HurryUp() {
	select {
	case db.hurryup <- true:
	default:
	}
}

// Close - Flush the data and close all the files
func (db *UnspentDB) Close() {
	db.HurryUp()
	db.volatimemode = false
	db.Idle()
	db.writingDone.Wait()
	db.lastFileClosed.Wait()
}

// UnspentGet - Get given unspent output
func (db *UnspentDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut) {
	var ind KeyType
	var v []byte
	copy(ind[:], po.Hash[:])

	db.RWMutex.RLock()
	v = db.HashMap[ind]
	db.RWMutex.RUnlock()
	if v != nil {
		res = OneUtxoRec(ind, v, po.Vout)
	}

	return
}

// TxPresent - Returns true if gived TXID is in UTXO
func (db *UnspentDB) TxPresent(id *btc.Uint256) (res bool) {
	var ind KeyType
	copy(ind[:], id.Hash[:])
	db.RWMutex.RLock()
	_, res = db.HashMap[ind]
	db.RWMutex.RUnlock()
	return
}

func (db *UnspentDB) del(hash []byte, outs []bool) {
	var ind KeyType
	copy(ind[:], hash)
	db.RWMutex.RLock()
	v := db.HashMap[ind]
	db.RWMutex.RUnlock()
	if v == nil {
		return // no such txid in UTXO (just ignorde delete request)
	}
	rec := NewUtxoRec(ind, v)
	if db.CB.NotifyTxDel != nil {
		db.CB.NotifyTxDel(rec, outs)
	}
	var anyout bool
	for i, rm := range outs {
		if rm {
			rec.Outs[i] = nil
		} else if rec.Outs[i] != nil {
			anyout = true
		}
	}
	db.RWMutex.Lock()
	if anyout {
		db.HashMap[ind] = mallocAndCopy(rec.Bytes())
	} else {
		delete(db.HashMap, ind)
	}
	db.RWMutex.Unlock()
	free(v)
}

func (db *UnspentDB) commit(changes *BlockChanges) {
	// Now aplly the unspent changes
	for _, rec := range changes.AddList {
		var ind KeyType
		copy(ind[:], rec.TxID[:])
		if db.CB.NotifyTxAdd != nil {
			db.CB.NotifyTxAdd(rec)
		}
		db.RWMutex.Lock()
		db.HashMap[ind] = mallocAndCopy(rec.Bytes())
		db.RWMutex.Unlock()
	}
	for k, v := range changes.DeledTxs {
		db.del(k[:], v)
	}
}

// AbortWriting -
func (db *UnspentDB) AbortWriting() {
	db.Mutex.Lock()
	db.abortWriting()
	db.Mutex.Unlock()
}

func (db *UnspentDB) abortWriting() {
	if db.WritingInProgress.Get() {
		db.abortwritingnow <- true
		db.writingDone.Wait()
		select {
		case <-db.abortwritingnow:
		default:
		}
	}
}

// UTXOStats -
func (db *UnspentDB) UTXOStats() (s string) {
	var outcnt, sum, sumcb uint64
	var totdatasize, unspendable, unspendableRecs, unspendableBytes uint64

	db.RWMutex.RLock()

	lele := len(db.HashMap)

	for k, v := range db.HashMap {
		totdatasize += uint64(len(v) + 8)
		rec := NewUtxoRecStatic(k, v)
		var spendableFound bool
		for _, r := range rec.Outs {
			if r != nil {
				outcnt++
				sum += r.Value
				if rec.Coinbase {
					sumcb += r.Value
				}
				if len(r.PKScr) > 0 && r.PKScr[0] == 0x6a {
					unspendable++
					unspendableBytes += uint64(8 + len(r.PKScr))
				} else {
					spendableFound = true
				}
			}
		}
		if !spendableFound {
			unspendableRecs++
		}
	}

	db.RWMutex.RUnlock()

	s = fmt.Sprintf("UNSPENT: %.8f BTC in %d outs from %d txs. %.8f BTC in coinbase.\n",
		float64(sum)/1e8, outcnt, lele, float64(sumcb)/1e8)
	s += fmt.Sprintf(" TotalData:%.1fMB  MaxTxOutCnt:%d  DirtyDB:%t  Writing:%t  Abort:%t\n",
		float64(totdatasize)/1e6, len(rec_outs), db.DirtyDB.Get(), db.WritingInProgress.Get(), len(db.abortwritingnow) > 0)
	s += fmt.Sprintf(" Last Block : %s @ %d\n", btc.NewUint256(db.LastBlockHash).String(),
		db.LastBlockHeight)
	s += fmt.Sprintf(" Unspendable outputs: %d (%dKB)  txs:%d\n",
		unspendable, unspendableBytes>>10, unspendableRecs)

	return
}

// GetStats - Return DB statistics
func (db *UnspentDB) GetStats() (s string) {
	db.RWMutex.RLock()
	hml := len(db.HashMap)
	db.RWMutex.RUnlock()

	s = fmt.Sprintf("UNSPENT: %d records. MaxTxOutCnt:%d  DirtyDB:%t  Writing:%t  Abort:%t\n",
		hml, len(rec_outs), db.DirtyDB.Get(), db.WritingInProgress.Get(), len(db.abortwritingnow) > 0)
	s += fmt.Sprintf(" Last Block : %s @ %d\n", btc.NewUint256(db.LastBlockHash).String(),
		db.LastBlockHeight)
	return
}

// PurgeUnspendable -
func (db *UnspentDB) PurgeUnspendable(all bool) {
	var unspendableTxs, unspendableRecs uint64
	db.Mutex.Lock()
	db.abortWriting()

	db.RWMutex.Lock()

	for k, v := range db.HashMap {
		rec := NewUtxoRecStatic(k, v)
		var spendableFound bool
		var recordRemoved uint64
		for idx, r := range rec.Outs {
			if r != nil {
				if len(r.PKScr) > 0 && r.PKScr[0] == 0x6a {
					unspendableRecs++
					if all {
						rec.Outs[idx] = nil
						recordRemoved++
					}
				} else {
					spendableFound = true
				}
			}
		}
		if !spendableFound {
			free(v)
			delete(db.HashMap, k)
			unspendableTxs++
		} else if recordRemoved > 0 {
			free(v)
			db.HashMap[k] = mallocAndCopy(rec.Serialize(false))
			unspendableRecs += recordRemoved
		}
	}
	db.RWMutex.Unlock()

	db.Mutex.Unlock()

	fmt.Println("Purged", unspendableTxs, "transactions and", unspendableRecs, "extra records")
}
