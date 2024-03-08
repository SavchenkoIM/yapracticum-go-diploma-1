package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io"
	"net/http"
	"strconv"
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

func createTestLogger() *zap.Logger {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) { enc.AppendString(t.Format("15:04:05.000")) }

	conf := zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.InfoLevel),
		Development:       true,
		DisableCaller:     true,
		DisableStacktrace: true,
		Sampling:          nil,
		Encoding:          "console",
		EncoderConfig:     encoderCfg,
		OutputPaths: []string{
			"stderr",
		},
		ErrorOutputPaths: []string{
			"stderr",
		},
	}

	return zap.Must(conf.Build())
}

func TestComplex(t *testing.T) {

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
	logger = createTestLogger()
	require.NoError(t, err)

	storageContainerRedis := testhelpers.NewTestRedis(t)
	endpRedis := fmt.Sprintf("%s:%d", storageContainerRedis.Host(), storageContainerRedis.Port(t))

	//////////////////////
	// Setup gophermart
	//////////////////////

	parentContext, cancel := context.WithCancel(context.Background())

	//if logger, err = zap.NewProduction(); err != nil { panic(err) }

	cfg := config.Config{ConnString: connstring, UseLuhn: false, Endpoint: "localhost:8080", AccrualAddress: "http://localhost:8090"}
	newOrdersCh := make(chan storage.OrderTag, 300)
	dbStorage, err = storage.New(cfg, logger, newOrdersCh)
	if err != nil {
		panic(err.Error())
	}

	dbStorage.Init(parentContext)

	workersWg := sync.WaitGroup{}
	ccw := utils.NewCtxCancelWaiter(parentContext, 0)
	accrualPoll := accrualpoll.NewAccrualPollWorker(ccw, dbStorage, &workersWg, logger, cfg.AccrualAddress, newOrdersCh)
	accrualPoll.StartPoll(5)
	go accrualPoll.GetUnhandledOrders(parentContext)

	h := handlers.Handlers{Logger: logger, DBStorage: dbStorage, Cfg: cfg}
	server := http.Server{Addr: cfg.Endpoint, Handler: handlers.GophermartRouter(h)}

	go shutdownSignal(parentContext, cancel, &workersWg, newOrdersCh, &server)

	//////////////////////
	// Setup accrual
	//////////////////////

	s := accrualstab.NewAccrualStab("localhost:8090", endpRedis)

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
	//var res *http.Response
	var req *http.Request
	var body []byte

	for _, tt := range tests {

		t.Run(tt.testName, func(t *testing.T) {

			if tt.url != "" {

				req, err = http.NewRequest(tt.method, tt.url, bytes.NewBuffer([]byte(tt.body)))
				req.Header.Set("Cookie", authCookie)

				body = []byte("")
				res, err := cli.Do(req)

				if err == nil {
					body, err = io.ReadAll(res.Body)
					res.Body.Close()
					require.NoError(t, err)

					_authCookie := res.Header.Get("Set-Cookie")
					if _authCookie != "" {
						authCookie = _authCookie
					}
				}
				require.NoError(t, err)

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

		errText := []string{`poll during requested timeout (order 780348943)
poll during requested timeout (order 429)`, `poll during requested timeout (order 429)
poll during requested timeout (order 780348943)`}

		assert.Contains(t, errText, err.Error())
	})

	t.Run("500 orders at once", func(t *testing.T) {

		for i := 2000; i <= 2500; i++ {
			//time.Sleep(10 * time.Millisecond)
			go PlaceOrder(i, authCookie)
		}

		time.Sleep(45 * time.Second)

		req, _ = http.NewRequest(http.MethodGet, "http://localhost:8080/api/user/balance", nil)
		req.Header.Set("Cookie", authCookie)
		res, err := cli.Do(req)
		require.NoError(t, err)
		if err == nil {
			body, err = io.ReadAll(res.Body)
			res.Body.Close()
			require.NoError(t, err)
		}

		assert.JSONEq(t, `{"current":7814666.22,"withdrawn":100.00}`, string(body))
	})
}

func PlaceOrder(i int, authCookie string) {
	success := false
	for !success {
		cli := http.Client{}
		req, _ := http.NewRequest(http.MethodPost, "http://localhost:8080/api/user/orders", bytes.NewBuffer([]byte(strconv.Itoa(i))))
		req.Header.Set("Cookie", authCookie)
		res, err := cli.Do(req)
		if err == nil {
			res.Body.Close()
			success = true
		} else {
			time.Sleep(10 * time.Millisecond)
		}
	}
}
