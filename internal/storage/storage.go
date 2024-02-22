package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"sync"
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

type Storage struct {
	dbConn   *pgxpool.Pool
	config   config.Config
	encKey   string
	logger   *zap.Logger
	rStarter sync.Once
}

func New(config config.Config, logger *zap.Logger) (*Storage, error) {
	encKey := make([]byte, 128)
	_, err := rand.Read(encKey)
	if err != nil {
		return nil, err
	}

	s := Storage{config: config, logger: logger, encKey: hex.EncodeToString(encKey)}

	poolConfig, err := pgxpool.ParseConfig(s.config.ConnString)
	if err != nil {
		s.logger.Sugar().Errorf("Unable to parse connection string: %s", err)
		return nil, err
	}
	s.dbConn, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		s.logger.Sugar().Errorf("Unable to create connection pool: %s", err)
		return nil, err
	}

	return &s, nil
}

func (s *Storage) Init(ctx context.Context) error {
	defer func() {
		s.rStarter.Do(func() {
			go s.NewOrdersRefresher(ctx)
			go s.autoInit(ctx)
		})
	}()

	var err error
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

	return nil
}

func (s *Storage) autoInit(ctx context.Context) {
	connPrev := true
	connected := false
	cw := NewCtxCancellWaiter(ctx, "autoInit", s.logger, 15)

	for {
		scan, err := cw.Scan()
		if err != nil {
			return
		}
		if !scan {
			continue
		}

		if connected = s.dbConn.Ping(ctx) == nil; connected && !connPrev {
			err := s.Init(ctx)
			if err != nil {
				s.logger.Sugar().Errorf("Initialization error: %s", err.Error())
			} else {
				s.logger.Sugar().Warnf("Database restored after fault.")
			}
		}
		connPrev = connected
	}
}

func (s *Storage) Close(ctx context.Context) {
	s.dbConn.Close()
}

func (s *Storage) GetOrderOwner(ctx context.Context, orderNum int) string {

	query := `SELECT user_id FROM orders WHERE orders.order_num = $1`

	var login string
	row := s.dbConn.QueryRow(ctx, query, orderNum)
	err := row.Scan(&login)
	if err != nil {
		return ""
	}
	return login
}
