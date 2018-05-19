// Bitcoin fork genesis block generator, based on https://bitcointalk.org/index.php?topic=181981.0 hosted at https://pastebin.com/nhuuV7y9
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"crypto/sha256"
	"time"
	// "github.com/nsf/termbox-go"
)

type transaction struct {
	merkleHash     []byte // 32 bytes long
	serializedData []byte
	version        uint32
	numInputs      uint8
	prevOutput     []byte // 32 bytes long
	prevoutIndex   uint32
	scriptSig      []byte
	sequence       uint32
	numOutputs     uint8
	outValue       uint64
	pubkeyScript   []byte
	locktime       uint32
}

const coin uint64 = 10000000

var (
	op_checksig   uint8 = 172
	startNonce    uint32
	unixtime      uint32
	maxNonce      = ^uint32(0)
)

// This function reverses the bytes in a byte array
func byteswap(buf []byte) {
	length := len(buf)
	for i := 0; i < length/2; i++ {
		buf[i], buf[length-i-1] = buf[length-i-1], buf[i]
	}
}

func initTransaction() (t transaction) {
	t.version = 1
	t.numInputs = 1
	t.numOutputs = 1
	t.locktime = 0
	t.prevoutIndex = 0xffffffff
	t.sequence = 0xfffffff
	t.outValue = coin
	t.prevOutput = make([]byte, 32, 32)
	return
}

