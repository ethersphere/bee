package api

import (
	"context"
	"net/http"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  swarm.ChunkSize,
		WriteBufferSize: swarm.ChunkSize,
	}

	writeDeadline = 4 * time.Second // write deadline. should be smaller than the shutdown timeout on api close

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

type PssMessage struct {
	Topic   string
	Message string
}

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
	s.wsWg.Add(1)
	go s.pumpWs(conn, t)
}

func (s *server) pumpWs(conn *websocket.Conn, topic string) {
	defer s.wsWg.Done()

	var (
		dataC  = make(chan []byte)
		gone   = make(chan struct{})
		topic  = trojan.NewTopic(t)
		ticker = time.NewTicker(pingPeriod)
		err    error
	)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()
	cleanup := s.Pss.Register(topic, func(ctx context.Context, m *trojan.Message) error {
		select {
		case dataC <- m.Payload:
		default:
		}
		return nil
	})

	defer cleanup()

	conn.SetCloseHandler(func(code int, text string) error {
		s.logger.Debugf("pss handler: client gone. code %d message %s", code, text)
		close(gone)
		return nil
	})

	for {
		select {
		case b <- dataC:
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debugf("pss set write deadline: %v", err)
				return
			}

			err = conn.WriteMessage(websocket.BinaryMessage, b)
			if err != nil {
				s.logger.Debugf("pss write to websocket: %v", err)
				return
			}

		case <-s.quit:
			// shutdown
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debugf("pss set write deadline: %v", err)
				return
			}
			err = conn.WriteMessage(websocket.CloseMessage, []byte{})
			if err != nil {
				s.logger.Debugf("pss write close message: %v", err)
			}
			return
		case <-gone:
			// client gone
			return
		case <-ticker.C:
			err = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err != nil {
				s.logger.Debugf("pss set write deadline: %v", err)
				return
			}
			if err = conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}

	}
}

//if err != nil {
//if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
//log.Printf("error: %v", err)
//}
//break
//}
