package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
)

func (s *Storage) ApplyAccrualResponse(ctx context.Context, response AccrualResponse) error {
	switch response.Status {
	case "REGISTERED":
		return nil
	case "PROCESSING":
		query := "UPDATE orders SET status = $1 WHERE order_num = $2"
		_, err := s.dbConn.Exec(ctx, query, StatusProcessing, response.Order)
		if err != nil {
			return err
		}
		return nil
	case "INVALID":
		query := "UPDATE orders SET status = $1 WHERE order_num = $2"
		_, err := s.dbConn.Exec(ctx, query, StatusInvalid, response.Order)
		if err != nil {
			return err
		}
		return nil
	case "PROCESSED":

		txOk := false
		tx, err := s.dbConn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			return err
		}
		defer func() {
			if !txOk {
				tx.Rollback(ctx)
			}
		}()

		query := "UPDATE orders SET status = $1, accrual = $2 WHERE order_num = $3"
		_, err = tx.Exec(ctx, query, StatusProcessed, response.Accrual, response.Order)
		if err != nil {
			return err
		}

		query = `UPDATE users SET balance = balance + $1 WHERE id =	(SELECT user_id FROM orders WHERE order_num = $2)`
		_, err = tx.Exec(ctx, query, response.Accrual, response.Order)
		if err != nil {
			return err
		}

		err = tx.Commit(ctx)
		if err != nil {
			return err
		}

		txOk = true

		return nil
	default:
		return errors.New("unknown accrual status")
	}
}

func (s *Storage) NewOrdersRefresher(ctx context.Context) {
	mainLoopContextManager := NewCtxCancellWaiter(ctx, "NewOrdersRefresher", s.logger, 15)
	innerLoopContextManager := NewCtxCancellWaiter(ctx, "NewOrdersRefresher", s.logger, 0)

	for {
		// Wait 15 sec between scans. Monitoring context each second.
		scan, err := mainLoopContextManager.Scan()
		if err != nil {
			return
		}
		if !scan {
			continue
		}

		orders, err := s.GetUnhandledOrders(ctx)
		if err != nil {
			continue
		}

		innerLoopContextManager.SetSkipSeconds(0)
		for _, order := range orders.Orders {

			for {
				scan, err := innerLoopContextManager.Scan()
				if err != nil {
					return
				}
				if scan {
					break
				}
			}
			innerLoopContextManager.SetSkipSeconds(0)

			s.logger.Info(fmt.Sprintf("Accrual Request: %s/api/orders/%s", s.config.AccrualAddress, order.Number))

			resp, err := http.Get(fmt.Sprintf("%s/api/orders/%s", s.config.AccrualAddress, order.Number))
			if err != nil {
				continue
			}

			respData := make([]byte, resp.ContentLength)
			resp.Body.Read(respData)
			resp.Body.Close()

			s.logger.Info(fmt.Sprintf("Accrual Response: %s, Status: %d", string(respData), resp.StatusCode))

			switch resp.StatusCode {
			case http.StatusOK:
				var respParsed AccrualResponse
				err = json.Unmarshal(respData, &respParsed)
				if err != nil {
					continue
				}

				err = s.ApplyAccrualResponse(ctx, respParsed)
				if err != nil {
					s.logger.Error(err.Error())
				}

			case http.StatusTooManyRequests:
				raHeader := resp.Header.Get("Retry-After")
				retryTime, err := strconv.Atoi(raHeader)
				if err != nil {
					retryTime = 10
				}
				innerLoopContextManager.SetSkipSeconds(retryTime)
			}
		}
	}
}
