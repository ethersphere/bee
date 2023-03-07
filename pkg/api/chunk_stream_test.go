// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/api"
	mockstorer "github.com/ethersphere/bee/pkg/localstorev2/mock"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/websocket"
)

// nolint:paralleltest
func TestChunkUploadStream(t *testing.T) {
	wsHeaders := http.Header{}
	wsHeaders.Set(api.SwarmDeferredUploadHeader, "true")
	wsHeaders.Set("Content-Type", "application/octet-stream")
	wsHeaders.Set("Swarm-Postage-Batch-Id", batchOkStr)

	var (
		storerMock      = mockstorer.New()
		_, wsConn, _, _ = newTestServer(t, testServerOptions{
			Storer:    storerMock,
			Post:      mockpost.New(mockpost.WithAcceptAll()),
			WsPath:    "/chunks/stream",
			WsHeaders: wsHeaders,
		})
	)

	t.Run("upload and verify", func(t *testing.T) {
		chsToGet := []swarm.Chunk{}
		for i := 0; i < 5; i++ {
			ch := testingc.GenerateTestRandomChunk()

			err := wsConn.SetWriteDeadline(time.Now().Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}

			err = wsConn.WriteMessage(websocket.BinaryMessage, ch.Data())
			if err != nil {
				t.Fatal(err)
			}

			err = wsConn.SetReadDeadline(time.Now().Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}

			mt, msg, err := wsConn.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}

			if mt != websocket.BinaryMessage || !bytes.Equal(msg, api.SuccessWsMsg) {
				t.Fatal("invalid response", mt, string(msg))
			}

			chsToGet = append(chsToGet, ch)
		}

		for _, c := range chsToGet {
			ch, err := storerMock.ChunkStore().Get(context.Background(), c.Address())
			if err != nil {
				t.Fatal("failed to get chunk after upload", err)
			}
			if !ch.Equal(c) {
				t.Fatal("invalid chunk read")
			}
		}
	})

	t.Run("close on incorrect msg", func(t *testing.T) {
		err := wsConn.SetWriteDeadline(time.Now().Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}

		err = wsConn.WriteMessage(websocket.TextMessage, []byte("incorrect msg"))
		if err != nil {
			t.Fatal(err)
		}

		err = wsConn.SetReadDeadline(time.Now().Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = wsConn.ReadMessage()
		if err == nil {
			t.Fatal("expected failure on read")
		}
		// nolint:errorlint
		if cerr, ok := err.(*websocket.CloseError); !ok {
			t.Fatal("invalid error on read")
		} else if cerr.Text != "invalid message" {
			t.Fatalf("incorrect response on error, exp: (invalid message) got (%s)", cerr.Text)
		}
	})
}
