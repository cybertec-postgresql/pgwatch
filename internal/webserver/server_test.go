package webserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/webserver"
	"github.com/stretchr/testify/assert"
)

type Credentials struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

type ReadyBool bool

func (ready *ReadyBool) Ready() bool {
	return bool(*ready)
}

func TestHealth(t *testing.T) {
	var ready ReadyBool
	ctx, cancel := context.WithCancel(context.Background())
	restsrv := webserver.Init(ctx, webserver.CmdOpts{WebAddr: "127.0.0.1:8080"}, os.DirFS("../webui/build"), nil, nil, &ready)
	assert.NotNil(t, restsrv)

	r, err := http.Get("http://localhost:8080/liveness")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, r.StatusCode)

	cancel()
	r, err = http.Get("http://localhost:8080/liveness")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, r.StatusCode)

	r, err = http.Get("http://localhost:8080/readiness")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, r.StatusCode)

	ready = true
	r, err = http.Get("http://localhost:8080/readiness")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, r.StatusCode)
}

func TestServerNoAuth(t *testing.T) {
	host := "http://localhost:8081"
	restsrv := webserver.Init(context.Background(), webserver.CmdOpts{WebAddr: "localhost:8081"}, os.DirFS("../webui/build"), nil, nil, nil)
	assert.NotNil(t, restsrv)
	rr := httptest.NewRecorder()
	// test request metrics
	reqMetric, err := http.NewRequest("GET", host+"/metric", nil)
	restsrv.Handler.ServeHTTP(rr, reqMetric)
	assert.Equal(t, err, nil)
	assert.Equal(t, rr.Code, http.StatusUnauthorized, "REQUEST WITHOUT AUTHENTICATION")

	// test request database
	reqDb, err := http.NewRequest("GET", host+"/db", nil)
	assert.Equal(t, err, nil)
	restsrv.Handler.ServeHTTP(rr, reqDb)
	assert.Equal(t, rr.Code, http.StatusUnauthorized, "REQUEST WITHOUT AUTHENTICATION")

	// test request
	reqLog, err := http.NewRequest("GET", host+"/log", nil)
	assert.Equal(t, err, nil)
	restsrv.Handler.ServeHTTP(rr, reqLog)
	assert.Equal(t, rr.Code, http.StatusUnauthorized, "REQUEST WITHOUT AUTHENTICATION")

	// request metrics
	reqConnect, err := http.NewRequest("GET", host+"/test-connect", nil)
	assert.Equal(t, err, nil)
	restsrv.Handler.ServeHTTP(rr, reqConnect)
	assert.Equal(t, rr.Code, http.StatusUnauthorized, "REQUEST WITHOUT AUTHENTICATION")

}

func TestGetToken(t *testing.T) {
	host := "http://localhost:8082"
	restsrv := webserver.Init(context.Background(), webserver.CmdOpts{WebAddr: "localhost:8082"}, os.DirFS("../webui/build"), nil, nil, nil)
	rr := httptest.NewRecorder()

	credentials := Credentials{
		User:     "admin",
		Password: "admin",
	}

	payload, err := json.Marshal(credentials)
	if err != nil {
		fmt.Println("Error marshaling ", err)
	}

	reqToken, err := http.NewRequest("POST", host+"/login", strings.NewReader(string(payload)))
	assert.Equal(t, err, nil)

	restsrv.Handler.ServeHTTP(rr, reqToken)

	assert.Equal(t, rr.Code, http.StatusOK, "TOKEN RESPONSE OK")

	token, err := io.ReadAll(rr.Body)
	fmt.Println(string(token))
	assert.Equal(t, err, nil)
	assert.NotEqual(t, token, nil)

}
