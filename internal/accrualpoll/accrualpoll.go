package accrualpoll

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"net/http"
	"strconv"
	"sync"
	"time"
	"yapracticum-go-diploma-1/internal/storage"
	"yapracticum-go-diploma-1/internal/utils"
)

type AccrualPollWorker struct {
	s               *storage.Storage
	wg              *sync.WaitGroup
	logger          *zap.Logger
	accrualAddress  string
	data            chan storage.OrderTag
	ccw             *utils.CtxCancelWaiter
	dbPollPeriod    time.Duration
	orderPollPeriod time.Duration
	lastDBPoll      time.Time
	lastDBPollM     *sync.RWMutex
}

func NewAccrualPollWorker(
	ccw *utils.CtxCancelWaiter,
	s *storage.Storage,
	wg *sync.WaitGroup,
	logger *zap.Logger,
	accrualAddress string,
	data chan storage.OrderTag) *AccrualPollWorker {
	if cap(data) < 10 {
		panic("Channel for processing orders must be buffered with capacity >= 10")
	}
	return &AccrualPollWorker{
		s:               s,
		wg:              wg,
		logger:          logger,
		accrualAddress:  accrualAddress,
		data:            data,
		ccw:             ccw,
		dbPollPeriod:    time.Minute,
		orderPollPeriod: 5 * time.Second,
		lastDBPoll:      time.Now().Add(-time.Second),
		lastDBPollM:     &sync.RWMutex{},
	}
}

func (apw *AccrualPollWorker) StartPoll(numWorkers int) {
	for i := 1; i <= numWorkers; i++ {
		go apw.DoWork(i)
	}
}

func (apw *AccrualPollWorker) DoWork(id int) {
	apw.wg.Add(1)
	apw.logger.Info(fmt.Sprintf("Accrual poll worker %d started", id))
	defer func() {
		apw.logger.Info(fmt.Sprintf("Accrual poll worker %d stopped", id))
		apw.wg.Done()
	}()

	var lastPoll time.Time

	for {

		if apw.ccw.Scan() != nil {
			return
		}
		//fmt.Printf("Do work, worker %d: %v\n", id, time.Now())

		select {
		case order := <-apw.data:

			// Will poll later
			if order.PollAfter.After(time.Now()) {
				apw.data <- order
				continue
			}
			// OrderTag Expired! (It's copy already pulled from database)
			apw.lastDBPollM.RLock()
			lastPoll = apw.lastDBPoll
			apw.lastDBPollM.RUnlock()
			if order.IssuedAt.Before(lastPoll) {
				apw.logger.Sugar().Infof("Skip order %s, Issued: %v, Now: %v", order.OrderNum, order.IssuedAt, time.Now())
				continue
			}

			apw.logger.Info(fmt.Sprintf("Worker %d. Accrual Request: %s/api/orders/%s", id, apw.accrualAddress, order.OrderNum))

			resp, err := http.Get(fmt.Sprintf("%s/api/orders/%s", apw.accrualAddress, order.OrderNum))
			if err != nil {
				apw.data <- storage.OrderTag{OrderNum: order.OrderNum, PollAfter: time.Now().Add(5 * time.Second)}
				continue
			}

			respData := make([]byte, resp.ContentLength)
			resp.Body.Read(respData)
			resp.Body.Close()

			apw.logger.Info(fmt.Sprintf("Worker %d. Accrual Response: %s, Status: %d", id, string(respData), resp.StatusCode))

			switch resp.StatusCode {
			case http.StatusOK:
				var respParsed storage.AccrualResponse
				err = json.Unmarshal(respData, &respParsed)
				if err != nil {
					apw.data <- storage.OrderTag{OrderNum: order.OrderNum, PollAfter: time.Now().Add(5 * time.Second)}
					continue
				}

				err = apw.s.ApplyAccrualResponse(apw.ccw.Ctx, respParsed)
				if err != nil {
					apw.logger.Error(err.Error())
				}

				if (respParsed.Status != "PROCESSED" && respParsed.Status != "INVALID") || err != nil {
					apw.data <- storage.OrderTag{OrderNum: order.OrderNum, PollAfter: time.Now().Add(apw.orderPollPeriod), IssuedAt: order.IssuedAt}
				}

			case http.StatusTooManyRequests:
				apw.data <- order
				raHeader := resp.Header.Get("Retry-After")
				retryTime, err := strconv.Atoi(raHeader)
				if err != nil {
					retryTime = 10
				}
				apw.ccw.SetTimeUntil(time.Now().Add(time.Duration(retryTime) * time.Second))

			default:
				apw.logger.Sugar().Errorf("Unexpected Accrual response code: %d, body: %s", resp.StatusCode, respData)
			}
		default:
			time.Sleep(1 * time.Millisecond)

		}
	}
}

func (apw *AccrualPollWorker) GetUnhandledOrders(ctx context.Context) {
	apw.wg.Add(1)
	apw.logger.Info("GetUnhandledOrders worker started")
	defer func() {
		apw.logger.Info("GetUnhandledOrders worker stopped")
		apw.wg.Done()
	}()

	var iTime time.Time

	ccw := utils.NewCtxCancelWaiter(ctx, apw.dbPollPeriod)
	for {
		if ccw.Scan() != nil {
			return
		}

		orders, err := apw.s.GetUnhandledOrders(ccw.Ctx)
		if err != nil {
			continue
		}

		apw.logger.Info("Pull orders from database")
		if l := len(orders.Orders); l > 0 {
			apw.logger.Sugar().Warnf("Found %d unhandled orders", l)
		}

		iTime = time.Now()
		apw.lastDBPollM.Lock()
		apw.lastDBPoll = iTime
		apw.lastDBPollM.Unlock()
		for _, v := range orders.Orders {
			v := v
			//select {
			//case
			apw.data <- storage.OrderTag{OrderNum: v.Number, PollAfter: iTime, IssuedAt: iTime}
			// default: Must push all data to channel. If not enough time, service overloaded.
			//}
		}
	}

}
