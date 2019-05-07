package main

import (
	"os/user"
	"path"

	_ "bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"time"

	_ "github.com/kr/pretty"
	"gopkg.in/yaml.v2"

	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/syntaqx/echo-middleware/requestid"
)

// var sessStoar *sessions.CookieStore
// var recaptchaSecret, authKey, encryptKey string
var letterRunes = []rune("01234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

//RandString make random strings of runes in n length, if that wasnt completely obvious.
func RandString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

type conf struct {
	RecaptchaSecret string `yaml:"recaptchaSecret"`
	SessAuthKey     string `yaml:"sessAuthKey"`
	SessCipher      string `yaml:"sessCipher"`
	LndTlsCertPath  string `yaml:"lndTlsCertPath"`
	LndMacaroonPath string `yaml:"lndMacaroonPath"`
	LndHost         string `yaml:"lndHost"`
}

func (c *conf) getConf() *conf {
	yamlFile, err := ioutil.ReadFile("goober.conf.yaml")
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
		panic("configuration error")
	}
	err = yaml.Unmarshal(yamlFile, c)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}

	if c.LndTlsCertPath == "" || c.LndMacaroonPath == "" {
		log.Println("getConf, missing tls cert path or macaroon path in config, using default paths.")
		usr, err := user.Current()
		if err != nil {
			panic("Cannot get current user:" + err.Error())
		}

		log.Printf("getConf: The user home directory: " + usr.HomeDir + "\n")

		c.LndTlsCertPath = path.Join(usr.HomeDir, ".lnd/tls.cert")
		c.LndMacaroonPath = path.Join(usr.HomeDir, ".lnd/data/chain/bitcoin/mainnet/admin.macaroon")
	}
	if (c.LndHost == "") {
		c.LndHost = "localhost"
	}
	return c
}

var myConf conf

// var lnConn *grpc.ClientConn

// var sessMgr = &sessionManager{}
var sessMgr *sessionManager
var ln *lndHelper
var captcha *recaptchaHelper

func init() {
	rand.Seed(time.Now().UnixNano())

	myConf.getConf()
	fmt.Printf("read config: %#vn\n", myConf)
	sessMgr = NewSessMgr(&myConf)
	ln = NewLNDHelper(&myConf)
	captcha = NewRecaptchaHelper(&myConf, sessMgr)

	// sessStoar = sessions.NewCookieStore([]byte(myConf.SessAuthKey), []byte(myConf.SessCipher)) //these should be random and not saved in this file. oh well. see docs for more info.

	// lnClient = client
	go ln.MonitorInvoices()
}

var reqIdKey string

func main() {

	r := mux.NewRouter()
	rid := requestid.New()
	reqIdKey = rid.HeaderKey
	// r.
	_ = r
	r.HandleFunc("/getInvoiceForm/", getInvoiceForm)
	r.HandleFunc("/getInvoice/", getInvoice)
	r.HandleFunc("/lastInvoice/", lastInvoice)
	r.HandleFunc("/pollInvoice/", pollInvoice)
	r.HandleFunc("/longPollInvoice/", longPollInvoice)
	// http.HandleFunc("/", sayHello)
	// http.HandleFunc("/bye/", sayBye)
	// if err := http.ListenAndServe(":8081", nil); err != nil {
	if err := http.ListenAndServe(":8081", rid.Handler(r)); err != nil {
		panic(err)
	}
}

