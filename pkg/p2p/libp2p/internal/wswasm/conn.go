package wswasm

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/coder/websocket"
	"github.com/libp2p/go-libp2p/core/network"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

type websocketNetConn struct {
	conn    *websocket.Conn
	laddr   ma.Multiaddr
	raddr   ma.Multiaddr
	readBuf []byte
	readPos int
	readCtx context.Context
}

// Ensure it satisfies net.Conn
var _ net.Conn = (*websocketNetConn)(nil)

func (w *websocketNetConn) Read(p []byte) (int, error) {
	// Drain the buffer if there's still unread data
	if w.readPos < len(w.readBuf) {
		n := copy(p, w.readBuf[w.readPos:])
		w.readPos += n
		return n, nil
	}
	for {
		// Read next message
		msgType, data, err := w.conn.Read(w.readCtx)
		if err != nil {
			return 0, err
		}
		if msgType != websocket.MessageBinary {
			// Skip non-binary frames (e.g., ping/pong/text)
			continue
		}
		if len(data) == 0 {
			// Skip empty binary frames
			continue
		}
		w.readBuf = data
		w.readPos = 0
		break
	}

	// Now buffer has data, so read from it
	return w.Read(p)
}

func (w *websocketNetConn) Write(p []byte) (int, error) {
	err := w.conn.Write(w.readCtx, websocket.MessageBinary, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *websocketNetConn) Close() error {
	return w.conn.Close(websocket.StatusNormalClosure, "closing")
}

func (w *websocketNetConn) LocalMultiaddr() ma.Multiaddr {
	return w.laddr
}

func (w *websocketNetConn) RemoteMultiaddr() ma.Multiaddr {
	return w.raddr
}

func (w *websocketNetConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1634}
}

func (w *websocketNetConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1634}
}

// WASM websockets don’t support deadlines, just stub
func (w *websocketNetConn) SetDeadline(t time.Time) error      { return nil }
func (w *websocketNetConn) SetReadDeadline(t time.Time) error  { return nil }
func (w *websocketNetConn) SetWriteDeadline(t time.Time) error { return nil }

func newConn(raw *websocket.Conn, scope network.ConnManagementScope, ctx context.Context, rna net.TCPAddr, lna net.TCPAddr) *websocketNetConn {
	laddr, err := manet.FromNetAddr(&lna)

	if err != nil {
		fmt.Errorf("BUG: invalid localaddr on websocket conn", lna)
		return nil
	}
	raddr, err := manet.FromNetAddr(&rna)
	if err != nil {
		fmt.Errorf("BUG: invalid remoteaddr on websocket conn", rna)
		return nil
	}

	c := &websocketNetConn{
		conn:    raw,
		raddr:   raddr,
		laddr:   laddr,
		readCtx: ctx,
	}

	return c
}
