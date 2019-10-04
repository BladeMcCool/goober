package main

import (
	"os"
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

var letterRunes = []rune("01234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
var earmarkOptions = [][2]string{
	{"outreach", "Homeless Outreach"},
	{"beach", "Beach Cleanup"},
	{"crypto_edu", "Cryptocurrency Education"},
	{"infotech_edu", "InfoTech and Computer Education"},
	{"marketing", "Messaging and Promotion"},
	{"admin", "Administration"},
	{"skunkworks", "Shadow Operations/Skunk Works"},
}

//RandString make random strings of runes in n length, if that wasnt completely obvious.
func RandString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

type conf struct {
	RecaptchaSecret  string `yaml:"recaptchaSecret"`
	RecaptchaSiteKey string `yaml:"recaptchaSiteKey"`
	SessAuthKey      string `yaml:"sessAuthKey"`
	SessCipher       string `yaml:"sessCipher"`
	LndTlsCertPath   string `yaml:"lndTlsCertPath"`
	LndMacaroonPath  string `yaml:"lndMacaroonPath"`
	LndRpcHostPort   string `yaml:"lndRpcHostPort"`
	ListenPort       string `yaml:"listenPort"`
	OnChainBTCAddr   string `yaml:"onChainBTCAddr"`
	RestartFile      string `yaml:"restartFile"`
	PgHost           string `yaml:"pgHost"`
	PgPass           string `yaml:"pgPass"`
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
	if c.LndRpcHostPort == "" {
		c.LndRpcHostPort = "localhost:10009"
	}
	if c.RestartFile == "" {
		c.RestartFile = "/shared/goober/restart"
	}
	return c
}

var myConf conf
var sessMgr *sessionManager
var ln *lndHelper
var captcha *recaptchaHelper
var pgConn *postgresHelper

func init() {
	log.SetOutput(os.Stdout)
	rand.Seed(time.Now().UnixNano())
	xx
	myConf.getConf()
	fmt.Println("!!!test message please ignore.")
	fmt.Printf("read config: %#vn\n", myConf)
	go monitorShutdown(&myConf)
	sessMgr = NewSessMgr(&myConf)
	ln = NewLNDHelper(&myConf)
	captcha = NewRecaptchaHelper(&myConf, sessMgr)

	fmt.Println("see TODOs in code!! (re saving paid invoice info into postgres)")
	// DbTest()
	pgConn = NewPostgresHelper(&myConf)

	go ln.MonitorInvoices()
}

var reqIdKey string
var quitChan = make(chan struct{})

func main() {
	defer pgConn.closeDb()

	// r := mux.NewRouter()
	// rid := requestid.New()
	// reqIdKey = rid.HeaderKey
	// _ = r
	// r.HandleFunc("/getRecaptchaSiteKey/", getRecaptchaSiteKey)
	// r.HandleFunc("/getInvoiceForm/", getInvoiceForm)
	// r.HandleFunc("/getInvoice/", getInvoice)
	// r.HandleFunc("/lastInvoice/", lastInvoice)
	// r.HandleFunc("/pollInvoice/", pollInvoice)
	// r.HandleFunc("/longPollInvoice/", longPollInvoice)
	// // http.HandleFunc("/", sayHello)
	// // http.HandleFunc("/bye/", sayBye)
	// // if err := http.ListenAndServe(":8081", nil); err != nil {}
	// listenPort := myConf.ListenPort
	// if listenPort == "" {
	// 	listenPort = "8081"
	// }
	// if err := http.ListenAndServe(":"+listenPort, rid.Handler(r)); err != nil {
	// 	panic(err)
	// }
	srv := startServer()
	_ = srv
	log.Printf("waiting to be told to quit listening.")
	_ = <-quitChan
	log.Printf("QUITTING TIME.")
}

func startServer() *http.Server {
	r := mux.NewRouter()
	rid := requestid.New()
	reqIdKey = rid.HeaderKey
	_ = r
	r.HandleFunc("/getRecaptchaSiteKey/", getRecaptchaSiteKey)
	r.HandleFunc("/getInvoiceForm/", getInvoiceForm)
	r.HandleFunc("/getInvoice/", getInvoice)
	r.HandleFunc("/lastInvoice/", lastInvoice)
	r.HandleFunc("/pollInvoice/", pollInvoice)
	r.HandleFunc("/longPollInvoice/", longPollInvoice)
	// http.HandleFunc("/", sayHello)
	// http.HandleFunc("/bye/", sayBye)
	// if err := http.ListenAndServe(":8081", nil); err != nil {}
	listenPort := myConf.ListenPort
	if listenPort == "" {
		listenPort = "8081"
	}
	srv := &http.Server{
		Addr:    ":" + listenPort,
		Handler: rid.Handler(r),
	}
	// if err := http.ListenAndServe(":"+listenPort, rid.Handler(r)); err != nil {
	// 	panic(err)
	// }
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			panic(err)
		}
	}()
	return srv
}

