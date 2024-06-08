package handlers

import (
	"errors"
	"os"

	"github.com/dgrijalva/jwt-go"
)


var SECRET_KEY = []byte(os.Getenv("SECRET_KEY"))



func  GenerateToken(userID string) (string, error) {

	claim := jwt.MapClaims{}
	claim["user_id"] = userID
	
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)

	signedToken, err := token.SignedString(SECRET_KEY)
	if err != nil {
		return signedToken, err
	}

	return signedToken, nil

}

func ValidateToken(encodedToken string) (*jwt.Token, error) {
	token, err := jwt.Parse(encodedToken, func(token *jwt.Token) (interface{}, error) {
		_, ok := token.Method.(*jwt.SigningMethodHMAC)

		if !ok {
			return nil, errors.New("invalid token")
		}

		return []byte(SECRET_KEY), nil
	})

	if err != nil {
		return token, err
	}

	return token, nil

}

