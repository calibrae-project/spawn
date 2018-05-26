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
	fmt.Println("Spawn FetchBlock version", Spawn.Version)

	if len(os.Args) < 2 {
		fmt.Println("Specify block hash on the command line (MSB).")
		return
	}

	hash := btc.NewUint256FromString(os.Args[1])
	bl := utils.GetBlockFromWeb(hash)
	if bl==nil {
		fmt.Println("Error fetching the block")
	} else {
		ioutil.WriteFile(bl.Hash.String()+".bin", bl.Raw, 0666)
	}
}
