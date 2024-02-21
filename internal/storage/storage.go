package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"yapracticum-go-diploma-1/internal/config"
)

//////////////////////////
// Storage
//////////////////////////

var ErrUserAlreadyExists error = errors.New("this login already exists in database")
var ErrUserAuthFailed error = errors.New("authentication failed")
var ErrUserNotLoggedIn error = errors.New("user session has expired")
var ErrOrderAlreadyExists error = errors.New("this order already exists in database")
var ErrOrderOtherUser error = errors.New("this order belongs to other user")
var ErrWithdrawNotEnough error = errors.New("hot enough bonus points")

var cfg config.Config
var logger *zap.Logger

type Storage struct {
	dbConn     *pgxpool.Pool
	connString string
	//sessionInfo *SessionInfoMap
	encKey string
}

func New(connString string) *Storage {
	return &Storage{connString: connString} //, sessionInfo: NewSessionInfoMap()}
}

func (s *Storage) Init(ctx context.Context, loger *zap.Logger, config config.Config) error {
	var err error

	logger = loger
	cfg = config

	poolConfig, err := pgxpool.ParseConfig(s.connString)
	if err != nil {
		logger.Sugar().Errorf("Unable to parse connection string: %s", err)
		return err
	}
	s.dbConn, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		logger.Sugar().Errorf("Unable to create connection pool: %s", err)
		return err
	}

	encKey := make([]byte, 128)
	_, err = rand.Read(encKey)
	if err != nil {
		return err
	}
	s.encKey = hex.EncodeToString(encKey)

	// Need execute to use uuid4
	_, err = s.dbConn.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`)
	if err != nil {
		return err
	}

	_, err = s.dbConn.Exec(ctx, queryCreateUsers)
	if err != nil {
		return err
	}

	_, err = s.dbConn.Exec(ctx, queryCreateOrders)
	if err != nil {
		return err
	}

	_, err = s.dbConn.Exec(ctx, queryCreateWithdrawals)
	if err != nil {
		return err
	}

	go s.NewOrdersRefresher(ctx)

	return nil
}

func (s *Storage) Close(ctx context.Context) {
	s.dbConn.Close()
}

func (s *Storage) GetOrderOwner(ctx context.Context, orderNum int) string {
	//"SELECT login FROM orders WHERE order_num = $1"
	query := `SELECT user_id FROM orders WHERE orders.order_num = $1`

	var login string
	row := s.dbConn.QueryRow(ctx, query, orderNum)
	err := row.Scan(&login)
	if err != nil {
		return ""
	}
	return login
}
