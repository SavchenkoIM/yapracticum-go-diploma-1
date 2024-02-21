package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"
	"strconv"
	"time"
	"yapracticum-go-diploma-1/internal/config"
	"yapracticum-go-diploma-1/internal/storage"
)

var logger *zap.Logger
var dbStorage *storage.Storage
var cfg config.Config

type UserRegisterStruct struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type WithdrawStruct struct {
	Order string           `json:"order"`
	Sum   *storage.Numeric `json:"sum"`
}

func Init(pLogger *zap.Logger, pStorage *storage.Storage, config config.Config) {
	logger = pLogger
	dbStorage = pStorage
	cfg = config
}

func GetBalance(w http.ResponseWriter, r *http.Request) {
	tokenID := r.Header.Get("LoggedUserId")
	balance, err := dbStorage.GetBalance(r.Context(), nil, tokenID)
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

func UserRegister(w http.ResponseWriter, r *http.Request) {

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
		logger.Error(err.Error())
		return
	}

	err = dbStorage.UserRegister(r.Context(), jsonData.Login, jsonData.Password)
	if err != nil {
		logger.Error(err.Error())
		if errors.Is(err, storage.ErrUserAlreadyExists) {
			w.WriteHeader(http.StatusConflict)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		return
	}

	UserLogin(w, r)
}

func UserLogin(w http.ResponseWriter, r *http.Request) {

	bodyData := make([]byte, r.ContentLength)
	r.Body.Read(bodyData)
	defer r.Body.Close()

	var jsonData UserRegisterStruct
	err := json.Unmarshal(bodyData, &jsonData)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error(err.Error())
		return
	}

	token, err := dbStorage.UserLogin(r.Context(), jsonData.Login, jsonData.Password)
	if err != nil {
		if errors.Is(err, storage.ErrUserAuthFailed) {
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		logger.Error(err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Value:   token,
		Expires: time.Date(2050, 1, 1, 0, 0, 0, 0, time.Local),
	})

}

func OrderLoad(w http.ResponseWriter, r *http.Request) {
	tokenID := r.Header.Get("LoggedUserId")
	bodyData := make([]byte, r.ContentLength)
	r.Body.Read(bodyData)
	r.Body.Close()

	ordernum, err := strconv.ParseInt(string(bodyData), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !LunaValid(int(ordernum)) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	err = dbStorage.OrderAddNew(r.Context(), tokenID, int(ordernum))
	if err != nil {

		if errors.Is(err, storage.ErrOrderOtherUser) {
			w.WriteHeader(http.StatusConflict)
			return
		}

		if errors.Is(err, storage.ErrOrderAlreadyExists) {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func UserCheckLoggedInHandler(w http.ResponseWriter, r *http.Request) {
	cSession, err := r.Cookie("session_token")
	if err != nil {
		w.Write([]byte("No session cookie"))
		return
	}
	login, err := dbStorage.UserCheckLoggedIn(cSession.Value)
	if err != nil {
		w.Write([]byte("Not logged in"))
		return
	}
	w.Write([]byte(fmt.Sprintf("Logged userID: %s", login)))
}

func OrderGetList(w http.ResponseWriter, r *http.Request) {
	tokenID := r.Header.Get("LoggedUserId")
	data, err := dbStorage.GetOrdersData(r.Context(), tokenID)
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
		logger.Sugar().Errorf(err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(marshalled)
}

func Withdraw(w http.ResponseWriter, r *http.Request) {
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

	logger.Sugar().Infof("Withdraw request: %s", bodyData)

	orderNum, err := strconv.ParseInt(parsedData.Order, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	err = dbStorage.Withdraw(r.Context(), tokenID, orderNum, *parsedData.Sum)
	if err != nil {
		if errors.Is(err, storage.ErrWithdrawNotEnough) {
			w.WriteHeader(http.StatusPaymentRequired)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}

func WithdrawGetList(w http.ResponseWriter, r *http.Request) {
	tokenID := r.Header.Get("LoggedUserId")
	data, err := dbStorage.GetWithdrawalsData(r.Context(), tokenID)
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
		logger.Sugar().Errorf(err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(marshalled)
}
