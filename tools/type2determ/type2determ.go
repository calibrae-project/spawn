// This tool outpus Type-2 deterministic addresses, as described here:
// https://bitcointalk.org/index.php?topic=19137.0
// At input it takes "A_publicKey" and "secret" - both values as hex encoded strings.
// Optionally, you can add a third parameter - number of public keys you want to calculate.
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"

	"github.com/ParallelCoinTeam/duod/lib/btc"
)

func main() {
	var testnet bool

	if len(os.Args) < 3 {
		fmt.Println("Specify secret, publicKey and optionaly number of addresses you want.")
		fmt.Println("Use a negative value for number of addresses, to work with Testnet addresses.")
		return
	}
	publicKey, er := hex.DecodeString(os.Args[2])
	if er != nil {
		println("Error parsing publicKey:", er.Error())
		os.Exit(1)
	}

	if len(publicKey) == 33 && (publicKey[0] == 2 || publicKey[0] == 3) {
		fmt.Println("Compressed")
	} else if len(publicKey) == 65 && (publicKey[0] == 4) {
		fmt.Println("Uncompressed")
	} else {
		println("Incorrect public key")
	}

	secret, er := hex.DecodeString(os.Args[1])
	if er != nil {
		println("Error parsing secret:", er.Error())
		os.Exit(1)
	}

	n := int64(25)

	if len(os.Args) > 3 {
		n, er = strconv.ParseInt(os.Args[3], 10, 32)
		if er != nil {
			println("Error parsing number of keys value:", er.Error())
			os.Exit(1)
		}
		if n == 0 {
			return
		}

		if n < 0 {
			n = -n
			testnet = true
		}
	}

	fmt.Println("# Type-2")
	fmt.Println("#", hex.EncodeToString(publicKey))
	fmt.Println("#", hex.EncodeToString(secret))

	for i := 1; i <= int(n); i++ {
		fmt.Println(btc.NewAddrFromPubkey(publicKey, btc.AddrVerPubkey(testnet)).String(), "TypB", i)
		if i >= int(n) {
			break
		}

		publicKey = btc.DeriveNextPublic(publicKey, secret)
	}
}
