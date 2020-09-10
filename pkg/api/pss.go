package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  swarm.ChunkSize,
	WriteBufferSize: swarm.ChunkSize,
}

type PssMessage struct {
	Topic   string
	Message string
}

// Client is a middleman between the websocket connection and the hub.
//type Client struct {
//hub *Hub

//// The websocket connection.
//conn *websocket.Conn

//// Buffered channel of outbound messages.
//send chan []byte
//}

// serveWs handles websocket requests from the peer.
//func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
//conn, err := upgrader.Upgrade(w, r, nil)
//if err != nil {
//log.Println(err)
//return
//}
//client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256)}
//client.hub.register <- client

//// Allow collection of memory referenced by the caller by doing all work in
//// new goroutines.
//go client.writePump()
//go client.readPump()
//}
func (s *server) pssPostHandler(w http.ResponseWriter, r *http.Request) {

}

func (s *server) pssWsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Debugf("pss ws: upgrade: %v", err)
		logger.Error("pss ws: cannot upgrade")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	t, ok := mux.Vars(r)["topic"]
	if !ok {
		logger.Error("pss ws: no topic")
		jsonhttp.BadRequest(w, nil)
		return
	}

	//if err != nil {
	//if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
	//log.Printf("error: %v", err)
	//}
	//break
	//}
	s.wsWg.Add(1)
	go s.pumpWs(conn, t)
}

func (s *server) pumpWs(conn *websocket.Conn, topic string) {
	dataC := make(chan []byte)

	defer s.wsWg.Done()
	defer func() { close(dataC) }()

	handler := func(ctx context.Context, m *trojan.Message) error {
		dataC <- m.Payload
		return nil
	}
	topic := trojan.NewTopic(t)
	s.Pss.Register(topic, handler)

	for {
		select {
		case b <- dataC:
			err := conn.WriteMessage(websocket.BinaryMessage, b)
			if err != nil {
				// terminate this websocket handler
				s.logger.Debugf("pss write to websocket: %v. terminating connection", err)
				return
			}
		}
	}
	fmt.Println(topic)
	fmt.Println(conn)

}
