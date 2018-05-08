package webui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/calibrae-project/spawn"
	"github.com/calibrae-project/spawn/client/common"
	"github.com/calibrae-project/spawn/client/usif"
)

var startTime time.Time

func ipchecker(r *http.Request) bool {
	if common.NetworkClosed.Get() || usif.ExitNow.Get() {
		return false
	}
	var a, b, c, d uint32
	n, _ := fmt.Sscanf(r.RemoteAddr, "%d.%d.%d.%d", &a, &b, &c, &d)
	if n != 4 {
		return false
	}
	addr := (a << 24) | (b << 16) | (c << 8) | d
	common.LockCfg()
	for i := range common.WebUIAllowed {
		if (addr & common.WebUIAllowed[i].Mask) == common.WebUIAllowed[i].Addr {
			common.UnlockCfg()
			r.ParseForm()
			return true
		}
	}
	common.UnlockCfg()
	println("ipchecker:", r.RemoteAddr, "is blocked")
	return false
}

func loadTemplate(fn string) string {
	dat, _ := ioutil.ReadFile("www/templates/" + fn)
	return string(dat)
}

func templateAdd(tmpl string, id string, val string) string {
	return strings.Replace(tmpl, id, val+id, 1)
}

func pWebUI(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	pth := strings.SplitN(r.URL.Path[1:], "/", 3)
	if len(pth) == 2 {
		dat, _ := ioutil.ReadFile("www/resources/" + pth[1])
		if len(dat) > 0 {
			switch filepath.Ext(r.URL.Path) {
			case ".js":
				w.Header()["Content-Type"] = []string{"text/javascript"}
			case ".css":
				w.Header()["Content-Type"] = []string{"text/css"}
			}
			w.Write(dat)
		} else {
			http.NotFound(w, r)
		}
	}
}

func sid(r *http.Request) string {
	c, _ := r.Cookie("sid")
	if c != nil {
		return c.Value
	}
	return ""
}

func checksid(r *http.Request) bool {
	if len(r.Form["sid"]) == 0 {
		return false
	}
	if len(r.Form["sid"][0]) < 16 {
		return false
	}
	return r.Form["sid"][0] == sid(r)
}

func newSessionID(w http.ResponseWriter) (sessid string) {
	var sid [16]byte
	rand.Read(sid[:])
	sessid = hex.EncodeToString(sid[:])
	http.SetCookie(w, &http.Cookie{Name: "sid", Value: sessid})
	return
}

func writeHTMLHead(w http.ResponseWriter, r *http.Request) {
	startTime = time.Now()

	sessid := sid(r)
	if sessid == "" {
		sessid = newSessionID(w)
	}

	s := loadTemplate("page_head.html")
	s = strings.Replace(s, "{PAGE_TITLE}", common.CFG.WebUI.Title, 1)
	s = strings.Replace(s, "/*_SESSION_ID_*/", "var sid = '"+sessid+"'", 1)
	s = strings.Replace(s, "/*_AVERAGE_FEE_SPB_*/", fmt.Sprint("var avg_fee_spb = ", common.GetAverageFee()), 1)
	s = strings.Replace(s, "/*_SERVER_MODE_*/", fmt.Sprint("var server_mode = ", common.CFG.WebUI.ServerMode), 1)
	s = strings.Replace(s, "/*_TIME_NOW_*/", fmt.Sprint("= ", time.Now().Unix()), 1)
	s = strings.Replace(s, "/*_WALLET_ON_*/", fmt.Sprint("var wallet_on = ", common.GetBool(&common.WalletON)), 1)
	s = strings.Replace(s, "/*_CHAIN_IN_SYNC_*/", fmt.Sprint("var chain_in_sync = ", common.GetBool(&common.BlockChainSynchronized)), 1)

	if r.URL.Path != "/" {
		s = strings.Replace(s, "{HELPURL}", "help#"+r.URL.Path[1:], 1)
	} else {
		s = strings.Replace(s, "{HELPURL}", "help", 1)
	}
	s = strings.Replace(s, "{VERSION}", Spawn.Version, 1)
	if common.Testnet {
		s = strings.Replace(s, "{TESTNET}", " Testnet ", 1)
	} else {
		s = strings.Replace(s, "{TESTNET}", "", 1)
	}

	w.Write([]byte(s))
}

func writeHTMLTail(w http.ResponseWriter) {
	s := loadTemplate("page_tail.html")
	s = strings.Replace(s, "<!--LOAD_TIME-->", time.Now().Sub(startTime).String(), 1)
	w.Write([]byte(s))
}

func pHelp(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	fname := "help.html"
	if len(r.Form["topic"]) > 0 && len(r.Form["topic"][0]) == 4 {
		for i := 0; i < 4; i++ {
			if r.Form["topic"][0][i] < 'a' || r.Form["topic"][0][i] > 'z' {
				goto broken_topic // we only accept 4 locase characters
			}
		}
		fname = "help_" + r.Form["topic"][0] + ".html"
	}
broken_topic:

	page := loadTemplate(fname)
	writeHTMLHead(w, r)
	w.Write([]byte(page))
	writeHTMLTail(w)
}

func pWalletIsOff(w http.ResponseWriter, r *http.Request) {
	s := loadTemplate("wallet_off.html")
	writeHTMLHead(w, r)
	w.Write([]byte(s))
	writeHTMLTail(w)
}

// ServerThread -
func ServerThread(iface string) {
	http.HandleFunc("/webui/", pWebUI)

	http.HandleFunc("/wal", pWal)
	http.HandleFunc("/snd", pSnd)
	http.HandleFunc("/balance.json", jsonBalance)
	http.HandleFunc("/payment.zip", dlPayment)
	http.HandleFunc("/balance.zip", dlBalance)

	http.HandleFunc("/net", pNet)
	http.HandleFunc("/txs", pTxs)
	http.HandleFunc("/blocks", pBlocks)
	http.HandleFunc("/miners", pMiners)
	http.HandleFunc("/counts", pCounts)
	http.HandleFunc("/cfg", pCfg)
	http.HandleFunc("/help", pHelp)

	http.HandleFunc("/txs2s.xml", xmlTxs2s)
	http.HandleFunc("/txsre.xml", xmlTxSre)
	http.HandleFunc("/txw4i.xml", xmlTxW4i)
	http.HandleFunc("/rawTx", rawTx)

	http.HandleFunc("/", pHome)
	http.HandleFunc("/status.json", jsonStatus)
	http.HandleFunc("/counts.json", jsonCounts)
	http.HandleFunc("/system.json", jsonSystem)
	http.HandleFunc("/bwidth.json", jsonBandwidth)
	http.HandleFunc("/txstat.json", jsonTxStat)
	http.HandleFunc("/netcon.json", jsonNetCon)
	http.HandleFunc("/blocks.json", jsonBlocks)
	http.HandleFunc("/peerst.json", jsonPeersT)
	http.HandleFunc("/bwchar.json", jsonBWChar)
	http.HandleFunc("/mempoolStats.json", jsonMempoolStats)
	http.HandleFunc("/mempool_fees.json", jsonMempoolFees)
	http.HandleFunc("/blkver.json", jsonBlkVer)
	http.HandleFunc("/miners.json", jsonMiners)
	http.HandleFunc("/blfees.json", jsonBlockFees)
	http.HandleFunc("/walsta.json", jsonWalletStatus)

	http.HandleFunc("/mempool_fees.txt", txtMempoolFees)

	http.ListenAndServe(iface, nil)
}
