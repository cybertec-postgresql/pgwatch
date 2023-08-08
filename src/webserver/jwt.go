package webserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt"
)

type loginReq struct {
	Username string `json:"user"`
	Password string `json:"password"`
}

func (Server *WebUIServer) IsCorrectPassword(lr loginReq) bool {
	return Server.WebUser == lr.Username && Server.WebPassword == lr.Password
}

func (Server *WebUIServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	var (
		err   error
		lr     loginReq
		token string
	)

	defer func() {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	switch r.Method {
	case "POST":
		if err = json.NewDecoder(r.Body).Decode(&lr); err != nil {
			return
		}
		if !Server.IsCorrectPassword(lr) {
			http.Error(w, "can not authenticate this user", http.StatusUnauthorized)
			return
		}
		if token, err = generateJWT(lr.Username); err != nil {
			return
		}
		_, err = w.Write([]byte(token))

	case "GET":
		fmt.Fprintf(w, "only POST methods is allowed.")
		return
	}
}

type EnsureAuth struct {
	handler http.HandlerFunc
}

func (ea *EnsureAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := validateToken(r); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	ea.handler(w, r)
}

func NewEnsureAuth(handlerToWrap http.HandlerFunc) *EnsureAuth {
	return &EnsureAuth{handlerToWrap}
}

var sampleSecretKey = []byte("5m3R7K4754p4m")

func generateJWT(username string) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)

	claims["authorized"] = true
	claims["username"] = username
	claims["exp"] = time.Now().Add(time.Hour * 8).Unix()

	return token.SignedString(sampleSecretKey)
}

func validateToken(r *http.Request) (err error) {
	if r.Header["Token"] == nil {
		return errors.New("can not find token in header")
	}
	token, err := jwt.Parse(r.Header["Token"][0], func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("there was an error in parsing")
		}
		return sampleSecretKey, nil
	})
	if err != nil {
		return err
	}
	if token == nil {
		return errors.New("invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return errors.New("cannot parse token claims")
	}
	if !claims.VerifyExpiresAt(time.Now().Local().Unix(), true) {
		return errors.New("token expired")
	}
	return nil
}
