package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/calibrae-project/spawn"
	"github.com/calibrae-project/spawn/lib/btc"
	"github.com/calibrae-project/spawn/lib/others/ltc"
	"github.com/calibrae-project/spawn/lib/others/utils"
	"github.com/calibrae-project/spawn/lib/utxo"
)

// MaxUnspentAtOnce -
const MaxUnspentAtOnce = 20

var (
	proxy   string
	ltcMode bool
	tbtc    bool
)

func printHelp() {
	fmt.Println()
	fmt.Println("Specify at lest one parameter on the command line.")
	fmt.Println("  Name of one text file containing BTC/LTC addresses,")
	fmt.Println("... or space separteted BTC/LTC addresses themselves.")
	fmt.Println()
	fmt.Println("Add -ltc at the command line, to force fetching Litecoin balance.")
	fmt.Println("Add -t at the command line, to force fetching Testnet balance.")
	fmt.Println()
	fmt.Println("To use Tor, setup environment variable TOR=host:port")
	fmt.Println("The host:port should point to your Tor's SOCKS proxy.")
}

func dials5(tcp, dest string) (conn net.Conn, err error) {
	//println("Tor'ing to", dest, "via", proxy)
	var buf [10]byte
	var host, ps string
	var port uint64

	conn, err = net.Dial(tcp, proxy)
	if err != nil {
		return
	}

	_, err = conn.Write([]byte{5, 1, 0})
	if err != nil {
		return
	}

	_, err = io.ReadFull(conn, buf[:2])
	if err != nil {
		return
	}

	if buf[0] != 5 {
		err = errors.New("we only support SOCKS5 proxy")
	} else if buf[1] != 0 {
		err = errors.New("SOCKS proxy connection refused")
		return
	}

	host, ps, err = net.SplitHostPort(dest)
	if err != nil {
		return
	}

	port, err = strconv.ParseUint(ps, 10, 16)
	if err != nil {
		return
	}

	req := make([]byte, 5+len(host)+2)
	copy(req[:4], []byte{5, 1, 0, 3})
	req[4] = byte(len(host))
	copy(req[5:], []byte(host))
	binary.BigEndian.PutUint16(req[len(req)-2:], uint16(port))
	_, err = conn.Write(req)
	if err != nil {
		return
	}

	_, err = io.ReadFull(conn, buf[:])
	if err != nil {
		return
	}

	if buf[1] != 0 {
		err = errors.New("SOCKS proxy connection terminated")
	}

	return
}

func splitHostPort(addr string) (host string, port uint16, err error) {
	host, portStr, err := net.SplitHostPort(addr)
	portInt, err := strconv.ParseUint(portStr, 10, 16)
	port = uint16(portInt)
	return
}

func currUnit() string {
	if ltcMode {
		return "LTC"
	}
	return "BTC"
}

func loadWallet(fn string) (addrs []*btc.Addr) {
	f, e := os.Open(fn)
	if e != nil {
		println(e.Error())
		return
	}
	defer f.Close()
	rd := bufio.NewReader(f)
	linenr := 0
	for {
		var l string
		l, e = rd.ReadString('\n')
		l = strings.Trim(l, " \t\r\n")
		linenr++
		if len(l) > 0 {
			if l[0] == '@' {
				fmt.Println("netsted wallet in line", linenr, "- ignore it")
			} else if l[0] != '#' {
				ls := strings.SplitN(l, " ", 2)
				if len(ls) > 0 {
					a, e := btc.NewAddrFromString(ls[0])
					if e != nil {
						println(fmt.Sprint(fn, ":", linenr), e.Error())
					} else {
						addrs = append(addrs, a)
					}
				}
			}
		}
		if e != nil {
			break
		}
	}
	return
}

func main() {
	fmt.Println("Spawn BalIO version", Spawn.Version)

	if len(os.Args) < 2 {
		printHelp()
		return
	}

	proxy = os.Getenv("TOR")
	if proxy != "" {
		fmt.Println("Using Tor at", proxy)
		http.DefaultClient.Transport = &http.Transport{Dial: dials5}
	} else {
		fmt.Println("WARNING: not using Tor (setup TOR variable, if you want)")
	}

	var addrs []*btc.Addr

	var argz []string
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "-ltc" {
			ltcMode = true
		} else if os.Args[i] == "-t" {
			tbtc = true
		} else {
			argz = append(argz, os.Args[i])
		}
	}

	if len(argz) == 1 {
		fi, er := os.Stat(argz[0])
		if er == nil && fi.Size() > 10 && !fi.IsDir() {
			addrs = loadWallet(argz[0])
			if addrs != nil {
				fmt.Println("Found", len(addrs), "address(es) in", argz[0])
			}
		}
	}

	if len(addrs) == 0 {
		for i := range argz {
			a, e := btc.NewAddrFromString(argz[i])
			if e != nil {
				println(argz[i], ": ", e.Error())
				return
			}
			addrs = append(addrs, a)
		}
	}

	if len(addrs) == 0 {
		printHelp()
		return
	}

	for i := range addrs {
		switch addrs[i].Version {
		case 48:
			ltcMode = true
		case 111:
			tbtc = true
		}
	}

	if tbtc && ltcMode {
		println("Litecoin's testnet is not suppported")
		return
	}

	if len(addrs) == 0 {
		println("No addresses to fetch balance for")
		return
	}

	var sum, outcnt uint64

	os.RemoveAll("balance/")
	os.Mkdir("balance/", 0700)
	unsp, _ := os.Create("balance/unspent.txt")
	for off := 0; off < len(addrs); off++ {
		var res utxo.AllUnspentTx
		if ltcMode {
			res = ltc.GetUnspent(addrs[off])
		} else if tbtc {
			res = utils.GetUnspentTestnet(addrs[off])
		} else {
			res = utils.GetUnspent(addrs[off])
		}
		for _, r := range res {
			var txraw []byte
			id := btc.NewUint256(r.TxPrevOut.Hash[:])
			if ltcMode {
				txraw = ltc.GetTxFromWeb(id)
			} else if tbtc {
				txraw = utils.GetTestnetTxFromWeb(id)
			} else {
				txraw = utils.GetTxFromWeb(id)
			}
			if len(txraw) > 0 {
				ioutil.WriteFile("balance/"+id.String()+".tx", txraw, 0666)
			} else {
				println("ERROR: cannot fetch raw tx data for", id.String())
				//os.Exit(1)
			}

			sum += r.Value
			outcnt++

			fmt.Fprintln(unsp, r.UnspentTextLine())
		}
	}
	unsp.Close()
	if outcnt > 0 {
		fmt.Printf("Total %.8f %s in %d unspent outputs.\n", float64(sum)/1e8, currUnit(), outcnt)
		fmt.Println("The data has been stored in 'balance' folder.")
		fmt.Println("Use it with the wallet app to spend any of it.")
	} else {
		fmt.Println("No coins found on the given address(es).")
	}
}
