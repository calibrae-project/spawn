package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"

	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/chain"
	"github.com/golang/snappy"
)

/*
	blockchain.dat - contains raw blocks data, no headers, nothing
	blockchain.new - contains records of 136 bytes (all values LSB):
		[0] - flags:
			bit(0) - "trusted" flag - this block's scripts have been verified
			bit(1) - "invalid" flag - this block's scripts have failed
			bit(2) - "compressed" flag - this block's data is compressed
			bit(3) - "snappy" flag - this block is compressed with snappy (not gzip'ed)
			bit(4) - if this bit is set, bytes [32:36] carry length of uncompressed block
			bit(5) - if this bit is set, bytes [28:32] carry data file index

		Used to be:
		[4:36]  - 256-bit block hash - DEPRECATED! (hash the header to get the value)

		[4:28] - reserved
		[28:32] - specifies which blockchain.dat file is used (if not zero, the filename is: blockchain-%08x.dat)
		[32:36] - length of uncompressed block

		[36:40] - 32-bit block height (genesis is 0)
		[40:48] - 64-bit block pos in blockchain.dat file
		[48:52] - 32-bit block lenght in bytes
		[52:56] - 32-bit number of transaction in the block
		[56:136] - 80 bytes blocks header
*/

const (
	// Trusted -
	Trusted = 0x01
	// Invalid -
	Invalid = 0x02
)

var (
	flHelp             bool
	flBlock, flStop    uint
	flDir              string
	flScan, flDefrag   bool
	flSplit            string
	flSkip             uint
	flAppend           string
	flTrunc            bool
	flCommit, flVerify bool
	flSaveBl           string
	flPurgeAll         bool
	flPurgeTo          uint
	flFlags            bool
	flFrom, flTo       uint
	flTrusted          int
	flInvalid          int
	flFixLen           bool
	flFixLenAll        bool

	flMergeDat uint
	flMoveDat  uint

	flSplitDat int
	flMB       uint

	flDatIdx int

	flPurgeDatIdx bool

	buf [5 * 1024 * 1024]byte // 5MB should be anough
)

/********************************************************/
type oneIdxRec struct {
	sl   []byte
	hash [32]byte
}

func newSL(sl []byte) (r oneIdxRec) {
	r.sl = sl[:136]
	btc.ShaHash(sl[56:136], r.hash[:])
	return
}

// Flags -
func (r oneIdxRec) Flags() uint32 {
	return binary.LittleEndian.Uint32(r.sl[0:4])
}

// Height -
func (r oneIdxRec) Height() uint32 {
	return binary.LittleEndian.Uint32(r.sl[36:40])
}

// DPos -
func (r oneIdxRec) DPos() uint64 {
	return binary.LittleEndian.Uint64(r.sl[40:48])
}

// SetDPos -
func (r oneIdxRec) SetDPos(dp uint64) {
	binary.LittleEndian.PutUint64(r.sl[40:48], dp)
}

// DLen -
func (r oneIdxRec) DLen() uint32 {
	return binary.LittleEndian.Uint32(r.sl[48:52])
}

// SetDLen -
func (r oneIdxRec) SetDLen(l uint32) {
	binary.LittleEndian.PutUint32(r.sl[48:52], l)
}

// SetDatIdx -
func (r oneIdxRec) SetDatIdx(l uint32) {
	r.sl[0] |= 0x20
	binary.LittleEndian.PutUint32(r.sl[28:32], l)
}

// Hash -
func (r oneIdxRec) Hash() []byte {
	return r.hash[:]
}

// HIdx -
func (r oneIdxRec) HIdx() (h [32]byte) {
	copy(h[:], r.hash[:])
	return
}

// Parent -
func (r oneIdxRec) Parent() []byte {
	return r.sl[60:92]
}

// PIdx -
func (r oneIdxRec) PIdx() [32]byte {
	var h [32]byte
	copy(h[:], r.sl[60:92])
	return h
}

// DatIdx -
func (r oneIdxRec) DatIdx() uint32 {
	if (r.sl[0] & 0x20) != 0 {
		return binary.LittleEndian.Uint32(r.sl[28:32])
	}
	return 0
}

/********************************************************/

type oneTreeNode struct {
	off int // offset in teh idx file
	oneIdxRec
	parent *oneTreeNode
	next   *oneTreeNode
}

/********************************************************/

