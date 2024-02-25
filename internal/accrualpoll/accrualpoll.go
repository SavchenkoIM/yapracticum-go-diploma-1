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

func AccrualPollWorker(ccw *utils.CtxCancelWaiter, s *storage.Storage, id int, wg *sync.WaitGroup, logger *zap.Logger, accrualAddress string, data chan storage.OrderTag) {
	wg.Add(1)
	logger.Info(fmt.Sprintf("Accrual poll worker %d started", id))
	defer func() {
		logger.Info(fmt.Sprintf("Accrual poll worker %d stopped", id))
		wg.Done()
	}()

	for {

		if ccw.Scan() != nil {
			return
		}
		//fmt.Printf("Do work, worker %d: %v\n", id, time.Now())

		select {
		case order := <-data:

			if order.PollAfter.After(time.Now()) {
				data <- order
				continue
			}

			logger.Info(fmt.Sprintf("Worker %d. Accrual Request: %s/api/orders/%s", id, accrualAddress, order.OrderNum))

			resp, err := http.Get(fmt.Sprintf("%s/api/orders/%s", accrualAddress, order.OrderNum))
			if err != nil {
				data <- storage.OrderTag{OrderNum: order.OrderNum, PollAfter: time.Now().Add(5 * time.Second)}
				continue
			}

			respData := make([]byte, resp.ContentLength)
			resp.Body.Read(respData)
			resp.Body.Close()

			logger.Info(fmt.Sprintf("Worker %d. Accrual Response: %s, Status: %d", id, string(respData), resp.StatusCode))

			switch resp.StatusCode {
			case http.StatusOK:
				var respParsed storage.AccrualResponse
				err = json.Unmarshal(respData, &respParsed)
				if err != nil {
					data <- storage.OrderTag{OrderNum: order.OrderNum, PollAfter: time.Now().Add(5 * time.Second)}
					continue
				}

				err = s.ApplyAccrualResponse(ccw.Ctx, respParsed)
				if err != nil {
					logger.Error(err.Error())
				}

				if (respParsed.Status != "PROCESSED" && respParsed.Status != "INVALID") || err != nil {
					data <- storage.OrderTag{OrderNum: order.OrderNum, PollAfter: time.Now().Add(5 * time.Second)}
				}

			case http.StatusTooManyRequests:
				data <- order
				raHeader := resp.Header.Get("Retry-After")
				retryTime, err := strconv.Atoi(raHeader)
				if err != nil {
					retryTime = 10
				}
				ccw.SetTimeUntil(time.Now().Add(time.Duration(retryTime) * time.Second))
			}
		default:
			time.Sleep(250 * time.Millisecond)

		}
	}
}

func GetUnhandledOrders(ctx context.Context, s *storage.Storage, wg *sync.WaitGroup, logger *zap.Logger, data chan storage.OrderTag) {
	wg.Add(1)
	logger.Info(fmt.Sprintf("GetUnhandledOrders worker started"))
	defer func() {
		logger.Info(fmt.Sprintf("GetUnhandledOrders worker stopped"))
		wg.Done()
	}()

	ccw := utils.NewCtxCancelWaiter(ctx, 30*time.Minute)
	for {
		if ccw.Scan() != nil {
			return
		}

		orders, err := s.GetUnhandledOrders(ccw.Ctx)
		if err != nil {
			continue
		}

		if l := len(orders.Orders); l > 0 {
			logger.Sugar().Warnf("Found %d unhandled orders", l)
		}

		for _, v := range orders.Orders {
			v := v
			select {
			case data <- storage.OrderTag{OrderNum: v.Number, PollAfter: time.Now()}:
			default:
			}
		}
	}

}
