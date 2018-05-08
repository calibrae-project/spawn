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
	if common.NetworkClosed.Get() || usif.Exit_now.Get() {
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

func load_template(fn string) string {
	dat, _ := ioutil.ReadFile("www/templates/" + fn)
	return string(dat)
}

func templ_add(tmpl string, id string, val string) string {
	return strings.Replace(tmpl, id, val+id, 1)
}

func p_webui(w http.ResponseWriter, r *http.Request) {
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

func new_session_id(w http.ResponseWriter) (sessid string) {
	var sid [16]byte
	rand.Read(sid[:])
	sessid = hex.EncodeToString(sid[:])
	http.SetCookie(w, &http.Cookie{Name: "sid", Value: sessid})
	return
}

func write_html_head(w http.ResponseWriter, r *http.Request) {
	startTime = time.Now()

	sessid := sid(r)
	if sessid == "" {
		sessid = new_session_id(w)
	}

	s := load_template("page_head.html")
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

func write_html_tail(w http.ResponseWriter) {
	s := load_template("page_tail.html")
	s = strings.Replace(s, "<!--LOAD_TIME-->", time.Now().Sub(startTime).String(), 1)
	w.Write([]byte(s))
}

func p_help(w http.ResponseWriter, r *http.Request) {
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

	page := load_template(fname)
	write_html_head(w, r)
	w.Write([]byte(page))
	write_html_tail(w)
}

func p_wallet_is_off(w http.ResponseWriter, r *http.Request) {
	s := load_template("wallet_off.html")
	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}

func ServerThread(iface string) {
	http.HandleFunc("/webui/", p_webui)

	http.HandleFunc("/wal", p_wal)
	http.HandleFunc("/snd", pSnd)
	http.HandleFunc("/balance.json", json_balance)
	http.HandleFunc("/payment.zip", dlPayment)
	http.HandleFunc("/balance.zip", dl_balance)

	http.HandleFunc("/net", pNet)
	http.HandleFunc("/txs", p_txs)
	http.HandleFunc("/blocks", pBlocks)
	http.HandleFunc("/miners", pMiners)
	http.HandleFunc("/counts", pCounts)
	http.HandleFunc("/cfg", pCfg)
	http.HandleFunc("/help", p_help)

	http.HandleFunc("/txs2s.xml", xml_txs2s)
	http.HandleFunc("/txsre.xml", xml_txsre)
	http.HandleFunc("/txw4i.xml", xml_txw4i)
	http.HandleFunc("/rawTx", rawTx)

	http.HandleFunc("/", pHome)
	http.HandleFunc("/status.json", jsonStatus)
	http.HandleFunc("/counts.json", jsonCounts)
	http.HandleFunc("/system.json", jsonSystem)
	http.HandleFunc("/bwidth.json", jsonBandwidth)
	http.HandleFunc("/txstat.json", json_txstat)
	http.HandleFunc("/netcon.json", jsonNetCon)
	http.HandleFunc("/blocks.json", jsonBlocks)
	http.HandleFunc("/peerst.json", jsonPeersT)
	http.HandleFunc("/bwchar.json", jsonBWChar)
	http.HandleFunc("/mempoolStats.json", json_mempool_stats)
	http.HandleFunc("/mempool_fees.json", json_mempool_fees)
	http.HandleFunc("/blkver.json", jsonBlkVer)
	http.HandleFunc("/miners.json", jsonMiners)
	http.HandleFunc("/blfees.json", jsonBlockFees)
	http.HandleFunc("/walsta.json", json_wallet_status)

	http.HandleFunc("/mempool_fees.txt", txt_mempool_fees)

	http.ListenAndServe(iface, nil)
}
