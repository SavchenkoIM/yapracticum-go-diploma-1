package storage

import (
	"crypto/rand"
	"fmt"
	"github.com/golang-jwt/jwt"
)

type AuthClaims struct {
	UserID  string `json:"user_id"`
	RandNum []byte `json:"rand_num"`
}

func (ac *AuthClaims) Valid() error {
	return nil
}

func (ac *AuthClaims) GetJWT(key string) (string, error) {
	secRandNum := make([]byte, 64)
	_, err := rand.Read(secRandNum)
	if err != nil {
		return "", err
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &AuthClaims{
		UserID:  ac.UserID,
		RandNum: secRandNum,
	})
	tokenString, err := token.SignedString([]byte(key))
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

func (ac *AuthClaims) SetFromJWT(tokenString string, key string) error {
	token, err := jwt.ParseWithClaims(tokenString, ac,
		func(t *jwt.Token) (interface{}, error) {
			return []byte(key), nil
		})
	if err != nil {
		ac.UserID = ""
		return err
	}
	if !token.Valid {
		ac.UserID = ""
		return fmt.Errorf("token %s is not valid", tokenString)
	}
	return nil
}
