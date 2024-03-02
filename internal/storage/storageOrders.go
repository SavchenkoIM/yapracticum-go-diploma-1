package storage

import (
	"context"
	"fmt"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"strconv"
	"strings"
	"time"
	"yapracticum-go-diploma-1/internal/utils"
)

func (s *Storage) OrderAddNew(ctx context.Context, userID string, orderNum string) error {
	oNum, err := strconv.Atoi(orderNum)
	if err != nil || (!utils.LuhnValid(int(oNum)) && s.config.UseLuhn) {
		return ErrOrderLuhnCheckFailed
	}

	query := `INSERT INTO orders (user_id, order_num) VALUES ($1, $2)`

	_, err = s.dbConn.Exec(ctx, query, userID, orderNum)
	if err != nil {
		if strings.Contains(err.Error(), pgerrcode.UniqueViolation) {

			if s.GetOrderOwner(ctx, orderNum) != userID {
				s.logger.Sugar().Errorf("Order %s belongs to other user", orderNum)
				return fmt.Errorf("%s: %w", err.Error(), ErrOrderOtherUser)
			}

			s.logger.Sugar().Errorf("Order %s already exists in database", orderNum)
			return fmt.Errorf("%s: %w", err.Error(), ErrOrderAlreadyExists)
		}
		s.logger.Sugar().Errorf(err.Error())
		return err
	}

	select {
	case s.newOrdersCh <- OrderTag{OrderNum: orderNum, PollAfter: time.Now()}:
	default:
		s.logger.Sugar().Warnf("Order %s processing is delayed due to high load", orderNum)
	}

	return nil
}

func (s *Storage) GetOrdersData(ctx context.Context, userID string) (OrdersInfo, error) {
	query := `SELECT order_num, status, accrual, uploaded_at FROM orders WHERE user_id = $1`
	rows, err := s.dbConn.Query(ctx, query, userID)
	if err != nil {
		s.logger.Sugar().Errorf(err.Error())
		return OrdersInfo{}, err
	}
	return s.getOrdersFromRequest(rows, query)
}

func (s *Storage) GetUnhandledOrders(ctx context.Context) (OrdersInfo, error) {
	query := `SELECT order_num, status, accrual, uploaded_at FROM orders	WHERE status NOT IN ($1, $2)`
	rows, err := s.dbConn.Query(ctx, query, StatusInvalid, StatusProcessed)
	if err != nil {
		s.logger.Sugar().Errorf(err.Error())
		return OrdersInfo{}, err
	}
	return s.getOrdersFromRequest(rows, query)
}

func (s *Storage) getOrdersFromRequest(rows pgx.Rows, query string) (OrdersInfo, error) {
	orders := make([]OrderInfo, 0)
	var (
		oUser       string
		oNumber     string
		oStatus     OrderStatus
		oAccrual    pgtype.Int8
		oUploadedAt time.Time
	)

	for rows.Next() {
		err := rows.Scan(&oNumber, &oStatus, &oAccrual, &oUploadedAt)
		if err != nil {
			s.logger.Sugar().Errorf("Query: %s, %s", query, err.Error())
			return OrdersInfo{}, err
		}
		accr := Numeric(oAccrual.Int64)
		orders = append(orders, OrderInfo{User: oUser, Number: oNumber, Status: oStatus, Accrual: &accr, UploadedAt: RFC3339Time(oUploadedAt)})
	}

	return OrdersInfo{Orders: orders}, nil
}
