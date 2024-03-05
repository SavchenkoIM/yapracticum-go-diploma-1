package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"strconv"
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

	logger, err := zap.NewProduction()
	require.NoError(sts.T(), err)

	_, err := New(config.Config{ConnString: "jfglwekflw", UseLuhn: true}, logger, make(chan OrderTag, 20))
	require.Error(sts.T(), err)

	_, err = New(config.Config{ConnString: "postgresql://localhost:67787/postgres?user=postgres&password=postgres", UseLuhn: true}, logger, make(chan OrderTag, 5))
	require.Error(sts.T(), err)

	storageContainer := testhelpers.NewTestDatabase(sts.T())
	connstring := fmt.Sprintf("postgresql://%s:%d/postgres?user=postgres&password=postgres", storageContainer.Host(), storageContainer.Port(sts.T()))

	store, _ := New(config.Config{ConnString: connstring, UseLuhn: true}, logger, make(chan OrderTag, 20))
	err = store.Init(context.Background())
	require.NoError(sts.T(), err)

	sts.TestStorager = store
	sts.container = storageContainer
}

func (sts *StorageTestSuite) TearDownTest() {
	sts.TestStorager.Close(context.Background())
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

func (sts *StorageTestSuite) Test_DTypes() {
	// Numeric
	sts.Run(`DType Numeric JSON Marshal`, func() {
		n := Numeric(25000)
		nJSON, err := json.Marshal(&n)
		require.NoError(sts.T(), err)
		assert.JSONEq(sts.T(), `250`, string(nJSON))
	})
	sts.Run(`DType Numeric Correct JSON UnMarshal`, func() {
		n := Numeric(0)
		err := json.Unmarshal([]byte("400"), &n)
		require.NoError(sts.T(), err)
		assert.Equal(sts.T(), 40000, int(n))
	})
	sts.Run(`DType Numeric Incorrect JSON UnMarshal`, func() {
		n := Numeric(0)
		err := json.Unmarshal([]byte("4d0"), &n)
		require.Error(sts.T(), err)
	})

	// OrderStatus
	sts.Run(`DType OrderStatus Incorrect JSON Marshal`, func() {
		n := OrderStatus(10)
		nJSON, err := json.Marshal(&n)
		require.NoError(sts.T(), err)
		assert.Equal(sts.T(), `"UNKNOWN!!!"`, string(nJSON))
	})
	sts.Run(`DType OrderStatus PROCESSING JSON Marshal`, func() {
		n := OrderStatus(StatusProcessing)
		nJSON, err := json.Marshal(&n)
		require.NoError(sts.T(), err)
		assert.Equal(sts.T(), `"PROCESSING"`, string(nJSON))
	})
	sts.Run(`DType OrderStatus INVALID JSON Marshal`, func() {
		n := OrderStatus(StatusInvalid)
		nJSON, err := json.Marshal(&n)
		require.NoError(sts.T(), err)
		assert.Equal(sts.T(), `"INVALID"`, string(nJSON))
	})
	sts.Run(`DType OrderStatus NEW JSON Marshal`, func() {
		n := OrderStatus(StatusNew)
		nJSON, err := json.Marshal(&n)
		require.NoError(sts.T(), err)
		assert.Equal(sts.T(), `"NEW"`, string(nJSON))
	})
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

	sts.Run(`Login Reg User Wrong Pass`, func() {
		token, err = sts.TestStorager.UserLogin(ctx, "TestUser", "TestPassword2")
		if err == nil {
			sts.T().Errorf("Unexpectedly logged in with wrong password")
		}
	})

	sts.Run(`Check LogIn Empty Token`, func() {
		userID, err = sts.TestStorager.UserCheckLoggedIn("")
		if err == nil {
			sts.T().Errorf("Unexpectedly logged in with wrong token")
		}
	})

	oldToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3MDg4ODE3MjUsIm5iZiI6MTcwODg3MDkyNSwiaWF0IjoxNzA4ODcwOTI1LCJ1c2VyX2lkIjoiYzJhNzkxYjctOTQwNi00ODMxLThmMzgtY2JkZjkxZTcwZDczIiwicmFuZF9udW0iOiJMRmh6NStkejJKTT0ifQ.udt4q16ML6JMnInnONLhHm3OLYSiWg2CMcyByOkKqsI"
	sts.Run(`Check LogIn Expired Token`, func() {
		userID, err = sts.TestStorager.UserCheckLoggedIn(oldToken)
		if err == nil {
			sts.T().Errorf("Unexpectedly logged in with expired token")
		}
	})

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

	sts.Run(`Register Other User`, func() {
		if err := sts.TestStorager.UserRegister(ctx, "TestUser2", "TestPassword2"); err != nil {
			sts.T().Errorf("Register user TestUser2 with passw TestPassword2, error: %s", err.Error())
		}
	})

	userID2 := ""
	sts.Run(`Login Other User`, func() {
		token, err = sts.TestStorager.UserLogin(ctx, "TestUser", "TestPassword")
		if err != nil {
			sts.T().Errorf("Login user TestUser with passw TestPassword, error: %s", err.Error())
		}

		userID2, err = sts.TestStorager.UserCheckLoggedIn(token)
		if err != nil {
			sts.T().Errorf("User token verification, error: %s", err.Error())
		}
	})

	/////////////////////////////
	// Add order
	/////////////////////////////

	sts.Run(`Add Correct Order`, func() {
		err := sts.TestStorager.OrderAddNew(ctx, userID, "27815869")
		if err != nil {
			sts.T().Errorf("Failed to add correct order 27815869, Error: %s", err.Error())
		}
	})

	sts.Run(`Add Correct Order Twice`, func() {
		err := sts.TestStorager.OrderAddNew(ctx, userID, "27815869")
		if err == nil {
			sts.T().Errorf("Unexpectedly added order twice")
		}
	})

	sts.Run(`Add Correct Order Other User`, func() {
		err := sts.TestStorager.OrderAddNew(ctx, userID2, "27815869")
		if err == nil {
			sts.T().Errorf("Unexpectedly added order with other user logged")
		}
	})

	sts.Run(`Add Incorrect Order`, func() {
		err := sts.TestStorager.OrderAddNew(ctx, userID, "27815871")
		if err == nil {
			sts.T().Errorf("Unexpectedly added incorrect order 27815871")
		}
	})

	sts.Run(`Get Orders`, func() {
		or, _ := sts.TestStorager.GetOrdersData(ctx, userID)
		jsonm, _ := json.Marshal(or.Orders)

		expLen := len(or.Orders) == 1

		var jsonTest = []byte("")
		if expLen {
			orTest := OrdersInfo{Orders: make([]OrderInfo, len(or.Orders))}
			val := Numeric(0)
			orTest.Orders[0] = OrderInfo{Number: "27815869", Status: 0, Accrual: &val, UploadedAt: or.Orders[0].UploadedAt}
			jsonTest, _ = json.Marshal(orTest.Orders)
		}

		if expLen && !sts.JSONEq(string(jsonTest), string(jsonm)) {
			sts.T().Errorf("Got incorrect orders data")
		}
	})

	sts.Run(`Add More Orders Than Channel Cap`, func() {
		or, _ := sts.TestStorager.GetOrdersData(ctx, userID)
		nOrders := len(or.Orders)

		cfg := sts.TestStorager.getConfig()
		cfg.UseLuhn = false
		sts.TestStorager.setConfig(cfg)
		for i := 40000; i < 40100; i++ {
			err = sts.TestStorager.OrderAddNew(ctx, userID, strconv.Itoa(i))
			assert.NoError(sts.T(), err)
		}

		or, _ = sts.TestStorager.GetOrdersData(ctx, userID)
		assert.Equal(sts.T(), nOrders+100, len(or.Orders))
	})

	/////////////////////////////
	// Withdraw and check balance
	/////////////////////////////

	acc := Numeric(20050)
	sts.Run(`Accrual Of 200.50 Bonus Points`, func() {
		err := sts.TestStorager.ApplyAccrualResponse(ctx, AccrualResponse{Accrual: &acc, Status: "PROCESSED", Order: "27815869"})
		if err != nil {
			sts.T().Errorf("Failed to apply accrual to existing unfinalized otrder, Error: %s", err.Error())
		}
	})

	sts.Run(`Accrual Of 200.50 Bonus Points To Not Existing Order`, func() {
		err := sts.TestStorager.ApplyAccrualResponse(ctx, AccrualResponse{Accrual: &acc, Status: "PROCESSED", Order: "27815871"})
		if err == nil {
			sts.T().Errorf("Unexpected successful accrual to not existing order")
		}
	})

	sts.Run(`Apply Accrual "INVALID"`, func() {
		err := sts.TestStorager.ApplyAccrualResponse(ctx, AccrualResponse{Status: "INVALID", Order: "27815869"})
		if err == nil {
			sts.T().Errorf("Unexpected successful write of INVALID state to already finalized order")
		}
	})

	sts.Run(`Apply Accrual "PROCESSING"`, func() {
		err := sts.TestStorager.ApplyAccrualResponse(ctx, AccrualResponse{Status: "PROCESSING", Order: "27815869"})
		if err == nil {
			sts.T().Errorf("Unexpected successful write of PROCESSING state to already finalized order")
		}
	})

	sts.Run(`Apply Accrual "REGISTERED"`, func() {
		err := sts.TestStorager.ApplyAccrualResponse(ctx, AccrualResponse{Status: "REGISTERED", Order: "27815869"})
		if err != nil {
			sts.T().Errorf("Unexpected error during writing accrual REGISTERED, Error: %s", err.Error())
		}
	})

	sts.Run(`Apply Accrual Incorrect Status`, func() {
		err := sts.TestStorager.ApplyAccrualResponse(ctx, AccrualResponse{Status: "REGISTERD", Order: "27815869"})
		if err == nil {
			sts.T().Errorf("Unexpected error absence while applying accrual with unknown status")
		}
	})

	acc2 := Numeric(30050)
	sts.Run(`Accrual Of 300.50 Bonus Points`, func() {
		err := sts.TestStorager.ApplyAccrualResponse(ctx, AccrualResponse{Accrual: &acc2, Status: "PROCESSED", Order: "27815869"})
		if err == nil {
			sts.T().Errorf("Unexpected successful write of PROCESSED state to already finalized order")
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
		err := sts.TestStorager.Withdraw(ctx, userID, "27815869", Numeric(10000))
		if err != nil {
			sts.T().Errorf("Failed to withdraw, Error: %s", err.Error())
		}
	})

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
		err := sts.TestStorager.Withdraw(ctx, userID, "27815869", Numeric(15000))
		if err == nil {
			sts.T().Errorf("Unexpectedly withdrawed 150 bonus points")
		}
	})

	sts.Run(`Get Withdrawals`, func() {
		wi, _ := sts.TestStorager.GetWithdrawalsData(ctx, userID)
		jsonm, _ := json.Marshal(wi.Withdrawals)

		expLen := len(wi.Withdrawals) == 1

		var jsonTest = []byte("")
		if expLen {
			wiTest := WithdrawalsInfo{Withdrawals: make([]WithdrawalInfo, len(wi.Withdrawals))}
			val := Numeric(10000)
			wiTest.Withdrawals[0] = WithdrawalInfo{Order: "27815869", Sum: &val, ProcessedAt: wi.Withdrawals[0].ProcessedAt}
			jsonTest, _ = json.Marshal(wiTest.Withdrawals)
		}

		if expLen && !sts.JSONEq(string(jsonTest), string(jsonm)) {
			sts.T().Errorf("Got incorrect withdrawals data")
		}
	})

	/////////////////////////////
	// Cancelled context
	/////////////////////////////

	ctxWCancel, cancel := context.WithCancel(context.Background())
	cancel()
	sts.Run(`Get Withdrawals CancelledCtx`, func() {
		_, err = sts.TestStorager.GetWithdrawalsData(ctxWCancel, userID)
		assert.Error(sts.T(), err)
	})
}
