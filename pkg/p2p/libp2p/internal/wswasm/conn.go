package wswasm

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/coder/websocket"
)

type websocketNetConn struct {
	conn    *websocket.Conn
	raddr   net.Addr
	laddr   net.Addr
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

	// Read next message
	msgType, data, err := w.conn.Read(w.readCtx)
	if err != nil {
		return 0, err
	}
	if msgType != websocket.MessageBinary {
		return 0, fmt.Errorf("unexpected message type: %v", msgType)
	}

	w.readBuf = data
	w.readPos = 0

	// Recurse to pull from the new buffer
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

func (w *websocketNetConn) LocalAddr() net.Addr {
	return w.laddr
}

func (w *websocketNetConn) RemoteAddr() net.Addr {
	return w.raddr
}

// WASM websockets don’t support deadlines, just stub
func (w *websocketNetConn) SetDeadline(t time.Time) error      { return nil }
func (w *websocketNetConn) SetReadDeadline(t time.Time) error  { return nil }
func (w *websocketNetConn) SetWriteDeadline(t time.Time) error { return nil }
