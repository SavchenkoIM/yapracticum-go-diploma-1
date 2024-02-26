package accrualstab

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
	"sync"
	"time"
	"yapracticum-go-diploma-1/internal/storage"
)

// TestServer

type AccrualStab struct {
	http.Server
	mErrors       sync.Mutex
	testErrors    []error
	ordersDB      *OrdersDB
	notAvailUntil time.Time
	not429Until   time.Time
}

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
			Accrual: &val.Accrual,
		}
	} else {
		to := TestOrder{Status: "PROCESSING", AddedAt: time.Now(), Accrual: 0}
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
		as.ordersDB.M.Lock()
		for k, v := range as.ordersDB.Orders {
			d := time.Since(v.AddedAt)
			switch {
			case d < 3*time.Second:
			default:
				v.Status = "PROCESSED"
				acc, _ := strconv.Atoi(k)
				v.Accrual = storage.Numeric(acc)
				as.ordersDB.Orders[k] = v
			}
		}
		as.ordersDB.M.Unlock()
	}
	//}
}

func NewAccrualStab(endpoint string) *AccrualStab {
	ordersDB := NewOrdersDB()
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

// TestOrder

type TestOrder struct {
	Accrual storage.Numeric
	Status  string
	AddedAt time.Time
}

// Orders DB

func NewOrdersDB() *OrdersDB {
	return &OrdersDB{Orders: make(map[string]TestOrder)}
}

type OrdersDB struct {
	M      sync.RWMutex
	Orders map[string]TestOrder
}

func (odb *OrdersDB) Get(key string) (TestOrder, bool) {
	odb.M.RLock()
	defer odb.M.RUnlock()
	order, exist := odb.Orders[key]
	return order, exist
}

func (odb *OrdersDB) Set(key string, val TestOrder) {
	odb.M.Lock()
	defer odb.M.Unlock()
	odb.Orders[key] = val
}
