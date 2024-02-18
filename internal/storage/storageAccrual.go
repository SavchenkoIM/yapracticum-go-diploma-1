package storage

import (
	"context"
	"github.com/jackc/pgx/v5"
	"strconv"
	"time"
)

func (s *Storage) Withdraw(ctx context.Context, tokenID string, orderNum int64, sum Numeric) error {
	var login SessionInfo
	var err error
	if login, err = s.UserCheckLoggedIn(tokenID); err != nil {
		return err
	}

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

	query := `SELECT SUM(COALESCE(accrual, 0)) FROM orders WHERE user_id = $1`

	var currAccrual Numeric
	row := tx.QueryRow(ctx, query, login.userID)
	err = row.Scan(&currAccrual)
	if err != nil {
		return err
	}

	if currAccrual < sum {
		return ErrWithdrawNotEnough
	}

	query = `INSERT INTO withdrawals (user_id, order_num, sum) VALUES ($1, $2, $3)`
	_, err = tx.Exec(ctx, query, login.userID, orderNum, sum)
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

func (s *Storage) GetWithdrawalsData(ctx context.Context, tokenID string) (WithdrawalsInfo, error) {
	var (
		login SessionInfo
		err   error
	)
	login, err = s.UserCheckLoggedIn(tokenID)
	if err != nil {
		return WithdrawalsInfo{}, ErrUserNotLoggedIn
	}

	var rows pgx.Rows
	query := `SELECT order_num, sum, processed_at FROM withdrawals WHERE user_id = $1`
	rows, err = s.dbConn.Query(ctx, query, login.userID)

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

func (s *Storage) GetBalance(ctx context.Context, tokenID string) (BalanceInfo, error) {

	var login SessionInfo
	var err error
	if login, err = s.UserCheckLoggedIn(tokenID); err != nil {
		return BalanceInfo{}, err
	}

	txOk := false
	tx, err := s.dbConn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return BalanceInfo{}, err
	}
	defer func() {
		if !txOk {
			tx.Rollback(ctx)
		}
	}()

	query := `SELECT SUM(COALESCE(accrual,0)) FROM orders	WHERE user_id = $1`

	var currAccrual int64
	row := tx.QueryRow(ctx, query, login.userID)
	err = row.Scan(&currAccrual)
	if err != nil {
		return BalanceInfo{}, err
	}

	query = `SELECT COALESCE(SUM(sum),0) FROM withdrawals WHERE user_id = $1`

	var withdrawn int64 = 0
	row = tx.QueryRow(ctx, query, login.userID)
	err = row.Scan(&withdrawn)
	if err != nil {
		return BalanceInfo{}, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return BalanceInfo{}, err
	}
	txOk = true

	curr := Numeric(currAccrual - withdrawn)
	with := Numeric(withdrawn)
	logger.Sugar().Infof("Balance: %s, withdrawn: %s", &curr, &with)
	return BalanceInfo{Current: &curr, Withdrawn: &with}, nil
}
