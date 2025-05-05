package webserver

import (
	"net/http"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write the file to the client.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the client.
	pongWait = 60 * time.Second

	// Send pings to client with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

func reader(ws *websocket.Conn) {
	defer ws.Close()
	ws.SetReadLimit(512)
	err := ws.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		return
	}
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(pongWait)) })
	for {
		_, _, err = ws.ReadMessage()
		if err != nil {
			break
		}
	}
}

func writer(ws *websocket.Conn, l log.LoggerHooker) {
	pingTicker := time.NewTicker(pingPeriod)
	defer func() {
		pingTicker.Stop()
		ws.Close()
	}()
	msgChan := make(log.MessageChanType)
	l.AddSubscriber(msgChan)
	defer l.RemoveSubscriber(msgChan)
	for {
		select {
		case msg := <-msgChan:
			if ws.SetWriteDeadline(time.Now().Add(writeWait)) != nil ||
				ws.WriteMessage(websocket.TextMessage, []byte(msg)) != nil {
				return
			}
		case <-pingTicker.C:
			if ws.SetWriteDeadline(time.Now().Add(writeWait)) != nil ||
				ws.WriteMessage(websocket.PingMessage, []byte{}) != nil {
				return
			}
		}
	}
}

func (Server *WebUIServer) serveWsLog(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		Server.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	Server.WithField("reguest", r.URL.String()).Debugf("established websocket connection")
	l, _ := Server.Logger.(log.LoggerHooker)
	go writer(ws, l)
	reader(ws)
}
