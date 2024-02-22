package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"golang.org/x/crypto/scrypt"
	"io"
	"strings"
)

func (s *Storage) UserRegister(ctx context.Context, login string, password string) error {

	salt := make([]byte, 32) // salt, 32 bytes len
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	hash, err := scrypt.Key([]byte(password), salt, 1<<14, 8, 1, 256) // hash, 256 bytes len
	if err != nil {
		return err
	}

	query := `INSERT INTO users (login, password, salt) VALUES ($1, $2, $3)`

	if _, err = s.dbConn.Exec(ctx, query, login, hex.EncodeToString(hash), hex.EncodeToString(salt)); err != nil {
		if strings.Contains(err.Error(), "(SQLSTATE 23505)") {
			s.logger.Sugar().Errorf("Login %s already exists in database", login)
			return fmt.Errorf("%s: %w", err.Error(), ErrUserAlreadyExists)
		}
		return err
	}

	return nil
}

func (s *Storage) UserCheckLoggedIn(token string) (string, error) {
	ac := AuthClaims{}
	err := ac.SetFromJWT(token, s.encKey)
	if err != nil {
		return "", err
	}

	return ac.UserID, nil
}

func (s *Storage) UserLogin(ctx context.Context, login string, password string) (string, error) {

	var err error
	query := `SELECT id, login, password, salt FROM users WHERE login=$1`
	row := s.dbConn.QueryRow(ctx, query, login)
	var (
		sUserID string
		sLogin  string
		sPassw  string
		sSalt   string
	)
	if err = row.Scan(&sUserID, &sLogin, &sPassw, &sSalt); err != nil {
		return "", err
	}

	xSalt, _ := hex.DecodeString(sSalt)

	var key []byte
	if key, err = scrypt.Key([]byte(password), xSalt, 1<<14, 8, 1, 256); err != nil {
		return "", err
	}

	if hex.EncodeToString(key) != sPassw {
		return "", ErrUserAuthFailed
	}

	ac := AuthClaims{UserID: sUserID}
	jwt, err := ac.GetJWT(s.encKey)
	if err != nil {
		return "", err
	}

	return jwt, nil
}
