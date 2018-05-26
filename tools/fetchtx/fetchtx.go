package main

import (
	"fmt"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"io/ioutil"
	"github.com/ParallelCoinTeam/duod/lib/others/utils"
	"github.com/ParallelCoinTeam/duod"
	"os"
)


func main() {
	fmt.Println("Duod FetchTx version", Duod.Version)

	if len(os.Args) < 2 {
		fmt.Println("Specify transaction id on the command line (MSB).")
		return
	}

	txid := btc.NewUint256FromString(os.Args[1])
	rawtx := utils.GetTxFromWeb(txid)
	if rawtx==nil {
		fmt.Println("Error fetching the transaction")
	} else {
		ioutil.WriteFile(txid.String()+".tx", rawtx, 0666)
	}
}
