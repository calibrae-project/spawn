package main

import (
	"bufio"
	"os"
	"strings"

	"github.com/ParallelCoinTeam/duod/lib/btc"
)

// Resolved while parsing "-send" parameter
type oneSendTo struct {
	addr   *btc.Addr
	amount uint64
}

var (
	// set in parseSpend():
	spendBtc, feeBtc, changeBtc uint64
	sendTo                      []oneSendTo
)

// parse the "-send ..." parameter
func parseSpend() {
	outs := strings.Split(*send, ",")

	for i := range outs {
		tmp := strings.Split(strings.Trim(outs[i], " "), "=")
		if len(tmp) != 2 {
			println("The outputs must be in a format address1=amount1[,addressN=amountN]")
			cleanExit(1)
		}

		a, e := btc.NewAddrFromString(tmp[0])
		if e != nil {
			println("NewAddrFromString:", e.Error())
			cleanExit(1)
		}
		assertAddressVersion(a)

		am, er := btc.StringToSatoshis(tmp[1])
		if er != nil {
			println("Incorrect amount: ", tmp[1], er.Error())
			cleanExit(1)
		}
		if *subfee && i == 0 {
			am -= curFee
		}

		sendTo = append(sendTo, oneSendTo{addr: a, amount: am})
		spendBtc += am
	}
}

// parse the "-batch ..." parameter
func parseBatch() {
	f, e := os.Open(*batch)
	if e == nil {
		defer f.Close()
		td := bufio.NewReader(f)
		var lcnt int
		for {
			li, _, _ := td.ReadLine()
			if li == nil {
				break
			}
			lcnt++
			tmp := strings.SplitN(strings.Trim(string(li), " "), "=", 2)
			if len(tmp) < 2 {
				println("Error in the batch file line", lcnt)
				cleanExit(1)
			}
			if tmp[0][0] == '#' {
				continue // Just a comment-line
			}

			a, e := btc.NewAddrFromString(tmp[0])
			if e != nil {
				println("NewAddrFromString:", e.Error())
				cleanExit(1)
			}
			assertAddressVersion(a)

			am, e := btc.StringToSatoshis(tmp[1])
			if e != nil {
				println("StringToSatoshis:", e.Error())
				cleanExit(1)
			}

			sendTo = append(sendTo, oneSendTo{addr: a, amount: am})
			spendBtc += am
		}
	} else {
		println(e.Error())
		cleanExit(1)
	}
}

// returns true if spend operation has been requested
func sendRequest() bool {
	feeBtc = curFee
	if *send != "" {
		parseSpend()
	}
	if *batch != "" {
		parseBatch()
	}
	return len(sendTo) > 0
}
