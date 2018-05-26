package wallet

import (
	"bytes"
	"encoding/gob"
	"io/ioutil"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/utxo"
)

var (
	// FetchingBalanceTick -
	FetchingBalanceTick func() bool
	// OnOff -
	OnOff = make(chan bool, 1)
)

// InitMaps -
func InitMaps(empty bool) {
	var szs [4]int
	var ok bool

	if empty {
		goto init
	}

	LoadMapSizes()
	szs, ok = WalletAddrsCount[common.AllBalMinVal()]
	if ok {
		//fmt.Println("Have map sizes for MinBal", common.AllBalMinVal(), ":", szs[0], szs[1], szs[2], szs[3])
	} else {
		//fmt.Println("No map sizes for MinBal", common.AllBalMinVal())
		szs = [4]int{10e6, 3e6, 10e3, 1e3} // defaults
	}

init:
	AllBalancesP2KH = make(map[[20]byte]*OneAllAddrBal, szs[0])
	AllBalancesP2SH = make(map[[20]byte]*OneAllAddrBal, szs[1])
	AllBalancesP2WKH = make(map[[20]byte]*OneAllAddrBal, szs[2])
	AllBalancesP2WSH = make(map[[32]byte]*OneAllAddrBal, szs[3])
}

// LoadBalance -
func LoadBalance() {
	if common.GetBool(&common.WalletON) {
		//fmt.Println("wallet.LoadBalance() ignore: ", common.GetBool(&common.WalletON))
		return
	}

	var aborted bool

	common.SetUint32(&common.WalletProgress, 1)
	common.ApplyBalMinVal()

	InitMaps(false)

	common.BlockChain.Unspent.RWMutex.RLock()
	defer common.BlockChain.Unspent.RWMutex.RUnlock()

	countDownFrom := (len(common.BlockChain.Unspent.HashMap) + 999) / 1000
	countDown := countDownFrom
	perc := uint32(1)

	for k, v := range common.BlockChain.Unspent.HashMap {
		NewUTXO(utxo.NewUtxoRecStatic(k, v))
		if countDown == 0 {
			perc++
			common.SetUint32(&common.WalletProgress, perc)
			countDown = countDownFrom
		} else {
			countDown--
		}
		if FetchingBalanceTick != nil && FetchingBalanceTick() {
			aborted = true
			break
		}
	}
	if aborted {
		InitMaps(true)
	} else {
		common.BlockChain.Unspent.CB.NotifyTxAdd = TxNotifyAdd
		common.BlockChain.Unspent.CB.NotifyTxDel = TxNotifyDel
		common.SetBool(&common.WalletON, true)
	}
	common.SetUint32(&common.WalletProgress, 0)
}

// Disable -
func Disable() {
	if !common.GetBool(&common.WalletON) {
		//fmt.Println("wallet.Disable() ignore: ", common.GetBool(&common.WalletON))
		return
	}
	UpdateMapSizes()
	common.BlockChain.Unspent.CB.NotifyTxAdd = nil
	common.BlockChain.Unspent.CB.NotifyTxDel = nil
	common.SetBool(&common.WalletON, false)
	InitMaps(true)
}

const (
	// MapSizeFileName -
	MapSizeFileName = "mapsize.gob"
)

var (
	// WalletAddrsCount -
	WalletAddrsCount = make(map[uint64][4]int) //index:MinValue, [0]-P2KH, [1]-P2SH, [2]-P2WSH, [3]-P2WKH
)

// UpdateMapSizes -
func UpdateMapSizes() {
	WalletAddrsCount[common.AllBalMinVal()] = [4]int{len(AllBalancesP2KH),
		len(AllBalancesP2SH), len(AllBalancesP2WKH), len(AllBalancesP2WSH)}

	buf := new(bytes.Buffer)
	gob.NewEncoder(buf).Encode(WalletAddrsCount)
	ioutil.WriteFile(common.SpawnHomeDir+MapSizeFileName, buf.Bytes(), 0600)
}

// LoadMapSizes -
func LoadMapSizes() {
	d, er := ioutil.ReadFile(common.SpawnHomeDir + MapSizeFileName)
	if er != nil {
		println("LoadMapSizes:", er.Error())
		return
	}

	buf := bytes.NewBuffer(d)

	er = gob.NewDecoder(buf).Decode(&WalletAddrsCount)
	if er != nil {
		println("LoadMapSizes:", er.Error())
	}
}
