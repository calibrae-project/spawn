package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/calibrae-project/spawn/lib/btc"
	"github.com/calibrae-project/spawn/lib/others/qdb"
)

var (
	blockHeight uint64
	blockHash   []byte
)

func loadMap4() (ndb map[qdb.KeyType][]byte) {
	var odb *qdb.DB
	ndb = make(map[qdb.KeyType][]byte, 21e6)
	for i := 0; i < 16; i++ {
		fmt.Print("\r", i, " of 16 ... ")
		er := qdb.NewDBExt(&odb, &qdb.NewDBOpts{Dir: fmt.Sprintf("unspent4/%06d", i),
			Volatile: true, LoadData: true, WalkFunction: func(key qdb.KeyType, val []byte) uint32 {
				if _, ok := ndb[key]; ok {
					panic("duplicate")
				}
				ndb[key] = val
				return 0
			}})
		if er != nil {
			fmt.Println(er.Error())
			return
		}
		odb.Close()
	}
	fmt.Print("\r                                                              \r")
	return
}

func loadLastBlock() {
	var maxBlFn string

	fis, _ := ioutil.ReadDir("unspent4/")
	var maxbl, undobl int
	for _, fi := range fis {
		if !fi.IsDir() && fi.Size() >= 32 {
			ss := strings.SplitN(fi.Name(), ".", 2)
			cb, er := strconv.ParseUint(ss[0], 10, 32)
			if er == nil && int(cb) > maxbl {
				maxbl = int(cb)
				maxBlFn = fi.Name()
				if len(ss) == 2 && ss[1] == "tmp" {
					undobl = maxbl
				}
			}
		}
	}
	if maxbl == 0 {
		fmt.Println("This unspent4 database is corrupt")
		return
	}
	if undobl == maxbl {
		fmt.Println("This unspent4 database is not properly closed")
		return
	}

	blockHeight = uint64(maxbl)
	blockHash = make([]byte, 32)

	f, _ := os.Open("unspent4/" + maxBlFn)
	f.Read(blockHash)
	f.Close()

}

func save_map(ndb map[qdb.KeyType][]byte) {
	var countDown, countDownFrom, perc int
	of, er := os.Create("UTXO.db")
	if er != nil {
		fmt.Println("Create file:", er.Error())
		return
	}

	countDownFrom = len(ndb) / 100
	wr := bufio.NewWriter(of)
	binary.Write(wr, binary.LittleEndian, uint64(blockHeight))
	wr.Write(blockHash)
	binary.Write(wr, binary.LittleEndian, uint64(len(ndb)))
	for k, v := range ndb {
		btc.WriteVlen(wr, uint64(len(v)+8))
		binary.Write(wr, binary.LittleEndian, k)
		//binary.Write(wr, binary.LittleEndian, uint32(len(v)))
		_, er = wr.Write(v)
		if er != nil {
			fmt.Println("\n\007Fatal error:", er.Error())
			break
		}
		if countDown == 0 {
			fmt.Print("\rSaving UTXO.db - ", perc, "% complete ... ")
			countDown = countDownFrom
			perc++
		} else {
			countDown--
		}
	}
	wr.Flush()
	of.Close()

	fmt.Print("\r                                                              \r")
}

func main() {
	var sta time.Time

	if fi, er := os.Stat("unspent4"); er != nil || !fi.IsDir() {
		fmt.Println("ERROR: Input database not found.")
		fmt.Println("Make sure to have unspent4/ directory, where you run this tool from")
		return
	}

	loadLastBlock()
	if len(blockHash) != 32 {
		fmt.Println("ERROR: Could not recover last block's data from the input database", len(blockHash))
		return
	}

	fmt.Println("Loading input database. Block", blockHeight, btc.NewUint256(blockHash).String())
	sta = time.Now()
	ndb := loadMap4()
	fmt.Println(len(ndb), "records loaded in", time.Now().Sub(sta).String())

	sta = time.Now()
	save_map(ndb)
	fmt.Println("Saved in in", time.Now().Sub(sta).String())
}
