package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os/user"
	"path"
	"sync"
	"time"

	_ "github.com/kr/pretty"

	"github.com/davecgh/go-spew/spew"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"

	"encoding/json"
	"net"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/syntaqx/echo-middleware/requestid"
)

var sessStoar *sessions.CookieStore
var recaptchaSecret, authKey, encryptKey string
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
}

func (c *conf) getConf() *conf {
	yamlFile, err := ioutil.ReadFile("goober.conf.yaml")
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
	}
	err = yaml.Unmarshal(yamlFile, c)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
	return c
}

var myConf conf

// var lnConn *grpc.ClientConn
var lnClient lnrpc.LightningClient

func getInvoiceFromLND(sats int64, memo string) *lnrpc.AddInvoiceResponse {
	log.Printf("getInvoiceFromLND: sats %d, memo %s\n", sats, memo)
	ctx := context.Background()
	//--------------
	// see example in https://github.com/michael1011/lightningtip/blob/master/backends/lnd.go
	// also examples in https://github.com/lightningnetwork/lnd/blob/master/lnd_test.go
	// var invoice *lnrpc.AddInvoiceResponse
	addInvoiceResp, err := lnClient.AddInvoice(ctx, &lnrpc.Invoice{
		Memo:   memo,
		Value:  sats,
		Expiry: 36000, //3600 is default.
	})

	if err != nil {
		panic(err)
	}
	log.Printf("getInvoiceFromLND teh AddInvoiceResponse: %#v\n", addInvoiceResp)
	// return invoice.PaymentRequest
	return addInvoiceResp
}
func lookupInvoiceFromLND(rhash string) *lnrpc.Invoice {
	ctx := context.Background()
	invoice, _ := lnClient.LookupInvoice(ctx, &lnrpc.PaymentHash{RHashStr: rhash})
	// if err.
	// if err != nil {
	// 	panic(err)
	// }
	if invoice == nil {
		invoice = &lnrpc.Invoice{}
	}
	// log.Printf("lookupInvoiceFromLND: teh invoice: %#v\n", invoice)
	// return invoice.PaymentRequest
	return invoice
}

type sessionManager struct {
	RhashMu          sync.Mutex
	RhashSettlements map[string](map[string]chan struct{})
}

func (sm *sessionManager) BotCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//return result of previous check from this session, or perform the check if we havent.
		userSess, err := sessStoar.Get(r, "ghey-sess")
		if err != nil {
			panic("cant read session ... very gay.")
		}
		token := r.URL.Query().Get("t")
		ip, port, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			fmt.Printf("userip: %q is not IP:port\n", r.RemoteAddr)
		}
		_, _ = ip, port
		_ = userSess
		_ = token

		// fmt.Printf("load3d this sess values: %#v\n", userSess.Values)

		// token := r.Header.Get("X-Session-Token")

		// if user, found := amw.tokenUsers[token]; found {
		//     // We found the token in our map
		//     log.Printf("Authenticated user %s\n", user)
		//     next.ServeHTTP(w, r)
		// } else {
		//     http.Error(w, "Forbidden", http.StatusForbidden)
		// }
	})
}

func (sm *sessionManager) GetSession(r *http.Request) *sessions.Session {
	userSess, err := sessStoar.Get(r, "chws-session")
	if err != nil {
		panic("cant read session ... very gay.")
	}
	return userSess
}

func (sm *sessionManager) UpdateRecaptchaScore(w http.ResponseWriter, r *http.Request) float64 {
	userSess := sm.GetSession(r)
	log.Printf("sessionManager Update: this sess values at start: %#v\n", userSess.Values)

	token := r.URL.Query().Get("t")
	ip := r.Header.Get("X-Real-Ip")
	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	if ip == "" {
		ip = "1.2.3.4"
	}

	score := sm.GetRecaptchaScore(token, ip)
	userSess.Values["recaptcha-score"] = score
	userSess.Save(r, w)
	return score
}

