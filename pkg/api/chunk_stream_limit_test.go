// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/postage"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/websocket"
)

// TestChunkUploadStreamReadLimit checks that the chunk stream rejects a message
// larger than any valid chunk instead of buffering it in memory.
// nolint:paralleltest
func TestChunkUploadStreamReadLimit(t *testing.T) {
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       mockstorer.New(),
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    wsHeaders,
		DirectUpload: true,
	})

	// A regular chunk is still accepted.
	ch := testingc.GenerateTestRandomChunk()
	if err := wsConn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := wsConn.WriteMessage(websocket.BinaryMessage, ch.Data()); err != nil {
		t.Fatal(err)
	}
	if err := wsConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	mt, msg, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if mt != websocket.BinaryMessage || !bytes.Equal(msg, api.SuccessWsMsg) {
		t.Fatal("invalid response", mt, string(msg))
	}

	// One byte over the largest valid message is rejected.
	oversized := make([]byte, postage.StampSize+swarm.SocMaxChunkSize+1)
	if err := wsConn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := wsConn.WriteMessage(websocket.BinaryMessage, oversized); err != nil {
		t.Fatal(err)
	}
	if err := wsConn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, _, err = wsConn.ReadMessage()
	var cerr *websocket.CloseError
	if !errors.As(err, &cerr) {
		t.Fatalf("expected close error, got %v", err)
	}
	if cerr.Code != websocket.CloseMessageTooBig {
		t.Fatalf("expected close code %d, got %d (%s)", websocket.CloseMessageTooBig, cerr.Code, cerr.Text)
	}
}
