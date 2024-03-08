package handlers

import (
	"net/http"
	"strings"
)

func (h *Handlers) userCheckLoggedIn(w http.ResponseWriter, r *http.Request) (string, bool) {
	cSession, err := r.Cookie("session_token")
	if err != nil {
		h.Logger.Info(err.Error())
		return "", false
	}
	userID, err := h.DBStorage.UserCheckLoggedIn(cSession.Value)
	if err != nil {
		h.Logger.Info(err.Error())
		return "", false
	}

	return userID, true
}

func (h *Handlers) CustomAuth(exclude ...string) func(http.Handler) http.Handler {
	return func(hand http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, v := range exclude {
				if strings.HasPrefix(r.RequestURI, v) {
					hand.ServeHTTP(w, r)
					return
				}
			}

			userID, loggedIn := h.userCheckLoggedIn(w, r)
			if !loggedIn {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			r.Header.Set("LoggedUserID", userID)

			hand.ServeHTTP(w, r)
		})
	}
}
