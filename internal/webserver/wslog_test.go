package webserver

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeWsLog_UpgradeError(t *testing.T) {
	ts := &WebUIServer{Logger: log.NewNoopLogger()}
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
	stopChan := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopChan:
				return
			default:
				ts.Info("Test message")
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	// Set a 5-second deadline for the initial read to succeed
	assert.NoError(t, ws.SetReadDeadline(time.Now().Add(5*time.Second)))

	var msgType int
	var msg []byte

	// Block and wait to receive the expected message
	for {
		tpe, m, err := ws.ReadMessage()
		require.NoError(t, err, "Websocket read failed or timed out")

		if strings.Contains(string(m), "Test message") {
			msgType = tpe
			msg = m
			break
		}
	}

	// Stop the background log spammer now that we caught our message
	close(stopChan)
	assert.Equal(t, websocket.TextMessage, msgType)
	assert.NotEmpty(t, msg)
	assert.Contains(t, string(msg), "Test message")
	time.Sleep(4 * time.Second)
	// check if the connection is closed
	assert.NoError(t, ws.Close())
	ts.Info("Test message after websocket close")
	assert.Error(t, ws.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, _, err = ws.ReadMessage()
	assert.Error(t, err, "should error because connection is closed")
}
