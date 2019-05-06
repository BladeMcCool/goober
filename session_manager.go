//session manager
package main

import (
	"net/http"
	"github.com/gorilla/sessions"
)

// var sessMgr sessionManager
// func init() {
// 	sessStoar =  //these should be random and not saved in this file. oh well. see docs for more info.
// }

type sessionManager struct {
	SessStoar *sessions.CookieStore
}

// func (sm *sessionManager) Init(myConf bool) {
// 	sessMgr = &sessionManager{
// 		SessStoar: sessions.NewCookieStore([]byte(myConf.SessAuthKey), []byte(myConf.SessCipher))
// 	}
// }
func NewSessMgr(myConf *conf) *sessionManager {
	return &sessionManager{
		SessStoar: sessions.NewCookieStore([]byte(myConf.SessAuthKey), []byte(myConf.SessCipher)),
	}
}

func (sm *sessionManager) GetSession(r *http.Request) *sessions.Session {
	userSess, err := sm.SessStoar.Get(r, "chws-session")
	if err != nil {
		panic("cant read session ... very gay.")
	}
	return userSess
}
