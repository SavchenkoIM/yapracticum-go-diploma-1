package storage

import (
	"context"
	"github.com/jackc/pgx/v5"
	"strconv"
	"time"
)

func (s *Storage) Withdraw(ctx context.Context, userID string, orderNum int64, sum Numeric) error {

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

	s.logger.Sugar().Infof("Withdraw attempt: Requested: %s", &sum)

	query := `INSERT INTO withdrawals (user_id, order_num, sum) VALUES ($1, $2, $3)`
	_, err = tx.Exec(ctx, query, userID, orderNum, sum)
	if err != nil {
		return err
	}

	query = "UPDATE users SET balance = balance - $2, withdrawn = withdrawn + $2 WHERE id = $1"
	_, err = tx.Exec(ctx, query, userID, sum)
	if err != nil {
		return ErrWithdrawNotEnough
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
		s.logger.Sugar().Errorf(err.Error())
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
			s.logger.Sugar().Errorf("Query %s, %s", query, err.Error())
			return WithdrawalsInfo{}, err
		}
		withdrawals = append(withdrawals, WithdrawalInfo{Order: strconv.FormatInt(oNumber, 10), Sum: &oSum, ProcessedAt: RFC3339Time(oUploadedAt)})
	}

	return WithdrawalsInfo{Withdrawals: withdrawals}, nil
}

func (s *Storage) GetBalance(ctx context.Context, userID string) (BalanceInfo, error) {
	query := "SELECT balance, withdrawn FROM users WHERE id = $1"
	row := s.dbConn.QueryRow(ctx, query, userID)
	var balance int64
	var withdraw int64
	err := row.Scan(&balance, &withdraw)
	if err != nil {
		return BalanceInfo{}, err
	}

	curr := Numeric(balance)
	with := Numeric(withdraw)
	s.logger.Sugar().Infof("Balance: %s, withdrawn: %s", &curr, &with)
	return BalanceInfo{Current: &curr, Withdrawn: &with}, nil
}
