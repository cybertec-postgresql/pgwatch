package webserver_test

import (
	"os"
	"testing"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/webserver"
	"github.com/stretchr/testify/assert"
)

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
