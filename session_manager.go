//session manager
package main

import (
	"net/http"

	"github.com/gorilla/sessions"
)

type sessionManager struct {
	SessStoar *sessions.CookieStore
}

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
