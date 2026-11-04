// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/gorilla/websocket"
)

// An upload stream whose client is idle must not hold up shutdown: the service
// closes it with 1001 instead of waiting for a message that never comes.
// newTestServer closes the service during cleanup and fails the test if Close
// reports open websockets.
// nolint:paralleltest
func TestChunkUploadStreamShutdownWithIdleClient(t *testing.T) {
	var wsConn *websocket.Conn

	// Registered before the server, so it runs after the service is closed.
	t.Cleanup(func() {
		if wsConn == nil {
			return
		}
		defer wsConn.Close()

		_ = wsConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, _, err := wsConn.ReadMessage()
		var cerr *websocket.CloseError
		if !errors.As(err, &cerr) || cerr.Code != websocket.CloseGoingAway {
			t.Errorf("expected close %d on shutdown, got %v", websocket.CloseGoingAway, err)
		}
	})

	_, _, listenAddr, _, _ := newTestServer(t, testServerOptions{
		Storer:       mockstorer.New(),
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		DirectUpload: true,
	})

	h := http.Header{}
	h.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	conn, _, err := websocket.DefaultDialer.Dial("ws://"+listenAddr+"/chunks/stream", h)
	if err != nil {
		t.Fatal(err)
	}
	wsConn = conn

	// Upload one chunk so the stream is up, then leave it idle.
	if err := wsConn.WriteMessage(websocket.BinaryMessage, testingc.GenerateTestRandomChunk().Data()); err != nil {
		t.Fatal(err)
	}
	_ = wsConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := wsConn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
}
