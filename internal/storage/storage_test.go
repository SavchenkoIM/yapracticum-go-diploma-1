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

	connstring := fmt.Sprintf("postgresql://%s:%d/postgres?user=postgres&password=postgres", storageContainer.Host(), storageContainer.Port(sts.T()))
	logger, err := zap.NewProduction()
	require.NoError(sts.T(), err)

	store := New(connstring)
	err = store.Init(context.Background(), logger, config.Config{
		ConnString: connstring})
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

func (sts *StorageTestSuite) Test_storage_User() {

	sts.Run(`Login Unregistered User`, func() {
		_, err := sts.TestStorager.UserLogin(context.Background(), "TestUser", "TestPassword")
		if err == nil {
			sts.T().Errorf("User TestUser with passw TestPassword unexpectedly logged in")
		}
	})

	sts.Run(`Register User`, func() {
		if err := sts.TestStorager.UserRegister(context.Background(), "TestUser", "TestPassword"); err != nil {
			sts.T().Errorf("Register user TestUser with passw TestPassword, error: %s", err.Error())
		}
	})

	sts.Run(`Register User Second Time`, func() {
		if err := sts.TestStorager.UserRegister(context.Background(), "TestUser", "TestPassword"); err == nil {
			sts.T().Errorf("User TestUser unexpectedly got registered second time")
		}
	})

	sts.Run(`Login Registered User`, func() {
		_, err := sts.TestStorager.UserLogin(context.Background(), "TestUser", "TestPassword")
		if err != nil {
			sts.T().Errorf("Login user TestUser with passw TestPassword, error: %s", err.Error())
		}
	})
}
