package handlers

import (
	"net/http"
)

func (h *Handlers) Recoverer(hand http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				h.Logger.Sugar().Errorf("500: %v", err)
				return
			}
			hand.ServeHTTP(w, r)
		}()
	})
}
