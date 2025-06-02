package webserver

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
)

var json = jsoniter.ConfigFastest

func TestIsCorrectPassword(t *testing.T) {
	ts := &WebUIServer{CmdOpts: CmdOpts{WebUser: "user", WebPassword: "pass"}}
	assert.True(t, ts.IsCorrectPassword(loginReq{Username: "user", Password: "pass"}))
	assert.False(t, ts.IsCorrectPassword(loginReq{Username: "user", Password: "wrong"}))
	assert.True(t, (&WebUIServer{}).IsCorrectPassword(loginReq{})) // empty user/pass disables auth
}

func TestHandleLogin_POST_Success(t *testing.T) {
	ts := &WebUIServer{CmdOpts: CmdOpts{WebUser: "user", WebPassword: "pass"}}
	body, _ := json.Marshal(map[string]string{"user": "user", "password": "pass"})
	r := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ts.handleLogin(w, r)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	token, _ := io.ReadAll(resp.Body)
	assert.NotEmpty(t, string(token))
}

func TestHandleLogin_POST_Fail(t *testing.T) {
	ts := &WebUIServer{CmdOpts: CmdOpts{WebUser: "user", WebPassword: "pass"}}
	body, _ := json.Marshal(map[string]string{"user": "user", "password": "wrong"})
	r := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	ts.handleLogin(w, r)
	resp := w.Result()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHandleLogin_POST_BadJSON(t *testing.T) {
	ts := &WebUIServer{CmdOpts: CmdOpts{WebUser: "user", WebPassword: "pass"}}
	r := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader([]byte("notjson")))
	w := httptest.NewRecorder()
	ts.handleLogin(w, r)
	resp := w.Result()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestHandleLogin_GET(t *testing.T) {
	ts := &WebUIServer{}
	r := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	ts.handleLogin(w, r)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "only POST methods is allowed.", string(body))
}

func TestGenerateAndValidateJWT(t *testing.T) {
	token, err := generateJWT("user1")
	assert.NoError(t, err)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Token", token)
	assert.NoError(t, validateToken(r))
}

func TestValidateToken_MissingToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	err := validateToken(r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "can not find token")
}

func TestValidateToken_InvalidToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Token", "invalidtoken")
	err := validateToken(r)
	assert.Error(t, err)
}

func TestEnsureAuth_ServeHTTP(t *testing.T) {
	called := false
	h := func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}
	token, _ := generateJWT("user1")
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Token", token)
	w := httptest.NewRecorder()
	NewEnsureAuth(h).ServeHTTP(w, r)
	resp := w.Result()
	assert.Equal(t, http.StatusTeapot, resp.StatusCode)
	assert.True(t, called)
}

func TestEnsureAuth_ServeHTTP_InvalidToken(t *testing.T) {
	h := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Token", "invalidtoken")
	w := httptest.NewRecorder()
	NewEnsureAuth(h).ServeHTTP(w, r)
	resp := w.Result()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestJWT_Expiration(t *testing.T) {
	tok := jwt.New(jwt.SigningMethodHS256)
	claims := tok.Claims.(jwt.MapClaims)
	claims["authorized"] = true
	claims["username"] = "user"
	claims["exp"] = time.Now().Add(-time.Hour).Unix() // expired
	token, _ := tok.SignedString(sampleSecretKey)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Token", token)
	err := validateToken(r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token is expired")
}
