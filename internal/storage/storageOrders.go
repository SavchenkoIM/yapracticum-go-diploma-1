package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"time"
)

func (s *Storage) OrderAddNew(ctx context.Context, userID string, orderNum string) error {
	var err error
	query := `INSERT INTO orders (user_id, order_num) VALUES ($1, $2)`

	_, err = s.dbConn.Exec(ctx, query, userID, orderNum)
	if err != nil {
		fmt.Println(err.Error())
		if strings.Contains(err.Error(), "(SQLSTATE 23505)") {

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

	s.newOrdersCh <- orderNum
	return nil
}

func (s *Storage) GetOrdersData(ctx context.Context, userID string) (OrdersInfo, error) {
	return s.getOrdersByCondition(ctx, userID, OrdersRequestByUser)
}

func (s *Storage) GetUnhandledOrders(ctx context.Context) (OrdersInfo, error) {
	return s.getOrdersByCondition(ctx, "", OrdersRequestByStatusUnhandled)
}

func (s *Storage) getOrdersByCondition(ctx context.Context, userID string, condition int) (OrdersInfo, error) {
	var err error
	var rows pgx.Rows
	var query string
	switch condition {
	case OrdersRequestByUser:
		query = `SELECT order_num, status, accrual, uploaded_at FROM orders WHERE user_id = $1`
		rows, err = s.dbConn.Query(ctx, query, userID)
	case OrdersRequestByStatusUnhandled:
		query = `SELECT order_num, status, accrual, uploaded_at FROM orders	WHERE status NOT IN ($1, $2)`
		rows, err = s.dbConn.Query(ctx, query, StatusInvalid, StatusProcessed)
	default:
		return OrdersInfo{}, errors.New("unknown condition for orders newOrdersCh request")
	}

	if err != nil {
		s.logger.Sugar().Errorf(err.Error())
		return OrdersInfo{}, err
	}

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
