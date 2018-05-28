package rpcapi

// test it with:
// curl --user someuser:somepass --data-binary '{"method":"Arith.Add","params":[{"A":7,"B":1}],"id":0}' -H 'content-type: text/plain;' http://127.0.0.1:8222/

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"

	"github.com/ParallelCoinTeam/duod/client/common"
	"github.com/ParallelCoinTeam/duod/lib/L"
)

// RPCError -
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// RPCResponse -
type RPCResponse struct {
	ID     interface{} `json:"id"`
	Result interface{} `json:"result"`
	Error  interface{} `json:"error"`
}

// RPCCommand -
type RPCCommand struct {
	ID     interface{} `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

// processRPC -
func processRPC(b []byte) (out []byte) {
	ioutil.WriteFile("rpc_cmd.json", b, 0777)
	exCmd := exec.Command("C:\\Tools\\DEV\\Git\\mingw64\\bin\\curl.EXE",
		"--user", "Duodrpc:Duodpwd", "--data-binary", "@rpc_cmd.json", "http://127.0.0.1:18332/")
	out, _ = exCmd.Output()
	return
}

func myHandler(w http.ResponseWriter, r *http.Request) {
	u, p, ok := r.BasicAuth()
	if !ok {
		L.Error("No HTTP Authentication data")
		return
	}
	if u != common.CFG.RPC.Username {
		L.Error("HTTP Authentication: bad username")
		return
	}
	if p != common.CFG.RPC.Password {
		L.Error("HTTP Authentication: bad password")
		return
	}
	L.Debug("========================handler", r.Method, r.URL.String(), u, p, ok, "=================")
	b, e := ioutil.ReadAll(r.Body)
	if e != nil {
		L.Error(e.Error())
		return
	}

	var RPCCmd RPCCommand
	jd := json.NewDecoder(bytes.NewReader(b))
	jd.UseNumber()
	e = jd.Decode(&RPCCmd)
	if e != nil {
		L.Error(e.Error())
	}

	var resp RPCResponse
	resp.ID = RPCCmd.ID
	switch RPCCmd.Method {
	case "getblocktemplate":
		var respMy RPCGetBlockTemplateResp

		GetNextBlockTemplate(&respMy.Result)

		if false {
			var respOK RPCGetBlockTemplateResp
			BitcoindResult := processRPC(b)
			//ioutil.WriteFile("getblocktemplate_resp.json", BitcoindResult, 0777)

			//fmt.Print("getblocktemplate...", sto.Sub(sta).String(), string(b))

			jd = json.NewDecoder(bytes.NewReader(BitcoindResult))
			jd.UseNumber()
			e = jd.Decode(&respOK)

			if respMy.Result.PreviousBlockHash != respOK.Result.PreviousBlockHash {
				L.Debug("satoshi @", respOK.Result.PreviousBlockHash, respOK.Result.Height)
				L.Debug("Duod  @", respMy.Result.PreviousBlockHash, respMy.Result.Height)
			} else {
				L.Debug(".", len(respMy.Result.Transactions), respMy.Result.Coinbasevalue)
				if respMy.Result.Mintime != respOK.Result.Mintime {
					L.Debug("\007Mintime:", respMy.Result.Mintime, respOK.Result.Mintime)
				}
				if respMy.Result.Bits != respOK.Result.Bits {
					L.Debug("\007Bits:", respMy.Result.Bits, respOK.Result.Bits)
				}
				if respMy.Result.Target != respOK.Result.Target {
					L.Debug("\007Target:", respMy.Result.Target, respOK.Result.Target)
				}
			}
		}

		b, _ = json.Marshal(&respMy)
		//ioutil.WriteFile("json/"+RPCCmd.Method+"_resp_my.json", b, 0777)
		w.Write(append(b, 0x0a))
		return

	case "validateaddress":
		switch uu := RPCCmd.Params.(type) {
		case []interface{}:
			if len(uu) == 1 {
				resp.Result = ValidateAddress(uu[0].(string))
			}
		default:
			L.Debug("unexpected type", uu)
		}

	case "submitblock":
		//ioutil.WriteFile("submitblock.json", b, 0777)
		SubmitBlock(&RPCCmd, &resp, b)

	default:
		L.Debug("Method:", RPCCmd.Method, len(b))
		//w.Write(BitcoindResult)
		resp.Error = RPCError{Code: -32601, Message: "Method not found"}
	}

	b, e = json.Marshal(&resp)
	if e != nil {
		L.Debug("json.Marshal(&resp):", e.Error())
	}

	//ioutil.WriteFile(RPCCmd.Method+"_resp.json", b, 0777)
	w.Write(append(b, 0x0a))
}

// StartServer -
func StartServer(port uint32) {
	L.Debug("Starting RPC server at port", port)
	mux := http.NewServeMux()
	mux.HandleFunc("/", myHandler)
	http.ListenAndServe(fmt.Sprint("127.0.0.1:", port), mux)
}