func main() {
	args := os.Args
	if len(args) != 4 {
		fmt.Println("Bitcoin fork genesis block generator")
		fmt.Println("Usage:")
		fmt.Println("    ", args[0], "<pubkey> <timestamp> <nBits>")
		fmt.Println("Example:")
		fmt.Println("    ", args[0], "04678afdb0fe5548271967f1a67130b7105cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef38c4f35504e51ec112de5c384df7ba0b8d578a4c702b6bf11d5f \"The Times 03/Jan/2009 Chancellor on brink of second bailout for banks\" 486604799")
		fmt.Println("\nIf you execute this without parameters the above example will instead be processed")
		args = []string{
			os.Args[0],
			"04678afdb0fe5548271967f1a67130b7105cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef38c4f35504e51ec112de5c384df7ba0b8d578a4c702b6bf11d5f",
			"The Times 03/Jan/2009 Chancellor on brink of second bailout for banks",
			"486604799",
		}
	}
	if len(args[1]) != 130 {
		fmt.Println("Invalid public key length. Should be 130 hex digits,")
		os.Exit(1)
	}
	pubkey, err := hex.DecodeString(args[1])
	if err != nil {
		fmt.Println("Public key had invalid characters")
	}
	timestamp := args[2]
	if len(timestamp) > 254 || len(timestamp) < 1 {
		fmt.Println("Timestamp was either longer than 254 characters or zero length")
		os.Exit(1)
	}
	tx := initTransaction()
	nbits, err := strconv.ParseInt(args[3], 10, 64)
	if err != nil {
		fmt.Println("nBits was not a decimal number or exceeded the precision of 32 bits")
		os.Exit(0)
	}
	nBits := uint32(nbits)
	tx.pubkeyScript = append([]byte{0x41}, pubkey...)
	tx.pubkeyScript = append(tx.pubkeyScript, op_checksig)
	switch {
	case nBits <= 255:
		tx.scriptSig = append([]byte{1}, byte(nBits))
	case nBits <= 65535:
		tx.scriptSig = append([]byte{2}, byte(nBits))
		tx.scriptSig = append(tx.scriptSig, byte(nBits>>8))
	case nBits <= 16777215:
		tx.scriptSig = append([]byte{3}, byte(nBits))
		for i := uint(1); i < 3; i++ {
			tx.scriptSig = append(tx.scriptSig, byte(nBits>>(8*i)))
		}
	default:
		tx.scriptSig = append([]byte{4}, byte(nBits))
		for i := uint(1); i < 4; i++ {
			tx.scriptSig = append(tx.scriptSig, byte(nBits>>(8*i)))
		}
	}
	tx.scriptSig = append(tx.scriptSig, 0x01)
	tx.scriptSig = append(tx.scriptSig, 0x04)
	tx.scriptSig = append(tx.scriptSig, byte(len(timestamp)))
	tx.scriptSig = append(tx.scriptSig, []byte(timestamp)...)
	tx.serializedData = append(tx.serializedData, uint32tobytes(tx.version)...)
	tx.serializedData = append(tx.serializedData, tx.numInputs)
	tx.serializedData = append(tx.serializedData, tx.prevOutput...)
	tx.serializedData = append(tx.serializedData, uint32tobytes(tx.prevoutIndex)...)
	tx.serializedData = append(tx.serializedData, byte(len(tx.scriptSig)))
	tx.serializedData = append(tx.serializedData, tx.scriptSig...)
	tx.serializedData = append(tx.serializedData, uint32tobytes(tx.sequence)...)
	tx.serializedData = append(tx.serializedData, tx.numOutputs)
	tx.serializedData = append(tx.serializedData, uint64tobytes(tx.outValue)...)
	tx.serializedData = append(tx.serializedData, byte(len(tx.pubkeyScript)))
	tx.serializedData = append(tx.serializedData, tx.pubkeyScript...)
	tx.serializedData = append(tx.serializedData, uint32tobytes(tx.locktime)...)
	hash1 := sha256.Sum256(tx.serializedData)
	hash2 := sha256.Sum256(hash1[:])
	tx.merkleHash = hash2[:]
	merkleHash := hex.EncodeToString(tx.merkleHash)
	byteswap(tx.merkleHash)
	merkleHashSwapped := hex.EncodeToString(tx.merkleHash)
	byteswap(tx.merkleHash)
	txScriptSig := hex.EncodeToString(tx.scriptSig)
	pubScriptSig := hex.EncodeToString(tx.pubkeyScript)
	fmt.Println(
		"\nCoinbase:    ", txScriptSig, 
		"\nPubKeyScript:", pubScriptSig, 
		"\nMerkle Hash: ", merkleHash, 
		"\nByteswapped: ", merkleHashSwapped )
	fmt.Println("Generating valid nonce based on block header hash, be patient...")
	unixtime := uint32(time.Now().Unix())
	var blockversion uint32 = 1
	blockHeader := uint32tobytes(blockversion)
	blockHeader = append(blockHeader, make([]byte, 32)...)
	blockHeader = append(blockHeader, tx.merkleHash...)
	blockHeader = append(blockHeader, uint32tobytes(uint32(unixtime))...) // byte 68 - 71
	blockHeader = append(blockHeader, uint32tobytes(uint32(nBits))...)
	blockHeader = append(blockHeader, uint32tobytes(startNonce)...)       // byte 76 - 79  
	start := time.Now()
	for {
		blockhash1 := sha256.Sum256(blockHeader)
		blockhash2 := sha256.Sum256(blockhash1[:])
		if bytesarezero(blockhash2[nBits>>24:]) {
			fmt.Println("\n native block hash:", hex.EncodeToString(blockhash2[:]))
			byteswap(blockhash2[:])
			blockHash := hex.EncodeToString(blockhash2[:])
			fmt.Println("\nBlock found!\n",
				"\nHash:     ", blockHash, 
				"\nNonce:    ", startNonce, 
				"\nUnix time:", unixtime)
				fmt.Println("\nBlock header encoded in hex:\n", hex.EncodeToString(blockHeader))
				fmt.Println("\nTime for nonce search:", time.Since(start))
				os.Exit(0)
		}
		startNonce++
		if startNonce < maxNonce {
			blockHeader[76] = byte(startNonce)
			blockHeader[77] = byte(startNonce>>8)
			blockHeader[78] = byte(startNonce>>16)
			blockHeader[79] = byte(startNonce>>24)
		} else {
			startNonce = 0
			unixtime = uint32(time.Now().Unix())
			blockHeader[68] = byte(unixtime)
			blockHeader[69] = byte(unixtime>>8)
			blockHeader[70] = byte(unixtime>>16)
			blockHeader[71] = byte(unixtime>>24)
		}
	}
}

func uint32tobytes(u uint32) []byte {
	b := make([]byte, 4)
	b[0] = byte(u)
	for i := uint(1); i<4; i++ { b[i] = byte(u>>(i*8)) }
	return b
}

func bytesarezero(b []byte) bool {
	for i := range b {
		if b[i] != 0 { return false }
	}
	return true
}

func uint64tobytes(u uint64) []byte {
	b := make([]byte, 8)
	b[0] = byte(u)
	for i := uint(1); i<8; i++ { b[i] = byte(u>>(i*8)) }
	return b
}