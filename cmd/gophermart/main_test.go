package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
	"yapracticum-go-diploma-1/internal/accrualpoll"
	"yapracticum-go-diploma-1/internal/accrualstab"
	"yapracticum-go-diploma-1/internal/config"
	"yapracticum-go-diploma-1/internal/handlers"
	"yapracticum-go-diploma-1/internal/storage"
	"yapracticum-go-diploma-1/internal/storage/testhelpers"
	"yapracticum-go-diploma-1/internal/utils"
)

func TestIter2Server(t *testing.T) {

	tests := []struct {
		testName       string
		method         string
		url            string
		body           string
		wantStatusCode int
		wantBody       string
		delayAfter     time.Duration
	}{
		{testName: "Reg user", method: http.MethodPost, url: "http://localhost:8080/api/user/register", body: `{"login":"TestLogin","password":"TestPassword"}`, wantStatusCode: http.StatusOK, wantBody: ""},
		{testName: "Log user", method: http.MethodPost, url: "http://localhost:8080/api/user/login", body: `{"login":"TestLogin","password":"TestPassword"}`, wantStatusCode: http.StatusOK, wantBody: ""},
		{testName: "Add order", method: http.MethodPost, url: "http://localhost:8080/api/user/orders", body: `780348943`, wantStatusCode: http.StatusAccepted, wantBody: ""},
		{testName: "Wait for accrual", method: http.MethodPost, url: "", body: ``, delayAfter: 8 * time.Second},
		{testName: "Check balance", method: http.MethodGet, url: "http://localhost:8080/api/user/balance", body: ``, wantStatusCode: http.StatusOK, wantBody: `{"current": 7803489.43,"withdrawn": 0}`},
		{testName: "Withdraw 100 points", method: http.MethodPost, url: "http://localhost:8080/api/user/balance/withdraw", body: `{"order": "2377225624","sum": 100}`, wantStatusCode: http.StatusOK, wantBody: ``},
		{testName: "ReCheck balance", method: http.MethodGet, url: "http://localhost:8080/api/user/balance", body: ``, wantStatusCode: http.StatusOK, wantBody: `{"current": 7803389.43,"withdrawn": 100}`},
		{testName: "Cause 429", method: http.MethodPost, url: "http://localhost:8080/api/user/orders", body: `429`, wantStatusCode: http.StatusAccepted, wantBody: "", delayAfter: 5 * time.Second},
		{testName: "Check 429", method: http.MethodGet, url: "http://localhost:8090/api/orders/780348943", body: ``, wantStatusCode: http.StatusTooManyRequests, wantBody: "", delayAfter: 10 * time.Second},
		{testName: "Check 429 over", method: http.MethodGet, url: "http://localhost:8090/api/orders/780348943", body: ``, wantStatusCode: http.StatusOK, wantBody: ``, delayAfter: 10 * time.Second},
		{testName: "Check accrual of 4.29", method: http.MethodGet, url: "http://localhost:8080/api/user/balance", body: ``, wantStatusCode: http.StatusOK, wantBody: `{"current": 7803393.72,"withdrawn": 100}`},
	}

	authCookie := ""

	///////////////////////
	// Setup database
	///////////////////////

	var err error
	storageContainer := testhelpers.NewTestDatabase(t)
	connstring := fmt.Sprintf("postgresql://%s:%d/postgres?user=postgres&password=postgres", storageContainer.Host(), storageContainer.Port(t))
	logger, err = zap.NewProduction()
	require.NoError(t, err)

	//////////////////////
	// Setup gophermart
	//////////////////////

	parentContext, cancel := context.WithCancel(context.Background())

	if logger, err = zap.NewProduction(); err != nil {
		panic(err)
	}

	cfg := config.Config{ConnString: connstring, UseLuna: false, Endpoint: "localhost:8080", AccrualAddress: "http://localhost:8090"}
	newOrdersCh := make(chan storage.OrderTag, 1000)
	dbStorage, err = storage.New(cfg, logger, newOrdersCh)
	if err != nil {
		panic(err.Error())
	}

	dbStorage.Init(parentContext)

	workersWg := sync.WaitGroup{}
	ccw := utils.NewCtxCancelWaiter(parentContext, 0)
	for i := 1; i <= 5; i++ {
		go accrualpoll.AccrualPollWorker(ccw, dbStorage, i, &workersWg, logger, cfg.AccrualAddress, newOrdersCh)
	}

	go accrualpoll.GetUnhandledOrders(parentContext, dbStorage, &workersWg, logger, newOrdersCh)

	h := handlers.Handlers{Logger: logger, DBStorage: dbStorage, Cfg: cfg}
	server := http.Server{Addr: cfg.Endpoint, Handler: handlers.GophermartRouter(h)}

	go shutdownSignal(parentContext, cancel, &workersWg, newOrdersCh, &server)

	//////////////////////
	// Setup accrual
	//////////////////////

	s := accrualstab.NewAccrualStab("localhost:8090")

	//////////////////////
	// Start services
	//////////////////////

	go func() {
		err := s.Serve()
		require.NoError(t, err)
	}()
	go func() {
		err := server.ListenAndServe()
		require.NoError(t, err)
	}()

	//////////////////////
	// RunTests
	//////////////////////

	cli := http.Client{}
	var res *http.Response
	var req *http.Request
	var body []byte

	for _, tt := range tests {

		t.Run(tt.testName, func(t *testing.T) {

			if tt.url != "" {

				req, err = http.NewRequest(tt.method, tt.url, bytes.NewBuffer([]byte(tt.body)))
				req.Header.Set("Cookie", authCookie)

				res, err = cli.Do(req)
				require.NoError(t, err)

				if err == nil {
					body, err = io.ReadAll(res.Body)
					if err != nil {
						body = []byte("")
					}
					_authCookie := res.Header.Get("Set-Cookie")
					if _authCookie != "" {
						authCookie = _authCookie
					}
				}
				res.Body.Close()

				println("Response Body: \"" + string(body) + "\"")

				assert.Equal(t, tt.wantStatusCode, res.StatusCode)
				if tt.wantBody != `` {
					assert.JSONEq(t, tt.wantBody, string(body))
				}
			}

			time.Sleep(tt.delayAfter)

		})
	}

	t.Run("Check no requests while 429 timeout", func(t *testing.T) {
		err = s.Error()
		require.Error(t, err)

		errText := `poll during requested timeout (order 780348943)
poll during requested timeout (order 429)`

		assert.Equal(t, err.Error(), errText)
	})
}