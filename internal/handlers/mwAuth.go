package handlers

import (
	"net/http"
	"strings"
)

func userCheckLoggedIn(w http.ResponseWriter, r *http.Request) (string, bool) {
	cSession, err := r.Cookie("session_token")
	if err != nil {
		logger.Info(err.Error())
		return "", false
	}
	userID, err := dbStorage.UserCheckLoggedIn(cSession.Value)
	if err != nil {
		logger.Info(err.Error())
		return "", false
	}

	return userID, true
}

func CustomAuth(exclude ...string) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, v := range exclude {
				if strings.HasPrefix(r.RequestURI, v) {
					h.ServeHTTP(w, r)
					return
				}
			}

			userID, loggedIn := userCheckLoggedIn(w, r)
			if !loggedIn {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			r.Header.Set("LoggedUserID", userID)

			h.ServeHTTP(w, r)
		})
	}
}
