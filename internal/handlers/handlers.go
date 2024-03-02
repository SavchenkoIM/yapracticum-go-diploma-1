package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
	"yapracticum-go-diploma-1/internal/config"
	"yapracticum-go-diploma-1/internal/storage"
)

type Handlers struct {
	Logger    *zap.Logger
	DBStorage *storage.Storage
	Cfg       config.Config
}

type UserRegisterStruct struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type WithdrawStruct struct {
	Order string           `json:"order"`
	Sum   *storage.Numeric `json:"sum"`
}

func (h *Handlers) GetBalance(w http.ResponseWriter, r *http.Request) {
	tokenID := r.Header.Get("LoggedUserId")
	balance, err := h.DBStorage.GetBalance(r.Context(), tokenID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	mJSON, err := json.Marshal(balance)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(mJSON)
}

func (h *Handlers) UserRegister(w http.ResponseWriter, r *http.Request) {

	bodyData := make([]byte, r.ContentLength)
	_, err := r.Body.Read(bodyData)
	if err == nil {
		r.Body.Close()
	}
	bb := bytes.NewBuffer(bodyData)
	r.Body = io.NopCloser(bb)

	var jsonData UserRegisterStruct
	err = json.Unmarshal(bodyData, &jsonData)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		h.Logger.Error(err.Error())
		return
	}

	err = h.DBStorage.UserRegister(r.Context(), jsonData.Login, jsonData.Password)
	if err != nil {
		h.Logger.Error(err.Error())
		if errors.Is(err, storage.ErrUserAlreadyExists) {
			w.WriteHeader(http.StatusConflict)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		return
	}

	h.UserLogin(w, r)
}

func (h *Handlers) UserLogin(w http.ResponseWriter, r *http.Request) {

	bodyData := make([]byte, r.ContentLength)
	r.Body.Read(bodyData)
	defer r.Body.Close()

	var jsonData UserRegisterStruct
	err := json.Unmarshal(bodyData, &jsonData)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		h.Logger.Error(err.Error())
		return
	}

	token, err := h.DBStorage.UserLogin(r.Context(), jsonData.Login, jsonData.Password)
	if err != nil {
		if errors.Is(err, storage.ErrUserAuthFailed) {
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		h.Logger.Error(err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Value:   token,
		Expires: time.Now().Add(storage.TokenExp).Add(time.Hour), // Cookie expires a hour after token
	})
	w.WriteHeader(http.StatusOK)

}

func (h *Handlers) OrderLoad(w http.ResponseWriter, r *http.Request) {
	tokenID := r.Header.Get("LoggedUserId")
	bodyData := make([]byte, r.ContentLength)
	r.Body.Read(bodyData)
	r.Body.Close()

	ordernum, err := strconv.ParseInt(string(bodyData), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = h.DBStorage.OrderAddNew(r.Context(), tokenID, strconv.Itoa(int(ordernum)))
	if err != nil {

		if errors.Is(err, storage.ErrOrderOtherUser) {
			w.WriteHeader(http.StatusConflict)
			return
		}

		if errors.Is(err, storage.ErrOrderAlreadyExists) {
			w.WriteHeader(http.StatusOK)
			return
		}

		if errors.Is(err, storage.ErrOrderLuhnCheckFailed) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handlers) UserCheckLoggedInHandler(w http.ResponseWriter, r *http.Request) {
	cSession, err := r.Cookie("session_token")
	if err != nil {
		w.Write([]byte("No session cookie"))
		return
	}
	login, err := h.DBStorage.UserCheckLoggedIn(cSession.Value)
	if err != nil {
		w.Write([]byte("Not logged in"))
		return
	}
	w.Write([]byte(fmt.Sprintf("Logged userID: %s", login)))
}

func (h *Handlers) OrderGetList(w http.ResponseWriter, r *http.Request) {
	tokenID := r.Header.Get("LoggedUserId")
	data, err := h.DBStorage.GetOrdersData(r.Context(), tokenID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if len(data.Orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	marshalled, err := json.Marshal(data.Orders)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.Logger.Sugar().Errorf(err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(marshalled)
}

func (h *Handlers) Withdraw(w http.ResponseWriter, r *http.Request) {
	tokenID := r.Header.Get("LoggedUserId")
	bodyData := make([]byte, r.ContentLength)
	r.Body.Read(bodyData)
	r.Body.Close()

	var parsedData WithdrawStruct
	err := json.Unmarshal(bodyData, &parsedData)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	h.Logger.Sugar().Infof("Withdraw request: %s", bodyData)

	digRe, _ := regexp.Compile(`^\d+$`)
	m := digRe.FindStringSubmatch(parsedData.Order)

	if m == nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	err = h.DBStorage.Withdraw(r.Context(), tokenID, parsedData.Order, *parsedData.Sum)
	if err != nil {
		if errors.Is(err, storage.ErrWithdrawNotEnough) {
			w.WriteHeader(http.StatusPaymentRequired)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}

func (h *Handlers) WithdrawGetList(w http.ResponseWriter, r *http.Request) {
	tokenID := r.Header.Get("LoggedUserId")
	data, err := h.DBStorage.GetWithdrawalsData(r.Context(), tokenID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if len(data.Withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	marshalled, err := json.Marshal(data.Withdrawals)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.Logger.Sugar().Errorf(err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(marshalled)
}
