package webserver

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/golang-jwt/jwt/v5"
)

type loginReq struct {
	Username string `json:"user"`
	Password string `json:"password"`
}

func (Server *WebUIServer) IsCorrectPassword(lr loginReq) bool {
	return (Server.WebUser+Server.WebPassword == "") ||
		(Server.WebUser == lr.Username && Server.WebPassword == lr.Password)
}

func (Server *WebUIServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	var (
		err   error
		lr    loginReq
		token string
	)

	defer func() {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	switch r.Method {
	case "POST":
		if err = jsoniter.ConfigFastest.NewDecoder(r.Body).Decode(&lr); err != nil {
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

var (
	secretKeyOnce sync.Once
	secretKey     []byte
)

// jwtSecretKey returns the HS256 signing key used for Web UI/API tokens.
// The key is a cryptographically random 32-byte value generated once per
// process at first use. Generating it at runtime (instead of hardcoding a
// value in the source) prevents attackers from forging valid tokens.
//
// The key is intentionally not persisted: every process restart rotates it,
// which invalidates any previously issued token. This is a security feature,
// not a limitation - it bounds the lifetime of leaked tokens and forces users
// to re-authenticate with the configured credentials rather than relying on
// long-lived sessions.
//
// Operational note: because each instance generates its own key, tokens are
// not portable across instances. Multi-replica/HA deployments should use
// sticky sessions at the load balancer.
func jwtSecretKey() []byte {
	secretKeyOnce.Do(func() {
		secretKey = make([]byte, 32)
		if _, err := rand.Read(secretKey); err != nil {
			// crypto/rand.Read should never fail; if it does there is no safe
			// way to continue issuing/validating tokens, so fail loudly.
			panic("webserver: unable to generate JWT secret key: " + err.Error())
		}
	})
	return secretKey
}

func generateJWT(username string) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)

	claims["authorized"] = true
	claims["username"] = username
	claims["exp"] = time.Now().Add(time.Hour * 8).Unix()

	return token.SignedString(jwtSecretKey())
}

func validateToken(r *http.Request) (err error) {
	var t string
	if r.Header["Token"] == nil {
		t = r.URL.Query().Get("Token")
	} else {
		t = r.Header["Token"][0]
	}
	if t == "" {
		return errors.New("can not find token in header")
	}

	_, err = jwt.Parse(t,
		func(_ *jwt.Token) (any, error) {
			return jwtSecretKey(), nil
		},
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	return err
}
