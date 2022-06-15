package webui_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/cybertec-postgresql/pgwatch3/webui"
	"github.com/stretchr/testify/assert"
)

func TestStatus(t *testing.T) {
	restsrv := webui.Init("127.0.0.1:8080")
	assert.NotNil(t, restsrv)

	r, err := http.Get("http://localhost:8080/")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, r.StatusCode)
	b, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, "Hello world!", string(b))
}