func (sm *sessionManager) GetRecaptchaScore(token string, ip string) float64 {
	// fmt.Printf("said hello:'%s'\n", message)

	aurl := "https://www.google.com/recaptcha/api/siteverify"
	// fmt.Println("aURL:>", aurl)

	// type verifyReq struct {
	// 	Secret   string `json:"secret"`
	// 	Response string `json:"response"`
	// 	Remoteip string `json:"remoteip"`
	// }
	// vreq := &verifyReq{
	// 	Secret:   "xxxxxx",
	// 	Response: message,
	// 	Remoteip: ip,
	// }
	// vreqSer, err := json.Marshal(vreq)
	// if err != nil {
	// 	fmt.Println(err.Error())
	// }

	//https://gist.github.com/slv922/fc88ca8ce52d2b46f27df95c86800b8b
	form := url.Values{
		"secret":   {myConf.RecaptchaSecret},
		"response": {token},
		"remoteip": {ip},
	}
	formEnc := form.Encode()
	sendBody := bytes.NewBufferString(formEnc)
	// fmt.Println("gonna send this: " + string(vreqSer))
	fmt.Println("gonna send this: " + formEnc)

	resp, err := http.Post(aurl, "application/x-www-form-urlencoded", sendBody)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))

	//what did we get
	var dat map[string]interface{}
	if err := json.Unmarshal(body, &dat); err != nil {
		panic(err)
	}
	score := 0.0
	if dat["success"] == nil || dat["success"].(bool) != true {
		fmt.Println("not success. problem talking to recaptcha??")
		return score
	}
	if dat["score"] != nil {
		score = dat["score"].(float64)
		fmt.Printf("scoar lulz: %f\n", score)
	}
	return score
}

func (sm *sessionManager) MonitorInvoices() {
	// sm.RhashSettlements = map[string]chan struct{}{}
	sm.RhashSettlements = map[string](map[string]chan struct{}){}
	ctx := context.Background()
	log.Printf("MonitorInvoices startup")
	in := &lnrpc.InvoiceSubscription{}
	subscribeClient, err := lnClient.SubscribeInvoices(ctx, in)
	if err != nil {
		panic(err)
	}
	for {
		wot, err := subscribeClient.Recv()
		rhash := hex.EncodeToString(wot.RHash)
		// log.Printf("MonitorInvoices: got invoice %# v", pretty.Formatter(wot))

		if wot.State != lnrpc.Invoice_SETTLED {
			continue
		}
		log.Printf("MonitorInvoices: got invoice settlement for %s\n", rhash)

		// if sm.RhashSettlements[rhash] == nil || len(sm.RhashSettlements[rhash]) == 0 {
		// 	//a long polling channel reader would have created the channel about the invoice it was interested in knowing was paid. no channel, no interest.
		// 	continue
		// }
		if sm.RhashSettlements[rhash] == nil {
			log.Printf("MonitorInvoices: there is nothing defined under rhash settlements map for %s\n", rhash)
			//a long polling channel reader would have created the channel about the invoice it was interested in knowing was paid. no channel, no interest.
			continue
		}
		// if len(sm.RhashSettlements[rhash]) > 0 {
		// 	//hrm i dont think this can actually happen.
		// 	continue
		// }
		// sm.RhashSettlements[rhash] = make(chan struct{}, 1)
		log.Printf("MonitorInvoices: there are %d requests wanting to know about settlement of %s\n", len(sm.RhashSettlements[rhash]), rhash)
		for reqId := range sm.RhashSettlements[rhash] {
			sm.RhashSettlements[rhash][reqId] <- struct{}{}
			log.Printf("MonitorInvoices: so, umm, we just put something in the channel for reqid %s to know about settlement of %s\n", reqId, rhash)
		}
		if err != nil {
			panic(err)
		}
		// log.Printf("MonitorInvoices: got invoice %# v", pretty.Formatter(wot))
	}

}

var sessMgr = &sessionManager{}

func init() {
	rand.Seed(time.Now().UnixNano())

	myConf.getConf()
	fmt.Printf("read config: %#vn\n", myConf)
	sessStoar = sessions.NewCookieStore([]byte(myConf.SessAuthKey), []byte(myConf.SessCipher)) //these should be random and not saved in this file. oh well. see docs for more info.

	usr, err := user.Current()
	if err != nil {
		fmt.Println("Cannot get current user:", err)
		return
	}

	fmt.Println("The user home directory: " + usr.HomeDir)
	tlsCertPath := path.Join(usr.HomeDir, ".lnd/tls.cert")
	macaroonPath := path.Join(usr.HomeDir, ".lnd/data/chain/bitcoin/mainnet/admin.macaroon")
	tlsCreds, err := credentials.NewClientTLSFromFile(tlsCertPath, "")
	if err != nil {
		fmt.Println("Cannot get node tls credentials", err)
		return
	}

	macaroonBytes, err := ioutil.ReadFile(macaroonPath)
	if err != nil {
		fmt.Println("Cannot read macaroon file", err)
		return
	}

	mac := &macaroon.Macaroon{}
	if err = mac.UnmarshalBinary(macaroonBytes); err != nil {
		fmt.Println("Cannot unmarshal macaroon", err)
		return
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithBlock(),
		grpc.WithPerRPCCredentials(macaroons.NewMacaroonCredential(mac)),
	}

	conn, err := grpc.Dial("localhost:10009", opts...)
	// lnConn = conn
	if err != nil {
		fmt.Println("cannot dial to lnd", err)
		return
	}
	client := lnrpc.NewLightningClient(conn)
	lnClient = client

	ctx := context.Background()
	getInfoResp, err := client.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		fmt.Println("Cannot get info from node:", err)
		return
	}

	var funBoi = &lnrpc.ListChannelsRequest{}
	getChanResp, err := client.ListChannels(ctx, funBoi)
	if err != nil {
		fmt.Println("Cannot get chan list from node:", err)
		return
	}

	fmt.Printf("%#v \n----\n", []*lnrpc.GetInfoResponse{getInfoResp, getInfoResp})
	spew.Dump(getInfoResp)
	spew.Dump(getChanResp)

	go sessMgr.MonitorInvoices()
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

