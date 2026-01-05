package webserver_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jsoniter "github.com/json-iterator/go"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/webserver"
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

func TestWebDisableOpt(t *testing.T) {
	var ready ReadyBool
	restsrv, err := webserver.Init(context.Background(), webserver.CmdOpts{WebDisable: "all"}, nil, nil, &ready)
	assert.Nil(t, restsrv, "no webserver should be started")
	assert.NoError(t, err)

	restsrv, err = webserver.Init(context.Background(), webserver.CmdOpts{WebAddr: "127.0.0.1:8079", WebDisable: "ui"}, nil, nil, &ready)
	assert.NotNil(t, restsrv)
	assert.NoError(t, err)
	r, err := http.Get("http://localhost:8079/")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, r.StatusCode, "no webui should be served")
	r, err = http.Get("http://localhost:8079/liveness")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, r.StatusCode, "rest api should be served though")

	restsrv, err = webserver.Init(context.Background(), webserver.CmdOpts{WebAddr: "127.0.0.1:8079"}, nil, nil, &ready)
	assert.Nil(t, restsrv)
	assert.Error(t, err, "port should be in use")
}

func TestHealth(t *testing.T) {
	var ready ReadyBool
	ctx, cancel := context.WithCancel(context.Background())
	restsrv, _ := webserver.Init(ctx, webserver.CmdOpts{WebAddr: "127.0.0.1:8080"}, nil, nil, &ready)
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
	restsrv, _ := webserver.Init(context.Background(), webserver.CmdOpts{WebAddr: "localhost:8081"}, nil, nil, nil)
	assert.NotNil(t, restsrv)
	rr := httptest.NewRecorder()
	// cors OPTIONS
	reqOpts, err := http.NewRequest("OPTIONS", host, nil)
	assert.NoError(t, err)
	restsrv.Handler.ServeHTTP(rr, reqOpts)
	assert.Equal(t, http.StatusOK, rr.Code)

	// test request metrics
	rr = httptest.NewRecorder()
	reqMetric, err := http.NewRequest("GET", host+"/metric", nil)
	restsrv.Handler.ServeHTTP(rr, reqMetric)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "REQUEST WITHOUT AUTHENTICATION")

	// test request database
	rr = httptest.NewRecorder()
	reqDb, err := http.NewRequest("GET", host+"/source", nil)
	assert.NoError(t, err)
	restsrv.Handler.ServeHTTP(rr, reqDb)
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "REQUEST WITHOUT AUTHENTICATION")

	// test request
	rr = httptest.NewRecorder()
	reqLog, err := http.NewRequest("GET", host+"/log", nil)
	assert.NoError(t, err)
	restsrv.Handler.ServeHTTP(rr, reqLog)
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "REQUEST WITHOUT AUTHENTICATION")

	// request metrics
	rr = httptest.NewRecorder()
	reqConnect, err := http.NewRequest("GET", host+"/test-connect", nil)
	assert.NoError(t, err)
	restsrv.Handler.ServeHTTP(rr, reqConnect)
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "REQUEST WITHOUT AUTHENTICATION")

}

func TestGetToken(t *testing.T) {
	host := "http://localhost:8082"
	restsrv, _ := webserver.Init(context.Background(), webserver.CmdOpts{WebAddr: "localhost:8082"}, nil, nil, nil)
	rr := httptest.NewRecorder()

	credentials := Credentials{
		User:     "admin",
		Password: "admin",
	}

	payload, err := jsoniter.ConfigFastest.Marshal(credentials)
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
