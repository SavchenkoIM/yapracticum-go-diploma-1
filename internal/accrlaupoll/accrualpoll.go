package accrlaupoll

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"net/http"
	"strconv"
	"sync"
	"time"
	"yapracticum-go-diploma-1/internal/storage"
	"yapracticum-go-diploma-1/internal/utils"
)

func AccrualPollWorker(ccw *utils.CtxCancellWaiter, s *storage.Storage, id int, wg *sync.WaitGroup, logger *zap.Logger, accrualAddress string, data chan string) {
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
			logger.Info(fmt.Sprintf("Worker %d. Accrual Request: %s/api/orders/%s", id, accrualAddress, order))

			resp, err := http.Get(fmt.Sprintf("%s/api/orders/%s", accrualAddress, order))
			if err != nil {
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
					continue
				}

				err = s.ApplyAccrualResponse(ccw.Ctx, respParsed)
				if err != nil {
					logger.Error(err.Error())
				}

				if (respParsed.Status != "PROCESSED" && respParsed.Status != "INVALID") || err != nil {
					fmt.Println(respParsed.Status)
					data <- order
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
