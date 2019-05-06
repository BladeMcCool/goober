// recaptchahelper.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
)

type recaptchaHelper struct {
	SessMgr *sessionManager
}

func NewRecaptchaHelper(myConf *conf, sessMgr *sessionManager) *recaptchaHelper {
	return &recaptchaHelper{SessMgr: sessMgr}
}
func (rh *recaptchaHelper) BotCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//return result of previous check from this session, or perform the check if we havent.
		userSess := rh.SessMgr.GetSession(r)
		// if err != nil {
		// 	panic("cant read session ... very gay.")
		// }
		token := r.URL.Query().Get("t")
		ip, port, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			fmt.Printf("userip: %q is not IP:port\n", r.RemoteAddr)
		}
		_, _ = ip, port
		_ = userSess
		_ = token
	})
}

func (rh *recaptchaHelper) isUserReal(w http.ResponseWriter, r *http.Request) (bool, float64) {
	score := rh.UpdateRecaptchaScore(w, r)
	log.Printf("isUserReal: captcha score was %f\n", score)
	if score < 0.5 {
		fmt.Printf("isUserReal captcha score was too low.\n")
		return false, score
	}
	return true, score
}

func (rh *recaptchaHelper) UpdateRecaptchaScore(w http.ResponseWriter, r *http.Request) float64 {
	userSess := rh.SessMgr.GetSession(r)
	log.Printf("recaptchaHelper UpdateRecaptchaScore: this sess values at start: %#v\n", userSess.Values)

	token := r.URL.Query().Get("t")
	ip := r.Header.Get("X-Real-Ip")
	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	if ip == "" {
		ip = "1.2.3.4"
	}

	score := rh.GetRecaptchaScore(token, ip)
	userSess.Values["recaptcha-score"] = score
	userSess.Save(r, w)
	return score
}

func (rh *recaptchaHelper) GetRecaptchaScore(token string, ip string) float64 {
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

	// fmt.Println("response Status:", resp.Status)
	// fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	log.Printf("recapthca response (from google) Body: %s", string(body))

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