func getInvoiceForm(w http.ResponseWriter, r *http.Request) {
	if pass, _ := captcha.isUserReal(w, r); !pass {
		w.Write([]byte(""))
		return
	}

	log.Println("getInvoiceForm: allow form")
	userSess := sessMgr.GetSession(r)
	rando := RandString(32)
	userSess.Values["authtoken"] = rando
	userSess.Save(r, w)
	log.Printf("getInvoiceForm: session data currently: %#v", userSess.Values)

	resp, err := json.Marshal([]interface{}{true, rando})
	if err != nil {
		fmt.Println(err.Error())
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(resp)
}

func getInvoice(w http.ResponseWriter, r *http.Request) {
	var params map[string]interface{}
	err := json.Unmarshal([]byte(r.URL.Query().Get("p")), &params)
	if err != nil {
		panic(err)
	}
	expectToken, haveExpectToken := sessMgr.GetSession(r).Values["authtoken"].(string)

	gotToken, haveParamToken := params["authtoken"].(string)
	amountFloat, _ := params["amount"].(float64)
	earmark, _ := params["earmark"].(string)
	amount := int64(amountFloat)
	if amount == 0 {
		amount = 1
	}
	authed := true
	if !haveExpectToken || !haveParamToken || (gotToken != expectToken) {
		authed = false
		log.Printf("got authtoken %s when we expected %s\n", gotToken, expectToken)
	}

	if !authed {
		w.Write([]byte(""))
		return
	}

	var score float64
	var pass = false
	if pass, score = captcha.isUserReal(w, r); !pass {
		w.Write([]byte(""))
		return
	}

	memo := "CHWS Donation"
	if earmark != "" {
		memo = memo + ", Earmarked for " + earmark
	}
	addInvoiceResp := ln.NewInvoiceFromLND(amount, memo)
	sess := sessMgr.GetSession(r)
	sess.Values["invoice-payreq"] = addInvoiceResp.PaymentRequest
	sess.Values["invoice-rhash"] = hex.EncodeToString(addInvoiceResp.RHash)
	sess.Save(r, w)

	log.Printf("clearly had a good enough score: %f\n", score)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write([]byte(addInvoiceResp.PaymentRequest))
}

func lastInvoice(w http.ResponseWriter, r *http.Request) {
	sess := sessMgr.GetSession(r)

	payreq, payreqOk := sess.Values["invoice-payreq"].(string)
	rhash, rhashOk := sess.Values["invoice-rhash"].(string)
	log.Printf("lastInvoice %s: %# v\n", reqIdKey, r.Header.Get(reqIdKey))
	if !payreqOk || !rhashOk {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	settled, expired := ln.getInvoiceStatus(rhash)
	if expired || settled {
		payreq = ""
		delete(sess.Values, "invoice-rhash")
		delete(sess.Values, "invoice-payreq")
		sess.Save(r, w)
	}

	w.Write([]byte(payreq))
}

func pollInvoice(w http.ResponseWriter, r *http.Request) {
	sess := sessMgr.GetSession(r)
	rhash, ok := sess.Values["invoice-rhash"].(string)
	if !ok {
		log.Printf("pollInvoice, no saved rhash in session, try for one from the query string")
		rhash = r.URL.Query().Get("rhash")
	}
	if rhash == "" {
		log.Printf("pollInvoice, no rhash from session or query :( ... just gonna return without doing anything. does it matter that i dont write anything first with w.Write?")
		return
	}

	settled, expired := ln.getInvoiceStatus(rhash)
	if expired || settled {
		// sess.Values["invoice-rhash"]
		delete(sess.Values, "invoice-rhash")
		delete(sess.Values, "invoice-payreq")
		sess.Save(r, w)
	}
	settledJson, _ := json.Marshal(settled)
	w.Write(settledJson)
}

func sendJSON(w http.ResponseWriter, value interface{}) {
	rStr, _ := json.Marshal(value)
	log.Printf("sendJSON: about to send this:%s\n", rStr)
	w.Write(rStr)
}

func longPollInvoice(w http.ResponseWriter, r *http.Request) {

	sess := sessMgr.GetSession(r)
	rhash, ok := sess.Values["invoice-rhash"].(string)
	validRequest := true
	if !ok {
		log.Printf("longPollInvoice, no saved rhash in session, try for one from the query string")
		rhash = r.URL.Query().Get("rhash")
	}
	if rhash == "" {
		log.Printf("longPollInvoice, no rhash from session or query :( ... just gonna return without doing anything. does it matter that i dont write anything first with w.Write?")
		validRequest = false
	}
	reqId := r.Header.Get(reqIdKey)

	result := map[string]bool{}
	if !validRequest {
		result["invalid"] = true
		sendJSON(w, result)
		return
	}

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	ctx, cancel = context.WithTimeout(context.Background(), 299*time.Second)
	defer cancel()
	closedNotify := w.(http.CloseNotifier).CloseNotify()

	select {
	case <-closedNotify:
		log.Printf("longPollInvoice: reqId %s client closed connection:\n", reqId)
		return
	case <-ln.ReadSettled(rhash, reqId):
		// case <-ln.RhashSettlements[rhash][reqId]:
		result["gotresult"] = true
		result["settled"] = true
		log.Printf("longPollInvoice: reqId %s got a result like: %#v\n", reqId, result)
	case <-ctx.Done():
		result["timedout"] = true
		log.Printf("longPollInvoice: reqId %s timed out with no settlement.\n", reqId)
	}
	log.Printf("longPollInvoice: reqId %s got down here.", reqId)

	sendJSON(w, result)
}
