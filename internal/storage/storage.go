package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"sync"
	"time"
	"yapracticum-go-diploma-1/internal/config"
	"yapracticum-go-diploma-1/internal/utils"
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
	dbConn      *pgxpool.Pool
	config      config.Config
	encKey      string
	logger      *zap.Logger
	rStarter    sync.Once          // First Init call detector
	workersWg   *sync.WaitGroup    // WaitGroup for Storage Workers
	stopWorkers context.CancelFunc // Cancel function for Storage Workers Context
	workersCtx  context.Context    // Storage Workers Context
	newOrdersCh chan string        // Channel for orders to be processed
}

func New(config config.Config, logger *zap.Logger, newOrdersCh chan string) (*Storage, error) {
	encKey := make([]byte, 128)
	_, err := rand.Read(encKey)
	if err != nil {
		return nil, err
	}

	s := Storage{
		config:      config,
		logger:      logger,
		encKey:      hex.EncodeToString(encKey),
		stopWorkers: nil,
		workersCtx:  nil,
		workersWg:   &sync.WaitGroup{},
		newOrdersCh: newOrdersCh,
	}

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
	firstInit := false
	s.rStarter.Do(func() {
		firstInit = true
	})

	if firstInit {
		s.workersCtx, s.stopWorkers = context.WithCancel(ctx)
	}

	// Need execute to use uuid4
	var err error
	errs := make([]error, 0)
	_, err = s.dbConn.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`)
	if err != nil {
		errs = append(errs, err)
	}

	_, err = s.dbConn.Exec(ctx, queryCreateUsers)
	if err != nil {
		errs = append(errs, err)
	}

	_, err = s.dbConn.Exec(ctx, queryCreateOrders)
	if err != nil {
		errs = append(errs, err)
	}

	_, err = s.dbConn.Exec(ctx, queryCreateWithdrawals)
	if err != nil {
		errs = append(errs, err)
	}

	if firstInit {
		s.workersWg.Add(1)
		go s.autoInit(s.workersCtx)
	}

	return errors.Join(errs...)
}

func (s *Storage) autoInit(ctx context.Context) {
	defer func() { s.workersWg.Done() }()
	connPrev := true
	connected := false
	cw := utils.NewCtxCancellWaiter(ctx, 15*time.Second)

	for {
		if cw.Scan() != nil {
			s.logger.Info("autoInit worker stopped")
			return
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
	s.logger.Info("Stopping storage workers...")
	s.stopWorkers()
	s.workersWg.Wait()
	s.dbConn.Close()
}

func (s *Storage) GetOrderOwner(ctx context.Context, orderNum string) string {

	query := `SELECT user_id FROM orders WHERE orders.order_num = $1`

	var login string
	row := s.dbConn.QueryRow(ctx, query, orderNum)
	err := row.Scan(&login)
	if err != nil {
		return ""
	}
	return login
}
