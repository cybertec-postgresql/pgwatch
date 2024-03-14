package webserver_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/webserver"
	"github.com/stretchr/testify/assert"
)

const host = "http://localhost:8080"

type Credentials struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

func TestStatus(t *testing.T) {
	restsrv := webserver.Init(config.WebUIOpts{WebAddr: "127.0.0.1:8080"}, os.DirFS("../webui/build"), nil, log.FallbackLogger)
	assert.NotNil(t, restsrv)
	// r, err := http.Get("http://localhost:8080/")
	// assert.NoError(t, err)
	// assert.Equal(t, http.StatusOK, r.StatusCode)
	// b, err := io.ReadAll(r.Body)
	// assert.NoError(t, err)
	// assert.True(t, len(b) > 0)
}

func TestServerNoAuth(t *testing.T) {
	restsrv := webserver.Init(config.WebUIOpts{WebAddr: host}, os.DirFS("../webui/build"), nil, log.FallbackLogger)
	assert.NotNil(t, restsrv)
	rr := httptest.NewRecorder()
	// test request metrics
	req_metric, err := http.NewRequest("GET", host+"/metric", nil)
	restsrv.Handler.ServeHTTP(rr, req_metric)
	assert.Equal(t, err, nil)
	assert.Equal(t, rr.Code, http.StatusUnauthorized, "REQUEST WITHOUT AUTHENTICATION")

	// test request database
	req_db, err := http.NewRequest("GET", host+"/db", nil)
	assert.Equal(t, err, nil)
	restsrv.Handler.ServeHTTP(rr, req_db)
	assert.Equal(t, rr.Code, http.StatusUnauthorized, "REQUEST WITHOUT AUTHENTICATION")

	// test request stats
	req_stats, err := http.NewRequest("GET", host+"/stats", nil)
	assert.Equal(t, err, nil)
	restsrv.Handler.ServeHTTP(rr, req_stats)
	assert.Equal(t, rr.Code, http.StatusUnauthorized, "REQUEST WITHOUT AUTHENTICATION")

	// test request
	req_log, err := http.NewRequest("GET", host+"/log", nil)
	assert.Equal(t, err, nil)
	restsrv.Handler.ServeHTTP(rr, req_log)
	assert.Equal(t, rr.Code, http.StatusUnauthorized, "REQUEST WITHOUT AUTHENTICATION")

	// request metrics
	req_connect, err := http.NewRequest("GET", host+"/test-connect", nil)
	assert.Equal(t, err, nil)
	restsrv.Handler.ServeHTTP(rr, req_connect)
	assert.Equal(t, rr.Code, http.StatusUnauthorized, "REQUEST WITHOUT AUTHENTICATION")

}

func TestGetToken(t *testing.T) {
	restsrv := webserver.Init(config.WebUIOpts{WebAddr: "127.0.0.1:8080"}, os.DirFS("../webui/build"), nil, log.FallbackLogger)
	rr := httptest.NewRecorder()

	credentials := Credentials{
		User:     "admin",
		Password: "admin",
	}

	payload, err := json.Marshal(credentials)
	if err != nil {
		fmt.Println("Error marshaling ", err)
	}

	req_token, err := http.NewRequest("POST", host+"/login", strings.NewReader(string(payload)))
	assert.Equal(t, err, nil)

	restsrv.Handler.ServeHTTP(rr, req_token)

	assert.Equal(t, rr.Code, http.StatusOK, "TOKEN RESPONSE OK")

	token, err := io.ReadAll(rr.Body)
	fmt.Println(string(token))
	assert.Equal(t, err, nil)
	assert.NotEqual(t, token, nil)

}
