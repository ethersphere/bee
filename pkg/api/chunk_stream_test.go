// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/postage"
	mockbatchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	testingpostage "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/websocket"
)

// nolint:paralleltest
func TestChunkUploadStream(t *testing.T) {
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)

	var (
		storerMock               = mockstorer.New()
		_, wsConn, _, chanStorer = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			WsPath:       "/chunks/stream",
			WsHeaders:    wsHeaders,
			DirectUpload: true,
		})
	)

	t.Run("upload and verify", func(t *testing.T) {
		chsToGet := make([]swarm.Chunk, 0, 5)
		for range 5 {
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
			err := spinlock.Wait(100*time.Millisecond, func() bool { return chanStorer.Has(c.Address()) })
			if err != nil {
				t.Fatal(err)
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

// nolint:paralleltest
func TestChunkUploadStreamWithStamp(t *testing.T) {
	// Generate signer and batch for pre-signed stamps
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	owner, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}

	// Generate chunks and their pre-signed stamps
	chunks := make([]swarm.Chunk, 5)
	stampBytes := make([][]byte, 5)

	for i := range 5 {
		chunks[i] = testingc.GenerateTestRandomChunk()
		stamp := testingpostage.MustNewValidStamp(signer, chunks[i].Address())
		sb, err := stamp.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		stampBytes[i] = sb
	}

	// Mock batch store: accept all batch IDs and return a batch with the correct owner
	batchStore := mockbatchstore.New(
		mockbatchstore.WithAcceptAllExistsFunc(),
		mockbatchstore.WithBatch(&postage.Batch{
			Owner: owner.Bytes(),
		}),
	)

	// No Swarm-Postage-Batch-Id header — triggers per-chunk stamp mode
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")

	var (
		storerMock               = mockstorer.New()
		_, wsConn, _, chanStorer = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			BatchStore:   batchStore,
			WsPath:       "/chunks/stream",
			WsHeaders:    wsHeaders,
			DirectUpload: true,
		})
	)

	t.Run("upload with pre-signed stamps", func(t *testing.T) {
		for i := range 5 {
			// Prepend stamp bytes to chunk data
			msg := append(stampBytes[i], chunks[i].Data()...)

			err := wsConn.SetWriteDeadline(time.Now().Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}

			err = wsConn.WriteMessage(websocket.BinaryMessage, msg)
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
		}

		for _, c := range chunks {
			err := spinlock.Wait(100*time.Millisecond, func() bool { return chanStorer.Has(c.Address()) })
			if err != nil {
				t.Fatal(err)
			}
		}
	})
}

// nolint:paralleltest
func TestChunkUploadStreamInvalidStamp(t *testing.T) {
	// No Swarm-Postage-Batch-Id header — triggers per-chunk stamp mode
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")

	var (
		storerMock      = mockstorer.New()
		_, wsConn, _, _ = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			WsPath:       "/chunks/stream",
			WsHeaders:    wsHeaders,
			DirectUpload: true,
		})
	)

	t.Run("message too small for stamp", func(t *testing.T) {
		// Send a message smaller than StampSize + SpanSize
		tooSmall := make([]byte, postage.StampSize)

		err := wsConn.SetWriteDeadline(time.Now().Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}

		err = wsConn.WriteMessage(websocket.BinaryMessage, tooSmall)
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
		} else if cerr.Text != "message too small for stamp + chunk" {
			t.Fatalf("incorrect response on error, exp: (message too small for stamp + chunk) got (%s)", cerr.Text)
		}
	})
}
