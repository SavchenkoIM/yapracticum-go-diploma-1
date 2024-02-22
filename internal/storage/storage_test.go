package storage

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"testing"
	"yapracticum-go-diploma-1/internal/config"

	"yapracticum-go-diploma-1/internal/storage/testhelpers"
)

type TestStorager interface {
	Storager
}

type StorageTestSuite struct {
	suite.Suite
	TestStorager
	container *testhelpers.TestDatabase
}

func (sts *StorageTestSuite) SetupTest() {

	storageContainer := testhelpers.NewTestDatabase(sts.T())

	//connstring := fmt.Sprintf("postgresql://%s:%d/postgres?user=postgres&password=postgres", storageContainer.Host(), storageContainer.Port(sts.T()))
	connstring := fmt.Sprintf("postgresql://%s:%d/postgres?user=postgres&password=postgres", "localhost", 5432)
	logger, err := zap.NewProduction()
	require.NoError(sts.T(), err)

	store := New(config.Config{ConnString: connstring}, logger)
	err = store.Init(context.Background())
	require.NoError(sts.T(), err)

	sts.TestStorager = store
	sts.container = storageContainer
}

func (sts *StorageTestSuite) TearDownTest() {
	sts.container.Close(sts.T())
}

func TestStorageTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip()
		return
	}

	t.Parallel()
	suite.Run(t, new(StorageTestSuite))
}

func (sts *StorageTestSuite) Test_End_To_End() {

	ctx := context.Background()

	/////////////////////////////
	// Login/Register
	/////////////////////////////

	sts.Run(`Login Unregistered User`, func() {
		_, err := sts.TestStorager.UserLogin(ctx, "TestUser", "TestPassword")
		if err == nil {
			sts.T().Errorf("User TestUser with passw TestPassword unexpectedly logged in")
		}
	})

	sts.Run(`Register User`, func() {
		if err := sts.TestStorager.UserRegister(ctx, "TestUser", "TestPassword"); err != nil {
			sts.T().Errorf("Register user TestUser with passw TestPassword, error: %s", err.Error())
		}
	})

	sts.Run(`Register User Second Time`, func() {
		if err := sts.TestStorager.UserRegister(ctx, "TestUser", "TestPassword"); err == nil {
			sts.T().Errorf("User TestUser unexpectedly got registered second time")
		}
	})

	var err error
	token := ""
	userID := ""
	sts.Run(`Login Registered User`, func() {
		token, err = sts.TestStorager.UserLogin(ctx, "TestUser", "TestPassword")
		if err != nil {
			sts.T().Errorf("Login user TestUser with passw TestPassword, error: %s", err.Error())
		}

		userID, err = sts.TestStorager.UserCheckLoggedIn(token)
		if err != nil {
			sts.T().Errorf("User token verification, error: %s", err.Error())
		}
	})

	/////////////////////////////
	// Add order
	/////////////////////////////

	sts.Run(`Add Correct Order`, func() {
		err := sts.TestStorager.OrderAddNew(ctx, userID, 27815869)
		if err != nil {
			sts.T().Errorf("Failed to add correct order 27815869, Error: %s", err.Error())
		}
	})

	/////////////////////////////
	// Withdraw and check balance
	/////////////////////////////

	acc := Numeric(20050)
	sts.Run(`Accrual Of 200.50 Bonus Points`, func() {
		err := sts.TestStorager.ApplyAccrualResponse(ctx, AccrualResponse{Accrual: &acc, Status: "PROCESSED", Order: "27815869"})
		if err != nil {
			sts.T().Errorf("Failed to add correct order 27815869, Error: %s", err.Error())
		}
	})

	sts.Run(`Check Balance`, func() {
		balance, err := sts.TestStorager.GetBalance(ctx, userID)
		if err != nil {
			sts.T().Errorf("Failed to check balance, Error: %s", err.Error())
		}
		if *balance.Current != 20050 {
			sts.T().Errorf("Incorrect balance!, Want: %s, Actual: %s", &acc, balance.Current)
		}
	})

	sts.Run(`Withdraw 100 Bonus Points`, func() {
		err := sts.TestStorager.Withdraw(ctx, userID, 27815869, Numeric(10000))
		if err != nil {
			sts.T().Errorf("Failed to withdraw, Error: %s", err.Error())
		}
	})

	data, err := sts.TestStorager.GetWithdrawalsData(ctx, userID)
	if err != nil {
		return
	}
	sts.T().Log(len(data.Withdrawals))
	for _, v := range data.Withdrawals {
		sts.T().Log(v)
	}

	sts.Run(`ReCheck Balance`, func() {
		balance, err := sts.TestStorager.GetBalance(ctx, userID)
		if err != nil {
			sts.T().Errorf("Failed to check balance, Error: %s", err.Error())
		}
		if *balance.Current != 10050 {
			sts.T().Errorf("Incorrect balance!, Want: %s, Actual: %s", "100.50", balance.Current)
		}
	})

	sts.Run(`Withdraw 150 Bonus Points`, func() {
		err := sts.TestStorager.Withdraw(ctx, userID, 27815869, Numeric(15000))
		if err == nil {
			sts.T().Errorf("Unexpectedly withdrawed 150 bonus points")
		}
	})

	data, err = sts.TestStorager.GetWithdrawalsData(ctx, userID)
	if err != nil {
		return
	}
	for _, v := range data.Withdrawals {
		sts.T().Log(v)
	}

}
