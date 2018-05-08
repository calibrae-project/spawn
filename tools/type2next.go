// This tool outpus Type-2 deterministic addresses, as described here:
// https://bitcointalk.org/index.php?topic=19137.0
// At input it takes "A_publicKey" and "B_secret" - both values as hex encoded strings.
// Optionally, you can add a third parameter - number of public keys you want to calculate.
package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/calibrae-project/spawn/lib/btc"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Specify secret and publicKey to get the next Type-2 deterministic address")
		fmt.Println("Add -t as the third argument to work with Testnet addresses.")
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

	testnet := len(os.Args) > 3 && os.Args[3] == "-t"

	// Old address
	publicKey = btc.DeriveNextPublic(publicKey, secret)

	// New address
	fmt.Println(btc.NewAddrFromPubkey(publicKey, btc.AddrVerPubkey(testnet)).String())
	// New key
	fmt.Println(hex.EncodeToString(publicKey))

}
