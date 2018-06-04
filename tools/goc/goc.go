package main

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/others/sys"
)

var (
	// Host -
	Host string
	// SID -
	SID string
)

func httpGet(url string) (res []byte) {
	req, _ := http.NewRequest("GET", url, nil)
	if SID != "" {
		req.AddCookie(&http.Cookie{Name: "sid", Value: SID})
	}
	r, er := new(http.Client).Do(req)
	if er != nil {
		println(url, er.Error())
		os.Exit(1)
	}
	if SID == "" {
		for i := range r.Cookies() {
			if r.Cookies()[i].Name == "sid" {
				SID = r.Cookies()[i].Value
				//fmt.Println("sid", SID)
			}
		}
	}
	if r.StatusCode == 200 {
		defer r.Body.Close()
		res, _ = ioutil.ReadAll(r.Body)
	} else {
		println(url, "http.Get returned code", r.StatusCode)
		os.Exit(1)
	}
	return
}

func fetchBalance() {
	os.RemoveAll("balance/")

	d := httpGet(Host + "balance.zip")
	r, er := zip.NewReader(bytes.NewReader(d), int64(len(d)))
	if er != nil {
		println(er.Error())
		os.Exit(1)
	}

	os.Mkdir("balance/", 0777)
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			println(err.Error())
			os.Exit(1)
		}
		dat, _ := ioutil.ReadAll(rc)
		rc.Close()
		ioutil.WriteFile(f.Name, dat, 0666)
	}
}

func listWallets() {
	d := httpGet(Host + "wallets.xml")
	var wls struct {
		Wallet []struct {
			Name     string
			Selected bool
		}
	}
	er := xml.Unmarshal(d, &wls)
	if er != nil {
		println(er.Error())
		os.Exit(1)
	}
	for i := range wls.Wallet {
		fmt.Print(wls.Wallet[i].Name)
		if wls.Wallet[i].Selected {
			fmt.Print(" (selected)")
		}
		fmt.Println()
	}
}

func switchToWallet(s string) {
	httpGet(Host + "cfg") // get SID
	u, _ := url.Parse(Host + "cfg")
	ps := url.Values{}
	ps.Add("sid", SID)
	ps.Add("qwalsel", s)
	u.RawQuery = ps.Encode()
	httpGet(u.String())
}

func pushTx(rawtx string) {
	dat := sys.GetRawData(rawtx)
	if dat == nil {
		println("Cannot fetch the raw transaction data (specify hexdump or filename)")
		return
	}

	val := make(url.Values)
	val["rawtx"] = []string{hex.EncodeToString(dat)}

	r, er := http.PostForm(Host+"txs", val)
	if er != nil {
		println(er.Error())
		os.Exit(1)
	}
	if r.StatusCode == 200 {
		defer r.Body.Close()
		res, _ := ioutil.ReadAll(r.Body)
		if len(res) > 100 {
			txid := btc.NewSha2Hash(dat)
			fmt.Println("TxID", txid.String(), "loaded")

			httpGet(Host + "cfg") // get SID
			//fmt.Println("sid", SID)

			u, _ := url.Parse(Host + "txs2s.xml")
			ps := url.Values{}
			ps.Add("sid", SID)
			ps.Add("send", txid.String())
			u.RawQuery = ps.Encode()
			httpGet(u.String())
		}
	} else {
		println("http.Post returned code", r.StatusCode)
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Println("Specify the command and (optionally) its arguments:")
	fmt.Println("  wal [wallet_name] - switch to a given wallet (or list them)")
	fmt.Println("  bal - creates balance/ folder for current wallet")
	fmt.Println("  ptx <rawtx> - pushes raw tx into the network")
}

func main() {
	if len(os.Args) < 2 {
		showHelp()
		return
	}

	Host = os.Getenv("Duod_WEBUI")
	if Host == "" {
		Host = "http://127.0.0.1:8833/"
	} else {
		if !strings.HasPrefix(Host, "http://") {
			Host = "http://" + Host
		}
		if !strings.HasSuffix(Host, "/") {
			Host = Host + "/"
		}
	}
	fmt.Println("Duod WebUI at", Host, "(you can overwrite it via env variable Duod_WEBUI)")

	switch os.Args[1] {
	case "wal":
		if len(os.Args) > 2 {
			switchToWallet(os.Args[2])
		} else {
			listWallets()
		}

	case "bal":
		fetchBalance()

	case "ptx":
		pushTx(os.Args[2])

	default:
		showHelp()
	}
}