func printRecord(sl []byte) {
	var datIdx uint32
	if (sl[0] & 0x20) != 0 {
		datIdx = binary.LittleEndian.Uint32(sl[28:32])
	}
	bh := btc.NewSha2Hash(sl[56:136])
	fmt.Println("Block", bh.String())
	fmt.Println(" ... Height", binary.LittleEndian.Uint32(sl[36:40]),
		" Flags", fmt.Sprintf("0x%02x", sl[0]),
		" - ", binary.LittleEndian.Uint32(sl[48:52]), "bytes @",
		binary.LittleEndian.Uint64(sl[40:48]), "in", datFilename(datIdx))
	if (sl[0] & 0x10) != 0 {
		fmt.Println("     Uncompressed length:",
			binary.LittleEndian.Uint32(sl[32:36]), "bytes")
	}
	if (sl[0] & 0x20) != 0 {
		fmt.Println("     Data file index:", datIdx)
	}
	hdr := sl[56:136]
	fmt.Println("   ->", btc.NewUint256(hdr[4:36]).String())
}

func verifyBlock(blk []byte, sl oneIdxRec, off int) {
	bl, er := btc.NewBlock(blk)
	if er != nil {
		println("\nERROR verifyBlock", sl.Height(), btc.NewUint256(sl.Hash()).String(), er.Error())
		return
	}
	if !bytes.Equal(bl.Hash.Hash[:], sl.Hash()) {
		println("\nERROR verifyBlock", sl.Height(), btc.NewUint256(sl.Hash()).String(), "Header invalid")
		return
	}

	er = bl.BuildTxList()
	if er != nil {
		println("\nERROR verifyBlock", sl.Height(), btc.NewUint256(sl.Hash()).String(), er.Error())
		return
	}

	merk, _ := bl.GetMerkle()
	if !bytes.Equal(bl.MerkleRoot(), merk) {
		println("\nERROR verifyBlock", sl.Height(), btc.NewUint256(sl.Hash()).String(), "Payload invalid / Merkle mismatch")
		return
	}
}

func decompBlock(fl uint32, buf []byte) (blk []byte) {
	if (fl & chain.BlockCompressed) != 0 {
		if (fl & chain.BlockSnapped) != 0 {
			blk, _ = snappy.Decode(nil, buf)
		} else {
			gz, _ := gzip.NewReader(bytes.NewReader(buf))
			blk, _ = ioutil.ReadAll(gz)
			gz.Close()
		}
	} else {
		blk = buf
	}
	return
}

// Look for the first and last records with the given index
func lookForRange(dat []byte, _idx uint32) (minValidOff, maxValidOff int) {
	minValidOff = -1
	for off := 0; off < len(dat); off += 136 {
		sl := newSL(dat[off:])
		idx := sl.DatIdx()
		if sl.DLen() > 0 {
			if idx == _idx {
				if minValidOff == -1 {
					minValidOff = off
				}
				maxValidOff = off
			} else if minValidOff != -1 {
				break
			}
		}
	}
	return
}

func datFilename(idx uint32) string {
	if idx == 0 {
		return "blockchain.dat"
	}
	return fmt.Sprintf("blockchain-%08x.dat", idx)
}

func splitTheDataFile(parentF *os.File, idx uint32, maxlen uint64, dat []byte, minValidOff, maxValidOff int) bool {
	fname := datFilename(idx)

	if fi, _ := os.Stat(fname); fi != nil {
		fmt.Println(fi.Name(), "exist - get rid of it first")
		return false
	}

	recFrom := newSL(dat[minValidOff : minValidOff+136])
	posFrom := recFrom.DPos()

	for off := minValidOff; off <= maxValidOff; off += 136 {
		rec := newSL(dat[off : off+136])
		if rec.DLen() == 0 {
			continue
		}
		dpos := rec.DPos()
		if dpos-posFrom+uint64(rec.DLen()) > maxlen {
			if !splitTheDataFile(parentF, idx+1, maxlen, dat, off, maxValidOff) {
				return false // abort spliting
			}
			//println("truncate parent at", dpos)
			er := parentF.Truncate(int64(dpos))
			if er != nil {
				println(er.Error())
			}
			maxValidOff = off - 136
			break // go to the next stage
		}
	}

	// at this point parentF should be truncated
	f, er := os.Create(fname)
	if er != nil {
		fmt.Println(er.Error())
		return false
	}

	parentF.Seek(int64(posFrom), os.SEEK_SET)
	for {
		n, _ := parentF.Read(buf[:])
		if n > 0 {
			f.Write(buf[:n])
		}
		if n != len(buf) {
			break
		}
	}

	//println(".. child split", fname, "at offs", minValidOff/136, "...", maxValidOff/136, "fpos:", posFrom, " maxlen:", maxlen)
	for off := minValidOff; off <= maxValidOff; off += 136 {
		sl := newSL(dat[off : off+136])
		sl.SetDatIdx(idx)
		sl.SetDPos(sl.DPos() - posFrom)
	}
	// flush blockchain.new to disk wicth each noe split for safety
	ioutil.WriteFile("blockchain.tmp", dat, 0600)
	os.Rename("blockchain.tmp", "blockchain.new")

	return true
}

