package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ParallelCoinTeam/duod"
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/others/sys"
)

var (
	// PassSeedFilename -
	PassSeedFilename = ".secret"
	// RawKeysFilename -
	RawKeysFilename = ".others"
)

var (
	// Command line switches

	// Wallet options
	list      = flag.Bool("l", false, "List public addressses from the wallet")
	singleask = flag.Bool("1", false, "Do not re-ask for the password (when used along with -l)")
	noverify  = flag.Bool("q", false, "Do not verify keys while listing them")
	verbose   = flag.Bool("v", false, "Verbose version (print more info)")
	ask4pass  = flag.Bool("p", false, "Force the wallet to ask for seed password")
	nosseed   = flag.Bool("is", false, "Ignore seed from the config file")
	subfee    = flag.Bool("f", false, "Substract fee from the first value")

	dumppriv = flag.String("dump", "", "Export a private key of a given address (use * for all)")

	// Spending money options
	send   = flag.String("send", "", "Send money to list of comma separated pairs: address=amount")
	batch  = flag.String("batch", "", "Send money as per the given batch file (each line: address=amount)")
	change = flag.String("change", "", "Send any change to this address (otherwise return to 1st input)")

	// Message signing options
	signaddr = flag.String("sign", "", "Request a sign operation with a given bitcoin address")
	message  = flag.String("msg", "", "Message to be signed or included into transaction")

	useallinputs = flag.Bool("useallinputs", false, "Use all the unspent outputs as the transaction inputs")

	// Sign raw TX
	rawtx = flag.String("raw", "", "Sign a raw transaction (use hex-encoded string)")

	// Decode raw tx
	dumptxfn = flag.String("d", "", "Decode raw transaction from the specified file")

	// Sign raw message
	signhash = flag.String("hash", "", "Sign a raw hash (use together with -sign parameter)")

	// Print a public key of a give bitcoin address
	pubkey = flag.String("pub", "", "Print public key of the give bitcoin address")

	// Print a public key of a give bitcoin address
	p2sh             = flag.String("p2sh", "", "Insert P2SH script into each transaction input (use together with -raw)")
	input            = flag.Int("input", -1, "Insert P2SH script only at this intput number (-1 for all inputs)")
	multisign        = flag.String("msign", "", "Sign multisig transaction with given bitcoin address (use with -raw)")
	allowextramsigns = flag.Bool("xtramsigs", false, "Allow to put more signatures than needed (for multisig txs)")

	sequence = flag.Int("seq", 0, "Use given RBF sequence number (-1 or -2 for final)")

	segwitMode = flag.Bool("segwit", false, "List SegWit deposit addresses (instead of P2KH)")
	bech32Mode = flag.Bool("bech32", false, "use with -segwit to see P2WPKH deposit addresses (instead of P2SH-WPKH)")
)

// exit after cleaning up private data from memory
func cleanExit(code int) {
	if *verbose {
		fmt.Println("Cleaning up private keys")
	}
	for k := range keys {
		sys.ClearBuffer(keys[k].Key)
	}
	if type2Secret != nil {
		sys.ClearBuffer(type2Secret)
	}
	os.Exit(code)
}

func main() {
	// Print the logo to stderr
	println("Duod Wallet version", Duod.Version)
	println("This program comes with ABSOLUTELY NO WARRANTY")
	println()

	parseConfig()
	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}

	flag.Parse()

	if uncompressed {
		println("For SegWit address safety, uncompressed keys are disabled in this version")
		os.Exit(1)
	}

	// convert string fee to uint64
	if val, e := btc.StringToSatoshis(fee); e != nil {
		println("Incorrect fee value", fee)
		os.Exit(1)
	} else {
		curFee = val
	}

	// decode raw transaction?
	if *dumptxfn != "" {
		dumpRawTx()
		return
	}

	// dump public key or secret scan key?
	if *pubkey != "" {
		makeWallet()
		cleanExit(0)
	}

	// list public addresses?
	if *list {
		makeWallet()
		dumpAddrs()
		cleanExit(0)
	}

	// dump privete key?
	if *dumppriv != "" {
		makeWallet()
		dumpPrivKey()
		cleanExit(0)
	}

	// sign a message or a hash?
	if *signaddr != "" {
		makeWallet()
		signMessage()
		if *send == "" {
			// Don't loadBalance if he did not want to spend coins as well
			cleanExit(0)
		}
	}

	// raw transaction?
	if *rawtx != "" {
		// add p2sh sript to it?
		if *p2sh != "" {
			makeP2sh()
			cleanExit(0)
		}

		makeWallet()

		// multisig sign with a specific key?
		if *multisign != "" {
			multisigSign()
			cleanExit(0)
		}

		// this must be signing of a raw trasnaction
		loadBalance()
		processRawTx()
		cleanExit(0)
	}

	// make the wallet nad print balance
	makeWallet()
	if e := loadBalance(); e != nil {
		fmt.Println("ERROR:", e.Error())
		cleanExit(1)
	}

	// send command?
	if sendRequest() {
		makeSignedTx()
		cleanExit(0)
	}

	showBalance()
	cleanExit(0)
}
