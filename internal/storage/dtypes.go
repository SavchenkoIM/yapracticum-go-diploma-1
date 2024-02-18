package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Storager interface {
	UserRegister(context.Context, string, string) error
	UserLogin(context.Context, string, string) (string, error)
	UserCheckLoggedIn(string) (SessionInfo, error)
	OrderAddNew(context.Context, string, int) error
	GetOrdersData(context.Context, string) (OrdersInfo, error)
	GetUnhandledOrders(context.Context) (OrdersInfo, error)
	Withdraw(context.Context, string, int64, Numeric) error
	GetWithdrawalsData(context.Context, string) (WithdrawalsInfo, error)
	GetBalance(context.Context, pgx.Tx, string) (BalanceInfo, error)
	ApplyAccrualResponse(context.Context, AccrualResponse) error
}

//////////////////////////
// SessionInfo
//////////////////////////

var sessionInactiveTime time.Duration = 5 * time.Minute

type SessionInfo struct {
	UserName string
	userID   string
	expiry   time.Time
}

func (s SessionInfo) isExpired() bool {
	return s.expiry.Before(time.Now())
}

type SessionTokenData struct {
	TokenID string
	Expiry  time.Time
}

//////////////////////////
// SessionInfoMap
//////////////////////////

type SessionInfoMap struct {
	data  map[string]SessionInfo
	mutex sync.RWMutex
}

func NewSessionInfoMap() *SessionInfoMap {
	return &SessionInfoMap{data: make(map[string]SessionInfo)}
}
func (sim *SessionInfoMap) Set(key string, value SessionInfo) {
	sim.mutex.Lock()
	defer sim.mutex.Unlock()
	sim.data[key] = value
}
func (sim *SessionInfoMap) Delete(key string) {
	sim.mutex.Lock()
	defer sim.mutex.Unlock()
	delete(sim.data, key)
}
func (sim *SessionInfoMap) Get(key string) (SessionInfo, bool) {
	sim.mutex.RLock()
	defer sim.mutex.RUnlock()
	val, ok := sim.data[key]
	return val, ok
}

//////////////////////////
// Numeric: int64 with two last significant digits as "currency cents"
//////////////////////////

type Numeric int64

func (n *Numeric) String() string {
	return fmt.Sprintf("%d.%02d", *n/100, *n%100)
}

func (n *Numeric) FromString(text string) error {
	re, _ := regexp.Compile(`^(?P<dollar>\d+)(?P<cent>(.\d{2})?)$`)
	m := re.FindStringSubmatch(text)
	// m[0] - "10.23", m[1] - "10", m[2] = ".23"
	if m == nil {
		return errors.New("incorrect Numeric value")
	}

	dollar, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return err
	}

	var cent int64 = 0
	cent, _ = strconv.ParseInt(strings.Replace(m[2], ".", "", 1), 10, 64)

	*n = Numeric(dollar*100 + cent)
	return nil
}

func (n *Numeric) MarshalJSON() ([]byte, error) {
	return []byte(n.String()), nil
}

func (n *Numeric) UnmarshalJSON(data []byte) error {
	return n.FromString(string(data))
}

//////////////////////////
// Accrual response
//////////////////////////

type AccrualResponse struct {
	Order   string   `json:"order"`
	Status  string   `json:"status"`
	Accrual *Numeric `json:"accrual"`
}

//////////////////////////
// Withdrawal info
//////////////////////////

type BalanceInfo struct {
	Current   *Numeric `json:"current"`
	Withdrawn *Numeric `json:"withdrawn"`
}

type WithdrawalInfo struct {
	Order       string      `json:"order"`
	Sum         *Numeric    `json:"sum"`
	ProcessedAt RFC3339Time `json:"processed_at"`
}

type WithdrawalsInfo struct {
	Withdrawals []WithdrawalInfo
}

//////////////////////////
// Order info
//////////////////////////

const (
	StatusNew        = iota // заказ загружен, но ещё не попал в обработку
	StatusProcessing        // вознаграждение за заказ рассчитывается;
	StatusInvalid           // система расчёта вознаграждений отказала в расчёте;
	StatusProcessed         // данные по заказу проверены и информация о расчёте успешно получена.
)

const (
	OrdersRequestByUser = iota
	OrdersRequestByStatusUnhandled
)

type OrdersInfo struct {
	Orders []OrderInfo
}

type OrderInfo struct {
	User       string      `json:"-"`
	Number     string      `json:"number"`
	Status     OrderStatus `json:"status"`
	Accrual    *Numeric    `json:"accrual,omitempty"`
	UploadedAt RFC3339Time `json:"uploaded_at"`
}

// RFC3339 Time
type RFC3339Time time.Time

func (rfTime RFC3339Time) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(rfTime).Format(time.RFC3339) + `"`), nil
}

// Status code
type OrderStatus int64

func (os *OrderStatus) MarshalJSON() ([]byte, error) {
	sRepr := ""
	switch *os {
	case StatusNew:
		sRepr = "NEW"
	case StatusProcessing:
		sRepr = "PROCESSING"
	case StatusInvalid:
		sRepr = "INVALID"
	case StatusProcessed:
		sRepr = "PROCESSED"
	default:
		sRepr = "UNKNOWN!!!"
	}
	return []byte(`"` + sRepr + `"`), nil
}