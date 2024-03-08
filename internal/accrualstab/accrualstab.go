package accrualstab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-redis/redis/v8"
	"net/http"
	"strconv"
	"sync"
	"time"
	"yapracticum-go-diploma-1/internal/storage"
)

////////////////////////
// TestServer
////////////////////////

type AccrualStab struct {
	http.Server
	mErrors       sync.Mutex
	testErrors    []error
	ordersDB      *OrdersDB
	notAvailUntil time.Time
	not429Until   time.Time
}

func NewAccrualStab(endpoint string, redisEndpoint string) *AccrualStab {
	ordersDB := NewOrdersDB(redisEndpoint)
	as := AccrualStab{testErrors: make([]error, 0), ordersDB: ordersDB}
	mux := chi.NewMux()
	mux.Get("/api/orders/{number}", as.GetOrderInfo)
	as.Server = http.Server{Addr: endpoint, Handler: mux}
	return &as
}

func (as *AccrualStab) Serve() error {
	go as.Processing()
	err := as.ListenAndServe()
	if err != nil {
		return err
	}
	return nil
}

func (as *AccrualStab) Error() error {
	return errors.Join(as.testErrors...)
}

//////////////////////
// Handler
//////////////////////

func (as *AccrualStab) GetOrderInfo(w http.ResponseWriter, r *http.Request) {

	orderNum := chi.URLParam(r, "number")

	if time.Now().Before(as.notAvailUntil) {
		w.Header().Add("Retry-After", "15")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Ya zhe skazal, 429!"))
		as.mErrors.Lock()
		defer as.mErrors.Unlock()
		as.testErrors = append(as.testErrors, fmt.Errorf("poll during requested timeout (order %s)", orderNum))
		return
	}

	if orderNum == "429" && time.Now().After(as.not429Until) {
		as.notAvailUntil = time.Now().Add(12 * time.Second)
		as.not429Until = time.Now().Add(60 * time.Second)
	}

	val, exist := as.ordersDB.Get(orderNum)
	var res storage.AccrualResponse
	if exist {
		res = storage.AccrualResponse{
			Order:   orderNum,
			Status:  val.Status,
			Accrual: val.Accrual,
		}
	} else {
		val := storage.Numeric(0)
		to := TestOrder{Status: "PROCESSING", AddedAt: time.Now(), Accrual: &val}
		as.ordersDB.Set(orderNum, to)
		res = storage.AccrualResponse{
			Order:  orderNum,
			Status: "REGISTERED",
		}
	}

	resMarsh, err := json.Marshal(res)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(resMarsh)

}

func (as *AccrualStab) Processing() {
	tmr := time.NewTicker(1 * time.Second)
	for range tmr.C {
		// For range instead select as govet requested
		//select {
		//case <-tmr.C:
		keys, err := as.ordersDB.cli.Keys(context.Background(), "*").Result()
		if err != nil {
			fmt.Println(err)
			continue
		}
		for _, iv := range keys {
			v, exist := as.ordersDB.Get(iv)
			if !exist {
				continue
			}
			d := time.Since(v.AddedAt)
			switch {
			case d < 3*time.Second:
			default:
				v.Status = "PROCESSED"
				acc, _ := strconv.Atoi(iv)
				val := storage.Numeric(acc)
				v.Accrual = &val
				as.ordersDB.Set(iv, v)
			}
		}
	}
	//}
}

/////////////////////////
// Orders DB
/////////////////////////

type TestOrder struct {
	Accrual *storage.Numeric `json:"accrual"`
	Status  string           `json:"status"`
	AddedAt time.Time        `json:"addedAt"`
}

func NewOrdersDB(endpoint string) *OrdersDB {
	return &OrdersDB{cli: redis.NewClient(&redis.Options{
		Addr:     endpoint,
		Password: "",
		DB:       0,
	})}
}

type OrdersDB struct {
	cli *redis.Client
}

func (odb *OrdersDB) Get(key string) (TestOrder, bool) {
	val, err := odb.cli.Get(context.Background(), key).Result()
	if err != nil {
		return TestOrder{}, false
	}
	var res TestOrder
	err = json.Unmarshal([]byte(val), &res)
	if err != nil {
		return TestOrder{}, false
	}
	return res, true
}

func (odb *OrdersDB) Set(key string, val TestOrder) {
	jsonm, err := json.Marshal(val)
	if err != nil {
		fmt.Println(err)
	}
	err = odb.cli.Set(context.Background(), key, jsonm, 0).Err()
	if err != nil {
		fmt.Println(err)
	}
}
