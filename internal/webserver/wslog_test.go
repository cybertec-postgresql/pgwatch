package webserver

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeWsLog_UpgradeError(t *testing.T) {
	ts := &WebUIServer{Logger: log.FallbackLogger}
	r := httptest.NewRequest(http.MethodGet, "/wslog", nil)
	w := httptest.NewRecorder()
	// No websocket headers, should fail to upgrade
	ts.serveWsLog(w, r)
	resp := w.Result()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestServeWsLog_Success(t *testing.T) {
	ts := &WebUIServer{Logger: log.Init(log.CmdOpts{LogLevel: "debug"})}
	h := http.HandlerFunc(ts.serveWsLog)
	tsServer := httptest.NewServer(h)
	defer tsServer.Close()

	u := "ws" + strings.TrimPrefix(tsServer.URL, "http")
	uParsed, _ := url.Parse(u)
	uParsed.Path = "/wslog"

	ws, resp, err := websocket.DefaultDialer.Dial(uParsed.String(), nil)
	require.NotNil(t, ws)
	require.NoError(t, err)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	// send ping message to keep connection alive
	assert.NoError(t, ws.WriteMessage(websocket.PingMessage, nil))

	// send some log message
	ts.Info("Test message")
	// check output though the websocket
	assert.NoError(t, ws.SetReadDeadline(time.Now().Add(300*time.Millisecond)))
	msgType, msg, err := ws.ReadMessage()
	assert.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, msgType)
	assert.NotEmpty(t, msg)
	assert.Contains(t, string(msg), "Test message")
	<-time.After(time.Second)
	// check if the connection is closed
	assert.NoError(t, ws.Close())
	assert.Error(t, ws.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
	_, _, err = ws.ReadMessage()
	assert.Error(t, err, "should error because connection is closed")

}