func captchaScoreHighEnough(w http.ResponseWriter, r *http.Request) (bool, float64) {
	score := sessMgr.UpdateRecaptchaScore(w, r)

	// userSess.Values["homo-code"] = token
	// userSess.Values["homo-factor"] = score
	log.Printf("captcha score was %f\n", score)

	if score < 0.5 {
		// ded(w)
		fmt.Printf("captcha score was too low.\n")
		// w.WriteHeader(http.StatusNoContent)
		return false, score
	}
	return true, score
}

func getInvoiceForm(w http.ResponseWriter, r *http.Request) {
	if pass, _ := captchaScoreHighEnough(w, r); !pass {
		w.Write([]byte(""))
		return
	}

	// type verifyReq struct {
	// 	Secret   string `json:"secret"`
	// 	Response string `json:"response"`
	// 	Remoteip string `json:"remoteip"`
	// }
	// vreq := &verifyReq{
	// 	Secret:   "xxxxxx",
	// 	Response: message,
	// 	Remoteip: ip,
	// }
	// vreqSer, err := json.Marshal(true)
	// if err != nil {
	// 	fmt.Println(err.Error())
	// }
	log.Println("allow form")
	userSess := sessMgr.GetSession(r)
	// score := userSess.Values["recaptcha-score"].(float64)
	rando := RandString(32)
	userSess.Values["authtoken"] = rando
	userSess.Save(r, w)
	log.Printf("getInvoiceForm: session data currently: %#v", userSess.Values)

	resp, err := json.Marshal([]interface{}{true, rando})
	if err != nil {
		fmt.Println(err.Error())
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	// w.Write([]byte("u win, maybe this will be a bool that allows the page to show the form. or some secret we make and put in the session that we can check later. ur score was: " + fmt.Sprintf("%f", score)))
	w.Write(resp)
}

func getInvoice(w http.ResponseWriter, r *http.Request) {

	// paramsUrlEnc := r.URL.Query().Get("p")
	var params map[string]interface{}
	err := json.Unmarshal([]byte(r.URL.Query().Get("p")), &params)
	if err != nil {
		panic(err)
	}
	expectToken, haveExpectToken := sessMgr.GetSession(r).Values["authtoken"].(string)

	// log.Println("DEBUG DELETE THOSE NEXT TWO LINES")
	// expectToken = "FAKE-AUTH"
	// haveExpectToken = true
	// log.Println("DEBUG DELETE THOSE ABOVE TWO LINES")

	gotToken, haveParamToken := params["authtoken"].(string)
	amountFloat, _ := params["amount"].(float64)
	earmark, _ := params["earmark"].(string)
	amount := int64(amountFloat)
	if amount == 0 {
		amount = 1
	}
	// log.Printf("fuckin wot: %#v, %d, %s\n", params, amount, earmark)
	// log.Println(reflect.TypeOf(params["amount"]))
	// panic("nibba")
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
	if pass, score = captchaScoreHighEnough(w, r); !pass {
		w.Write([]byte(""))
		return
	}

	memo := "CHWS Donation"
	if earmark != "" {
		memo = memo + ", Earmarked for " + earmark
	}
	addInvoiceResp := getInvoiceFromLND(amount, memo)
	sess := sessMgr.GetSession(r)
	sess.Values["invoice-payreq"] = addInvoiceResp.PaymentRequest
	sess.Values["invoice-rhash"] = hex.EncodeToString(addInvoiceResp.RHash)
	sess.Save(r, w)

	log.Printf("clearly had a good enough score: %f\n", score)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	// w.Write([]byte("u win, maybe this will be a bool that allows the page to show the form. or some secret we make and put in the session that we can check later. ur score was: " + fmt.Sprintf("%f", score)))
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

	settled, expired := getInvoiceStatus(rhash)
	if expired || settled {
		payreq = ""
		delete(sess.Values, "invoice-rhash")
		delete(sess.Values, "invoice-payreq")
		sess.Save(r, w)
	}

	w.Write([]byte(payreq))
}

func getInvoiceStatus(rhash string) (settled bool, expired bool) {

	invoice := lookupInvoiceFromLND(rhash)
	settled = (invoice.GetState() == lnrpc.Invoice_SETTLED)
	expired = false
	nowsec := time.Now().UnixNano() / int64(time.Second)
	created := invoice.GetCreationDate()
	expiry := invoice.GetExpiry()
	age := nowsec - created
	expiretime := created + expiry
	if nowsec > expiretime {
		expired = true
	}
	log.Printf("i think time is now %d, invoice creationdate of %d, making it %d seconds old, it has expiry of %d sec aka at %d, so is it expired? %t\n", nowsec, created, age, expiry, expiretime, expired)

	return settled, expired
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

	settled, expired := getInvoiceStatus(rhash)
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
	log.Printf("sendJSON: about to send this:\n%s\n", rStr)
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
	// resultChan := make(chan struct{})
	sessMgr.RhashMu.Lock()
	if sessMgr.RhashSettlements[rhash] == nil {
		// sessMgr.RhashSettlements[rhash] = make(chan struct{}, 1)
		sessMgr.RhashSettlements[rhash] = map[string]chan struct{}{}
	}
	sessMgr.RhashSettlements[rhash][reqId] = make(chan struct{}, 1)
	log.Printf("longPollInvoice: set up a spot for req %s to find out about settlement of %s\n", reqId, rhash)
	sessMgr.RhashMu.Unlock()

	// resultChan := sessMgr.RhashSettlements[rhash]
	// var result string
	defer cancel()
	// go func(resultChan chan<- string) {
	// timedout := false
	// _ = timedout
	closedNotify := w.(http.CloseNotifier).CloseNotify()
	// go func() {
	// 	// time.Sleep(298 * time.Second)
	// 	// resultChan <- "test restuls"
	// 	attempt := 0
	// 	for {
	// 		attempt++
	// 		if timedout {
	// 			break
	// 		}
	// 		select {
	// 		case <-closedNotify:
	// 			// if httpClosed {
	// 			log.Printf("methinks client closed connection, so lets stop polling.")
	// 			resultChan <- struct{}{}
	// 			return
	// 			//break
	// 			// }
	// 		default:
	// 			//keep running the loop.
	// 		}
	// 		log.Printf("about to do attempt %d to get invoicestatus of rhash %s\n", attempt, rhash)
	// 		settled, expired := getInvoiceStatus(rhash)

	// 		if expired {
	// 			result["expired"] = true
	// 		}
	// 		if settled {
	// 			result["settled"] = true
	// 		}

	// 		if expired || settled {
	// 			// sess.Values["invoice-rhash"]
	// 			delete(sess.Values, "invoice-rhash")
	// 			delete(sess.Values, "invoice-payreq")
	// 			sess.Save(r, w)

	// 			resultChan <- struct{}{}
	// 			return
	// 		}

	// 		time.Sleep(1 * time.Second)
	// 		// panic("reread this make sure it makes sense")
	// 	}

	// 	// }

	// }()

	select {
	case <-ctx.Done():
		result["timedout"] = true
		log.Printf("longPollInvoice: reqId %s timed out with no settlement.\n", reqId)
		// timedout = true
		break
	case <-sessMgr.RhashSettlements[rhash][reqId]:
		result["gotresult"] = true
		result["settled"] = true
		log.Printf("longPollInvoice: reqId %s got a result like: %#v\n", reqId, result)
		break
	case <-closedNotify:
		log.Printf("longPollInvoice: reqId %s client closed connection:\n", reqId)
		return
	}
	log.Printf("longPollInvoice: reqId %s got down here.", reqId)

	sendJSON(w, result)
}

// 	w.WriteHeader(http.StatusNoContent)
// 	w.WriteHeader(http.StatusNoContent)
// 	w.WriteHeader(http.StatusNoContent)
// func ded(w http.ResponseWriter) {

// func ded(w http.ResponseWriter) {
// 	fmt.Printf("mcFail\n")
// 	w.WriteHeader(http.StatusNoContent)
// 	w.Write([]byte("204 - T-gay"))
// }v
// func ded(w http.ResponseWriter) {
// func ded(w http.ResponseWriter) {
// func ded(w http.ResponseWriter) {
// 	if err != nil {
// 		panic("cant read session ... very gay.")
// 	}
// 	fmt.Printf("loaded this sess data: %#v\n", userSess.Values)

// 	var savedScore float64
// 	if userSess.Values["homo-factor"] != nil {
// 		savedScore = userSess.Values["homo-factor"].(float64)
// 	} else {
// 		savedScore = 0.012345
// 	}
// 	w.Write([]byte("bye shitlord " + fmt.Sprintf("%f", savedScore)))

// }