// func DbTest() {
// 	db := pg.Connect(&pg.Options{
// 		Addr:     myConf.PgHost + ":5432",
// 		User:     "postgres",
// 		Password: myConf.PgPass,
// 		Database: "postgres",
// 	})

// 	var horselegs struct {
// 		Legs int
// 	}

// 	res, err := db.QueryOne(&horselegs, `SELECT * FROM horse`)
// 	if err != nil {
// 		panic(err)
// 	}
// 	fmt.Println(res.RowsAffected())
// 	fmt.Println(horselegs)

// 	defer db.Close()
// }
func getRecaptchaSiteKey(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(myConf.RecaptchaSiteKey))
	return
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

	resp, err := json.Marshal([]interface{}{true, rando, myConf.OnChainBTCAddr, earmarkOptions})
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
	attribution, _ := params["attribution"].(string)
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
		earmarkChosenOpt := 0
		for ind, option := range earmarkOptions {
			if option[0] == earmark {
				earmarkChosenOpt = ind
				break
			}
		}
		memo = memo + ", Earmarked for " + earmarkOptions[earmarkChosenOpt][1]
	}
	if attribution != "" {
		memo = memo + ", Attribution: " + attribution
	}
	addInvoiceResp := ln.NewInvoiceFromLND(amount, memo)
	sess := sessMgr.GetSession(r)
	sess.Values["invoice-payreq"] = addInvoiceResp.PaymentRequest
	sess.Values["invoice-rhash"] = hex.EncodeToString(addInvoiceResp.RHash)
	sess.Values["invoice-sats"] = amount
	sess.Values["invoice-earmark"] = earmark
	sess.Values["invoice-attribution"] = attribution
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
	explicitClear := false
	if r.URL.Query().Get("clear") != "" {
		if isReal, _ := captcha.isUserReal(w, r); isReal {
			explicitClear = true
			log.Printf("lastInvoice, explicitClear, do it for a real person.")
		} else {
			log.Printf("lastInvoice, explicitClear, not doing it for a request that is believed to be automated bot.")
		}
	}
	if expired || settled || explicitClear {
		payreq = ""
		delete(sess.Values, "invoice-rhash")
		delete(sess.Values, "invoice-payreq")
		delete(sess.Values, "invoice-sats")
		delete(sess.Values, "invoice-earmark")
		delete(sess.Values, "invoice-attribution")
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
	if passed, score := captcha.LastRecaptchaPassed(w, r); !passed {
		log.Printf("longPollInvoice, last captcha score was too low at %f", score)
		validRequest = false
	}

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

	reqId := r.Header.Get(reqIdKey)
	select {
	case <-closedNotify:
		log.Printf("longPollInvoice: reqId %s client closed connection:\n", reqId)
		return
	case <-ln.ReadSettled(rhash, reqId):
		// case <-ln.RhashSettlements[rhash][reqId]:
		result["gotresult"] = true
		result["settled"] = true

		//TODO: call something to save the saved invoice data in the pg db, passing the session data map.
		// in there it must pull out fields like:
		//   sess.Values["invoice-earmark"].(string)
		//   sess.Values["invoice-attribution"].(string)
		//   sess.Values["invoice-rhash"].(string)
		//   sess.Values["invoice-payreq"].(string)
		pgConn.savePaidInvoiceDetail(sess.Values)
		// and save them into a record.
		// db conn should be established on startup in init
		// db teardown should be defered in main.

		log.Printf("longPollInvoice: reqId %s got a result like: %#v\n", reqId, result)
	case <-ctx.Done():
		result["timedout"] = true
		log.Printf("longPollInvoice: reqId %s timed out with no settlement.\n", reqId)
	}
	log.Printf("longPollInvoice: reqId %s got down here.", reqId)

	sendJSON(w, result)
}

func monitorShutdown(conf *conf) {
	for {
		if _, err := os.Stat(conf.RestartFile); os.IsNotExist(err) {
			time.Sleep(5 * time.Second)
			continue
		}
		log.Printf("monitorShutdown: presence of restart file detected at %s, removing it and exiting.", conf.RestartFile)
		if err := os.Remove(conf.RestartFile); err != nil {
			log.Printf("monitorShudown: error trying to remove restart file.")
			panic("that is bad.")
		}
		// os.Exit(0)
		quitChan <- struct{}{}
	}
}