func calcTotalSize(dat []byte) (res uint64) {
	for off := 0; off < len(dat); off += 136 {
		sl := newSL(dat[off : off+136])
		res += uint64(sl.DLen())
	}
	return
}

func main() {
	flag.BoolVar(&flHelp, "h", false, "Show help")
	flag.UintVar(&flBlock, "block", 0, "Print details of the given block number (or start -verify from it)")
	flag.BoolVar(&flScan, "scan", false, "Scan database for first extra blocks")
	flag.BoolVar(&flDefrag, "defrag", false, "Purge all the orphaned blocks")
	flag.UintVar(&flStop, "stop", 0, "Stop after so many scan errors")
	flag.StringVar(&flDir, "dir", "", "Use blockdb from this directory")
	flag.StringVar(&flSplit, "split", "", "Split blockdb at this block's hash")
	flag.UintVar(&flSkip, "skip", 0, "Skip this many blocks when splitting")
	flag.StringVar(&flAppend, "append", "", "Append blocks from this folder to the database")
	flag.BoolVar(&flTrunc, "trunc", false, "Truncate insted of splitting")
	flag.BoolVar(&flCommit, "commit", false, "Optimize the size of the data file")
	flag.BoolVar(&flVerify, "verify", false, "Verify each block inside the database")
	flag.StringVar(&flSaveBl, "savebl", "", "Save block with the given hash to disk")
	flag.BoolVar(&flPurgeAll, "purgeall", false, "Purge all blocks from the database")
	flag.UintVar(&flPurgeTo, "purgeto", 0, "Purge all blocks till (but excluding) the given height")

	flag.UintVar(&flFrom, "from", 0, "Set/clear flag from this block")
	flag.UintVar(&flTo, "to", 0xffffffff, "Set/clear flag to this block or merge/rename into this data file index")
	flag.IntVar(&flInvalid, "invalid", -1, "Set (1) or clear (0) Invalid flag")
	flag.IntVar(&flTrusted, "trusted", -1, "Set (1) or clear (0) Trusted flag")

	flag.BoolVar(&flFixLen, "fixlen", false, "Calculate (fix) orignial length of last 144 blocks")
	flag.BoolVar(&flFixLenAll, "fixlenall", false, "Calculate (fix) orignial length of each block")

	flag.UintVar(&flMergeDat, "mergedat", 0, "Merge this data file index into the data file specified by -to <idx>")
	flag.UintVar(&flMoveDat, "movedat", 0, "Rename this data file index into the data file specified by -to <idx>")

	flag.IntVar(&flSplitDat, "splitdat", -1, "Split this data file into smaller parts (-mb <mb>)")
	flag.UintVar(&flMB, "mb", 1000, "Split big data file into smaller parts of this size in MB (at least 8 MB)")

	flag.IntVar(&flDatIdx, "datidx", -1, "Show records with the specific data file index")

	flag.BoolVar(&flPurgeDatIdx, "purgedatidx", false, "Remove reerence to dat files which are not on disk")

	flag.Parse()

	if flHelp {
		flag.PrintDefaults()
		return
	}

	if flDir != "" && flDir[len(flDir)-1] != os.PathSeparator {
		flDir += string(os.PathSeparator)
	}

	if flAppend != "" {
		if flAppend[len(flAppend)-1] != os.PathSeparator {
			flAppend += string(os.PathSeparator)
		}
		fmt.Println("Loading", flAppend+"blockchain.new")
		dat, er := ioutil.ReadFile(flAppend + "blockchain.new")
		if er != nil {
			fmt.Println(er.Error())
			return
		}

		f, er := os.Open(flAppend + "blockchain.dat")
		if er != nil {
			fmt.Println(er.Error())
			return
		}

		fo, er := os.OpenFile(flDir+"blockchain.dat", os.O_WRONLY, 0600)
		if er != nil {
			f.Close()
			fmt.Println(er.Error())
			return
		}
		datfilelen, _ := fo.Seek(0, os.SEEK_END)

		fmt.Println("Appending blocks data to blockchain.dat")
		for {
			n, _ := f.Read(buf[:])
			if n > 0 {
				fo.Write(buf[:n])
			}
			if n != len(buf) {
				break
			}
		}
		fo.Close()
		f.Close()

		fmt.Println("Now appending", len(dat)/136, "records to blockchain.new")
		fo, er = os.OpenFile(flDir+"blockchain.new", os.O_WRONLY, 0600)
		if er != nil {
			f.Close()
			fmt.Println(er.Error())
			return
		}
		fo.Seek(0, os.SEEK_END)

		for off := 0; off < len(dat); off += 136 {
			sl := dat[off : off+136]
			newoffs := binary.LittleEndian.Uint64(sl[40:48]) + uint64(datfilelen)
			binary.LittleEndian.PutUint64(sl[40:48], newoffs)
			fo.Write(sl)
		}
		fo.Close()

		return
	}

	fmt.Println("Loading", flDir+"blockchain.new")
	dat, er := ioutil.ReadFile(flDir + "blockchain.new")
	if er != nil {
		fmt.Println(er.Error())
		return
	}

	fmt.Println(len(dat)/136, "records")

	if flMergeDat != 0 {
		if flTo >= flMergeDat {
			fmt.Println("To index must be lower than from index")
			return
		}
		minValidFrom, maxValidFrom := lookForRange(dat, uint32(flMergeDat))
		if minValidFrom == -1 {
			fmt.Println("Invalid from index")
			return
		}

		fromFn := datFilename(uint32(flMergeDat))
		toFn := datFilename(uint32(flTo))

		f, er := os.Open(fromFn)
		if er != nil {
			fmt.Println(er.Error())
			return
		}

		fo, er := os.OpenFile(toFn, os.O_WRONLY, 0600)
		if er != nil {
			f.Close()
			fmt.Println(er.Error())
			return
		}
		offsetToAdd, _ := fo.Seek(0, os.SEEK_END)

		fmt.Println("Appending", fromFn, "to", toFn, "at offset", offsetToAdd)
		for {
			n, _ := f.Read(buf[:])
			if n > 0 {
				fo.Write(buf[:n])
			}
			if n != len(buf) {
				break
			}
		}
		fo.Close()
		f.Close()

		var cnt int
		for off := minValidFrom; off <= maxValidFrom; off += 136 {
			sl := dat[off : off+136]
			fpos := binary.LittleEndian.Uint64(sl[40:48])
			fpos += uint64(offsetToAdd)
			binary.LittleEndian.PutUint64(sl[40:48], fpos)
			sl[0] |= 0x20
			binary.LittleEndian.PutUint32(sl[28:32], uint32(flTo))
			cnt++
		}
		ioutil.WriteFile("blockchain.tmp", dat, 0600)
		os.Rename("blockchain.tmp", "blockchain.new")
		os.Remove(fromFn)
		fmt.Println(fromFn, "removed and", cnt, "records updated in blockchain.new")
		return
	}

	if flMoveDat != 0 {
		if flTo == flMoveDat {
			fmt.Println("To index must be different than from index")
			return
		}
		minValid, maxValid := lookForRange(dat, uint32(flMoveDat))
		if minValid == -1 {
			fmt.Println("Invalid from index")
			return
		}
		toFn := datFilename(uint32(flTo))

		if fi, _ := os.Stat(toFn); fi != nil {
			fmt.Println(fi.Name(), "exist - get rid of it first")
			return
		}

		fromFn := datFilename(uint32(flMoveDat))

		// first discard all the records with the target index
		for off := 0; off < len(dat); off += 136 {
			rec := newSL(dat[off : off+136])
			if rec.DatIdx() == uint32(flTo) {
				rec.SetDLen(0)
				rec.SetDatIdx(0xffffffff)
			}
		}

		// now set the new index
		var cnt int
		for off := minValid; off <= maxValid; off += 136 {
			sl := dat[off : off+136]
			sl[0] |= 0x20
			binary.LittleEndian.PutUint32(sl[28:32], uint32(flTo))
			cnt++
		}
		ioutil.WriteFile("blockchain.tmp", dat, 0600)
		os.Rename(fromFn, toFn)
		os.Rename("blockchain.tmp", "blockchain.new")
		fmt.Println(fromFn, "renamed to ", toFn, "and", cnt, "records updated in blockchain.new")
		return
	}

	if flSplitDat >= 0 {
		if flMB < 8 {
			fmt.Println("Minimal value of -mb parameter is 8")
			return
		}
		fname := datFilename(uint32(flSplitDat))
		fmt.Println("Spliting file", fname, "into chunks - up to", flMB, "MB...")
		minValidOff, maxValidOff := lookForRange(dat, uint32(flSplitDat))
		f, er := os.OpenFile(fname, os.O_RDWR, 0600)
		if er != nil {
			fmt.Println(er.Error())
			return
		}
		defer f.Close()
		//fmt.Println("Range:", minValidOff/136, "...", maxValidOff/136)

		maxlen := uint64(flMB) << 20
		for off := minValidOff; off <= maxValidOff; off += 136 {
			rec := newSL(dat[off : off+136])
			if rec.DLen() == 0 {
				continue
			}
			dpos := rec.DPos()
			if dpos+uint64(rec.DLen()) > maxlen {
				//println("root split from", dpos)
				if !splitTheDataFile(f, uint32(flSplitDat)+1, maxlen, dat, off, maxValidOff) {
					fmt.Println("Splitting failed")
					return
				}
				f.Truncate(int64(dpos))
				fmt.Println("Splitting succeeded")
				return
			}
		}
		fmt.Println("There was nothing to split")
		return
	}

	if flDatIdx >= 0 {
		fname := datFilename(uint32(flDatIdx))
		minValidOff, maxValidOff := lookForRange(dat, uint32(flDatIdx))
		if minValidOff == -1 {
			fmt.Println(fname, "is not used by any record")
			return
		}
		fmt.Println(fname, "is used by", (maxValidOff-minValidOff)/136+1, "records. From", minValidOff/136, "to", maxValidOff/136)
		fmt.Println("Block height from", newSL(dat[minValidOff:]).Height(), "to", newSL(dat[maxValidOff:]).Height())
		return
	}

	if flPurgeDatIdx {
		cache := make(map[uint32]bool)
		var cnt int
		for off := 0; off < len(dat); off += 136 {
			rec := newSL(dat[off:])
			if rec.DLen() == 0 && rec.DatIdx() == 0xffffffff {
				continue
			}
			idx := rec.DatIdx()
			haveFile, ok := cache[idx]
			if !ok {
				fi, _ := os.Stat(datFilename(idx))
				haveFile = fi != nil
				cache[idx] = haveFile
			}
			if !haveFile {
				rec.SetDatIdx(0xffffffff)
				rec.SetDLen(0)
				cnt++
			}
		}
		if cnt > 0 {
			ioutil.WriteFile("blockchain.tmp", dat, 0600)
			os.Rename("blockchain.tmp", "blockchain.new")
			fmt.Println(cnt, "records removed from blockchain.new")
		} else {
			fmt.Println("Data files seem consisent - no need to remove anything")
		}
		return
	}

	if flInvalid == 0 || flInvalid == 1 || flTrusted == 0 || flTrusted == 1 {
		var cnt uint64
		for off := 0; off < len(dat); off += 136 {
			sl := dat[off : off+136]
			if uint(binary.LittleEndian.Uint32(sl[36:40])) < flFrom {
				continue
			}
			if uint(binary.LittleEndian.Uint32(sl[36:40])) > flTo {
				continue
			}
			if flInvalid == 0 {
				if (sl[0] & Invalid) != 0 {
					sl[0] &= ^byte(Invalid)
					cnt++
				}
			} else if flInvalid == 1 {
				if (sl[0] & Invalid) == 0 {
					sl[0] |= Invalid
					cnt++
				}
			}
			if flTrusted == 0 {
				if (sl[0] & Trusted) != 0 {
					sl[0] &= ^byte(Trusted)
					cnt++
				}
			} else if flTrusted == 1 {
				if (sl[0] & Trusted) == 0 {
					sl[0] |= Trusted
					cnt++
				}
			}
		}
		ioutil.WriteFile("blockchain.tmp", dat, 0600)
		os.Rename("blockchain.tmp", "blockchain.new")
		fmt.Println(cnt, "flags updated in blockchain.new")
	}

	if flPurgeAll {
		for off := 0; off < len(dat); off += 136 {
			sl := dat[off : off+136]
			binary.LittleEndian.PutUint64(sl[40:48], 0)
			binary.LittleEndian.PutUint32(sl[48:52], 0)
		}
		ioutil.WriteFile("blockchain.tmp", dat, 0600)
		os.Rename("blockchain.tmp", "blockchain.new")
		fmt.Println("blockchain.new upated. Now delete blockchain.dat yourself...")
	}

	if flPurgeTo != 0 {
		var curDatPos uint64

		f, er := os.Open("blockchain.dat")
		if er != nil {
			println(er.Error())
			return
		}
		defer f.Close()

		newdir := fmt.Sprint("purged_to_", flPurgeTo, string(os.PathSeparator))
		os.Mkdir(newdir, os.ModePerm)

		o, er := os.Create(newdir + "blockchain.dat")
		if er != nil {
			println(er.Error())
			return
		}
		defer o.Close()

		for off := 0; off < len(dat); off += 136 {
			sl := newSL(dat[off : off+136])

			if uint(sl.Height()) < flPurgeTo {
				sl.SetDLen(0)
				sl.SetDPos(0)
			} else {
				blen := int(sl.DLen())
				f.Seek(int64(sl.DPos()), os.SEEK_SET)
				er = btc.ReadAll(f, buf[:blen])
				if er != nil {
					println(er.Error())
					return
				}
				sl.SetDPos(curDatPos)
				curDatPos += uint64(blen)
				o.Write(buf[:blen])
			}
		}
		ioutil.WriteFile(newdir+"blockchain.new", dat, 0600)
		return
	}

	if flScan {
		var scanErrs uint
		lastBlHeight := binary.LittleEndian.Uint32(dat[36:40])
		expOffset := uint64(binary.LittleEndian.Uint32(dat[48:52]))
		fmt.Println("Scanning database for first extra block(s)...")
		fmt.Println("First block in the file has height", lastBlHeight)
		for off := 136; off < len(dat); off += 136 {
			sl := dat[off : off+136]
			height := binary.LittleEndian.Uint32(sl[36:40])
			offInBl := binary.LittleEndian.Uint64(sl[40:48])

			if height != lastBlHeight+1 {
				fmt.Println("Out of sequence block number", height, lastBlHeight+1, "found at offset", off)
				printRecord(dat[off-136 : off])
				printRecord(dat[off : off+136])
				fmt.Println()
				scanErrs++
			}
			if offInBl != expOffset {
				fmt.Println("Spare data found just before block number", height, offInBl, expOffset)
				printRecord(dat[off-136 : off])
				printRecord(dat[off : off+136])
				scanErrs++
			}

			if flStop != 0 && scanErrs >= flStop {
				break
			}

			lastBlHeight = height

			expOffset += uint64(binary.LittleEndian.Uint32(sl[48:52]))
		}
		return
	}

	if flDefrag {
		blks := make(map[[32]byte]*oneTreeNode, len(dat)/136)
		for off := 0; off < len(dat); off += 136 {
			sl := newSL(dat[off : off+136])
			blks[sl.HIdx()] = &oneTreeNode{off: off, oneIdxRec: sl}
		}
		var maxbl uint32
		var maxblptr *oneTreeNode
		for _, v := range blks {
			v.parent = blks[v.PIdx()]
			h := v.Height()
			if h > maxbl {
				maxbl = h
				maxblptr = v
			} else if h == maxbl {
				maxblptr = nil
			}
		}
		fmt.Println("Max block height =", maxbl)
		if maxblptr == nil {
			fmt.Println("More than one block at maximum height - cannot continue")
			return
		}
		used := make(map[[32]byte]bool)
		var firstBlock *oneTreeNode
		var totalDataSize uint64
		for n := maxblptr; n != nil; n = n.parent {
			if n.parent != nil {
				n.parent.next = n
			}
			used[n.PIdx()] = true
			if firstBlock == nil || firstBlock.Height() > n.Height() {
				firstBlock = n
			}
			totalDataSize += uint64(n.DLen())
		}
		if len(used) < len(blks) {
			fmt.Println("Purge", len(blks)-len(used), "blocks from the index file...")
			f, e := os.Create(flDir + "blockchain.tmp")
			if e != nil {
				println(e.Error())
				return
			}
			var off int
			for n := firstBlock; n != nil; n = n.next {
				n.off = off
				n.sl[0] = n.sl[0] & 0xfc
				f.Write(n.sl)
				off += len(n.sl)
			}
			f.Close()
			os.Rename(flDir+"blockchain.tmp", flDir+"blockchain.new")
		} else {
			fmt.Println("The index file looks perfect")
		}

		for n := firstBlock; n != nil && n.next != nil; n = n.next {
			if n.next.DPos() < n.DPos() {
				fmt.Println("There is a problem... swapped order in the data file!", n.off)
				return
			}
		}

		fdat, er := os.OpenFile(flDir+"blockchain.dat", os.O_RDWR, 0600)
		if er != nil {
			println(er.Error())
			return
		}

		if fl, _ := fdat.Seek(0, os.SEEK_END); uint64(fl) == totalDataSize {
			fdat.Close()
			fmt.Println("All good - blockchain.dat has an optimal length")
			return
		}

		if !flCommit {
			fdat.Close()
			fmt.Println("Warning: blockchain.dat shall be defragmented. Use \"-defrag -commit\"")
			return
		}

		fidx, er := os.OpenFile(flDir+"blockchain.new", os.O_RDWR, 0600)
		if er != nil {
			println(er.Error())
			return
		}

		// Capture Ctrl+C
		killchan := make(chan os.Signal, 1)
		signal.Notify(killchan, os.Interrupt, os.Kill)

		var doff uint64
		var prvPerc uint64 = 101
		for n := firstBlock; n != nil; n = n.next {
			perc := 1000 * doff / totalDataSize
			dp := n.DPos()
			dl := n.DLen()
			if perc != prvPerc {
				fmt.Printf("\rDefragmenting data file - %.1f%% (%d bytes saved so far)...",
					float64(perc)/10.0, dp-doff)
				prvPerc = perc
			}
			if dp > doff {
				fdat.Seek(int64(dp), os.SEEK_SET)
				fdat.Read(buf[:int(dl)])

				n.SetDPos(doff)

				fdat.Seek(int64(doff), os.SEEK_SET)
				fdat.Write(buf[:int(dl)])

				fidx.Seek(int64(n.off), os.SEEK_SET)
				fidx.Write(n.sl)
			}
			doff += uint64(dl)

			select {
			case <-killchan:
				fmt.Println("interrupted")
				fidx.Close()
				fdat.Close()
				fmt.Println("Database closed - should be still usable, but no space saved")
				return
			default:
			}
		}

		fidx.Close()
		fdat.Close()
		fmt.Println()

		fmt.Println("Truncating blockchain.dat at position", doff)
		os.Truncate(flDir+"blockchain.dat", int64(doff))

		return
	}

	if flVerify {
		var prvPerc uint64 = 0xffffffffff
		var totlen uint64
		var done sync.WaitGroup
		var datFileOpen uint32 = 0xffffffff
		var fdat *os.File
		var cnt, cntND, cntErr int
		var curProgress uint64

		totalDataSize := calcTotalSize(dat)

		for off := 0; off < len(dat); off += 136 {
			sl := newSL(dat[off : off+136])

			le := int(sl.DLen())
			if le == 0 {
				continue
			}
			curProgress += uint64(sl.DLen())

			hei := uint(sl.Height())

			if hei < flFrom {
				continue
			}

			idx := sl.DatIdx()
			if idx == 0xffffffff {
				continue
			}

			if idx != datFileOpen {
				var er error
				datFileOpen = idx
				if fdat != nil {
					fdat.Close()
				}
				fdat, er = os.OpenFile(flDir+datFilename(idx), os.O_RDWR, 0600)
				if er != nil {
					//println(er.Error())
					continue
				}
			}

			perc := 1000 * curProgress / totalDataSize
			if perc != prvPerc {
				fmt.Printf("\rVerifying blocks data - %.1f%% @ %d / %dMB processed...",
					float64(perc)/10.0, hei, totlen>>20)
				prvPerc = perc
			}

			if flBlock != 0 && hei < flBlock {
				continue
			}

			dp := int64(sl.DPos())
			fdat.Seek(dp, os.SEEK_SET)
			n, _ := fdat.Read(buf[:le])
			if n != le {
				//fmt.Println("Block", hei, "not in dat file", idx, dp)
				cntND++
				continue
			}

			blk := decompBlock(sl.Flags(), buf[:le])
			if blk == nil {
				fmt.Println("Block", hei, "decompression failed")
				cntErr++
				continue
			}

			done.Add(1)
			go func(blk []byte, sl oneIdxRec, off int) {
				verifyBlock(blk, sl, off)
				done.Done()
				cnt++
			}(blk, sl, off)

			totlen += uint64(len(blk))
		}
		done.Wait() // wait for all the goroutines to complete
		fdat.Close()
		if fdat != nil {
			fdat.Close()
		}
		fmt.Println("\nAll blocks done -", totlen>>20, "MB and", cnt, "blocks verified OK")
		fmt.Println("No data errors:", cntND, "  Decompression errors:", cntErr)
		return
	}

	if flBlock != 0 {
		for off := 0; off < len(dat); off += 136 {
			sl := dat[off : off+136]
			height := binary.LittleEndian.Uint32(sl[36:40])
			if uint(height) == flBlock {
				printRecord(dat[off : off+136])
			}
		}
		return
	}

	if flSplit != "" {
		th := btc.NewUint256FromString(flSplit)
		if th == nil {
			println("incorrect block hash")
			return
		}
		for off := 0; off < len(dat); off += 136 {
			sl := dat[off : off+136]
			height := binary.LittleEndian.Uint32(sl[36:40])
			bh := btc.NewSha2Hash(sl[56:136])
			if bh.Hash == th.Hash {
				truncIdxOffs := int64(off)
				truncDatOffs := int64(binary.LittleEndian.Uint64(sl[40:48]))
				fmt.Println("Truncate blockchain.new at offset", truncIdxOffs)
				fmt.Println("Truncate blockchain.dat at offset", truncDatOffs)
				if !flTrunc {
					newDir := flDir + fmt.Sprint(height) + string(os.PathSeparator)
					os.Mkdir(newDir, os.ModePerm)

					f, e := os.Open(flDir + "blockchain.dat")
					if e != nil {
						fmt.Println(e.Error())
						return
					}
					df, e := os.Create(newDir + "blockchain.dat")
					if e != nil {
						f.Close()
						fmt.Println(e.Error())
						return
					}

					f.Seek(truncDatOffs, os.SEEK_SET)

					fmt.Println("But fist save the rest in", newDir, "...")
					if flSkip != 0 {
						fmt.Println("Skip", flSkip, "blocks in the output file")
						for flSkip > 0 {
							skipbytes := binary.LittleEndian.Uint32(sl[48:52])
							fmt.Println(" -", skipbytes, "bytes of block", binary.LittleEndian.Uint32(sl[36:40]))
							off += 136
							if off < len(dat) {
								sl = dat[off : off+136]
								flSkip--
							} else {
								break
							}
						}
					}

					for {
						n, _ := f.Read(buf[:])
						if n > 0 {
							df.Write(buf[:n])
						}
						if n != len(buf) {
							break
						}
					}
					df.Close()
					f.Close()

					df, e = os.Create(newDir + "blockchain.new")
					if e != nil {
						f.Close()
						fmt.Println(e.Error())
						return
					}
					var off2 int
					for off2 = off; off2 < len(dat); off2 += 136 {
						sl := dat[off2 : off2+136]
						newoffs := binary.LittleEndian.Uint64(sl[40:48]) - uint64(truncDatOffs)
						binary.LittleEndian.PutUint64(sl[40:48], newoffs)
						df.Write(sl)
					}
					df.Close()
				}

				os.Truncate(flDir+"blockchain.new", truncIdxOffs)
				os.Truncate(flDir+"blockchain.dat", truncDatOffs)
				return
			}
		}
		fmt.Println("Block not found - nothing truncated")
	}

	if flSaveBl != "" {
		bh := btc.NewUint256FromString(flSaveBl)
		if bh == nil {
			println("Incortrect block hash:", flSaveBl)
			return
		}
		for off := 0; off < len(dat); off += 136 {
			sl := newSL(dat[off : off+136])
			if bytes.Equal(sl.Hash(), bh.Hash[:]) {
				f, er := os.Open(flDir + datFilename(sl.DatIdx()))
				if er != nil {
					println(er.Error())
					return
				}
				bu := buf[:int(sl.DLen())]
				f.Seek(int64(sl.DPos()), os.SEEK_SET)
				f.Read(bu)
				f.Close()
				ioutil.WriteFile(bh.String()+".bin", decompBlock(sl.Flags(), bu), 0600)
				fmt.Println(bh.String()+".bin written to disk. It has height", sl.Height())
				return
			}
		}
		fmt.Println("Block", bh.String(), "not found in the database")
		return
	}

	if flFixLen || flFixLenAll {
		fdat, er := os.OpenFile(flDir+"blockchain.dat", os.O_RDWR, 0600)
		if er != nil {
			println(er.Error())
			return
		}

		datFileSize, _ := fdat.Seek(0, os.SEEK_END)

		var prvPerc int64 = -1
		var totlen uint64
		var off int
		if !flFixLenAll {
			off = len(dat) - 144*136
		}
		for ; off < len(dat); off += 136 {
			sl := newSL(dat[off : off+136])
			olen := binary.LittleEndian.Uint32(sl.sl[32:36])
			if olen == 0 {
				sl := newSL(dat[off : off+136])
				dp := int64(sl.DPos())
				le := int(sl.DLen())

				perc := 1000 * dp / datFileSize
				if perc != prvPerc {
					fmt.Printf("\rUpdating blocks length - %.1f%% / %dMB processed...",
						float64(perc)/10.0, totlen>>20)
					prvPerc = perc
				}

				fdat.Seek(dp, os.SEEK_SET)
				fdat.Read(buf[:le])
				blk := decompBlock(sl.Flags(), buf[:le])
				binary.LittleEndian.PutUint32(sl.sl[32:36], uint32(len(blk)))
				sl.sl[0] |= 0x10

				totlen += uint64(len(blk))
			}
		}
		ioutil.WriteFile("blockchain.tmp", dat, 0600)
		os.Rename("blockchain.tmp", "blockchain.new")
		fmt.Println("blockchain.new updated")
	}

	var minbh, maxbh, valididx, validlen uint32
	minbh = binary.LittleEndian.Uint32(dat[36:40])
	maxbh = minbh
	for off := 136; off < len(dat); off += 136 {
		sl := newSL(dat[off : off+136])
		bh := sl.Height()
		if bh > maxbh {
			maxbh = bh
		} else if bh < minbh {
			minbh = bh
		}
		if sl.DatIdx() != 0xffffffff {
			valididx++
		}
		if sl.DLen() != 0 {
			validlen++
		}
	}
	fmt.Println("Block heights from", minbh, "to", maxbh)
	fmt.Println("Number of records with valid length:", validlen)
	fmt.Println("Number of records with valid data file:", valididx)
}
