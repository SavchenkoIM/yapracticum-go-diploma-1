package storage

import (
	"context"
	"github.com/jackc/pgx/v5"
	"strconv"
	"time"
)

func (s *Storage) Withdraw(ctx context.Context, userID string, orderNum int64, sum Numeric) error {

	txOk := false
	tx, err := s.dbConn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() {
		if !txOk {
			tx.Rollback(ctx)
		}
	}()

	balance, err := s.GetBalance(ctx, tx, userID)
	if err != nil {
		return err
	}

	logger.Sugar().Infof("Withdraw attempt: Balance: %s, Requested: %s", balance.Current, &sum)
	if *balance.Current < sum {
		return ErrWithdrawNotEnough
	}

	query := `INSERT INTO withdrawals (user_id, order_num, sum) VALUES ($1, $2, $3)`
	_, err = tx.Exec(ctx, query, userID, orderNum, sum)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return err
	}
	txOk = true

	return nil
}

func (s *Storage) GetWithdrawalsData(ctx context.Context, userID string) (WithdrawalsInfo, error) {

	var rows pgx.Rows
	var err error
	query := `SELECT order_num, sum, processed_at FROM withdrawals WHERE user_id = $1`
	rows, err = s.dbConn.Query(ctx, query, userID)

	if err != nil {
		logger.Sugar().Errorf(err.Error())
		return WithdrawalsInfo{}, err
	}

	withdrawals := make([]WithdrawalInfo, 0)
	var (
		oNumber     int64
		oSum        Numeric
		oUploadedAt time.Time
	)

	for rows.Next() {
		err := rows.Scan(&oNumber, &oSum, &oUploadedAt)
		if err != nil {
			logger.Sugar().Errorf("Query %s, %s", query, err.Error())
			return WithdrawalsInfo{}, err
		}
		withdrawals = append(withdrawals, WithdrawalInfo{Order: strconv.FormatInt(oNumber, 10), Sum: &oSum, ProcessedAt: RFC3339Time(oUploadedAt)})
	}

	return WithdrawalsInfo{Withdrawals: withdrawals}, nil
}

func (s *Storage) GetBalance(ctx context.Context, parentTx pgx.Tx, userID string) (BalanceInfo, error) {

	txOk := false
	var err error
	var tx pgx.Tx
	if parentTx == nil {
		tx, err = s.dbConn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
		if err != nil {
			return BalanceInfo{}, err
		}
		defer func() {
			if !txOk {
				tx.Rollback(ctx)
			}
		}()
	} else {
		tx, err = parentTx, nil
	}

	query := `SELECT SUM(COALESCE(accrual,0)) FROM orders	WHERE user_id = $1`

	var currAccrual int64
	row := tx.QueryRow(ctx, query, userID)
	err = row.Scan(&currAccrual)
	if err != nil {
		return BalanceInfo{}, err
	}

	query = `SELECT COALESCE(SUM(sum),0) FROM withdrawals WHERE user_id = $1`

	var withdrawn int64 = 0
	row = tx.QueryRow(ctx, query, userID)
	err = row.Scan(&withdrawn)
	if err != nil {
		return BalanceInfo{}, err
	}

	if parentTx == nil {
		err = tx.Commit(ctx)
		if err != nil {
			return BalanceInfo{}, err
		}
	}
	txOk = true

	curr := Numeric(currAccrual - withdrawn)
	with := Numeric(withdrawn)
	logger.Sugar().Infof("Balance: %s, withdrawn: %s", &curr, &with)
	return BalanceInfo{Current: &curr, Withdrawn: &with}, nil
}
