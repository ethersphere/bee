// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	mockbatchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	testingpostage "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/gorilla/websocket"
)

// downloadRequest builds a download request frame: [opcode][32-byte address]...
func downloadRequest(addrs ...swarm.Address) []byte {
	req := make([]byte, 0, 1+len(addrs)*swarm.HashSize)
	req = append(req, api.ChunkDownloadOpcode)
	for _, a := range addrs {
		req = append(req, a.Bytes()...)
	}
	return req
}

// streamTestTimeout bounds how long a test waits for a websocket response. It is
// generous on purpose: these tests share a machine with the rest of the suite and
// run under the race detector on CI.
const streamTestTimeout = 10 * time.Second

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

func TestChunkDownloadStream_Subprotocol(t *testing.T) {
	t.Parallel()

	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	// Seed test chunks
	chunks := make([]swarm.Chunk, 3)
	for i := range 3 {
		chunks[i] = testingc.GenerateTestRandomChunk()
		err := cs.Put(context.Background(), chunks[i])
		if err != nil {
			t.Fatal(err)
		}
	}

	dialer := &websocket.Dialer{
		Subprotocols: []string{api.ChunkDownloadSubprotocol},
	}
	wsConn, resp, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer wsConn.Close()

	if resp.Header.Get("Sec-WebSocket-Protocol") != api.ChunkDownloadSubprotocol {
		t.Fatalf("expected subprotocol %s, got %s", api.ChunkDownloadSubprotocol, resp.Header.Get("Sec-WebSocket-Protocol"))
	}

	// Request the first two chunks with raw 32-byte addresses; the third is
	// seeded but never requested, so it must not come back.
	requested := chunks[:2]
	for _, c := range requested {
		err = wsConn.WriteMessage(websocket.BinaryMessage, downloadRequest(c.Address()))
		if err != nil {
			t.Fatal(err)
		}
	}

	// Read and verify responses
	for range 2 {
		_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
		mt, msg, err := wsConn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if mt != websocket.BinaryMessage {
			t.Fatalf("expected binary message, got %v", mt)
		}
		if len(msg) < 1+swarm.HashSize {
			t.Fatalf("response too short: %d", len(msg))
		}
		status := msg[0]
		if status != api.WsChunkDeliverySuccess {
			t.Fatalf("expected status 0 (success), got %d", status)
		}
		respAddr := swarm.NewAddress(msg[1 : 1+swarm.HashSize])
		data := msg[1+swarm.HashSize:]

		var matched bool
		for _, c := range requested {
			if c.Address().Equal(respAddr) {
				matched = true
				if !bytes.Equal(data, c.Data()) {
					t.Fatalf("data mismatch for %s", respAddr)
				}
				break
			}
		}
		if !matched {
			t.Fatalf("unexpected chunk address in response: %s", respAddr)
		}
	}
}

func TestChunkDownloadStream_ModeQueryParam(t *testing.T) {
	t.Parallel()

	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	chunk := testingc.GenerateTestRandomChunk()
	err := cs.Put(context.Background(), chunk)
	if err != nil {
		t.Fatal(err)
	}

	wsConn, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/chunks/stream?mode=download", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer wsConn.Close()

	err = wsConn.WriteMessage(websocket.BinaryMessage, downloadRequest(chunk.Address()))
	if err != nil {
		t.Fatal(err)
	}

	_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
	mt, msg, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("expected binary message, got %v", mt)
	}
	if msg[0] != api.WsChunkDeliverySuccess {
		t.Fatalf("expected success, got %d", msg[0])
	}
	if !bytes.Equal(msg[1+swarm.HashSize:], chunk.Data()) {
		t.Fatal("chunk data mismatch")
	}
}

func TestChunkDownloadStream_NotFound(t *testing.T) {
	t.Parallel()

	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	dialer := &websocket.Dialer{
		Subprotocols: []string{api.ChunkDownloadSubprotocol},
	}
	wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer wsConn.Close()

	nonExistent := swarm.RandAddress(t)
	err = wsConn.WriteMessage(websocket.BinaryMessage, downloadRequest(nonExistent))
	if err != nil {
		t.Fatal(err)
	}

	_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
	_, msg, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if len(msg) != 1+swarm.HashSize {
		t.Fatalf("expected 33 bytes response, got %d", len(msg))
	}
	if msg[0] != api.WsChunkDeliveryNotFound {
		t.Fatalf("expected not found status %d, got %d", api.WsChunkDeliveryNotFound, msg[0])
	}
	if !bytes.Equal(msg[1:1+swarm.HashSize], nonExistent.Bytes()) {
		t.Fatal("address mismatch in not found response")
	}

	// Verify connection remains open by requesting an existing chunk
	existing := testingc.GenerateTestRandomChunk()
	err = cs.Put(context.Background(), existing)
	if err != nil {
		t.Fatal(err)
	}

	err = wsConn.WriteMessage(websocket.BinaryMessage, downloadRequest(existing.Address()))
	if err != nil {
		t.Fatal(err)
	}

	_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
	_, msg, err = wsConn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msg[0] != api.WsChunkDeliverySuccess {
		t.Fatalf("expected success on second request, got %d", msg[0])
	}
}

func TestChunkDownloadStream_Concurrent(t *testing.T) {
	t.Parallel()

	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	const count = 20
	chunks := make([]swarm.Chunk, count)
	for i := range count {
		chunks[i] = testingc.GenerateTestRandomChunk()
		err := cs.Put(context.Background(), chunks[i])
		if err != nil {
			t.Fatal(err)
		}
	}

	dialer := &websocket.Dialer{
		Subprotocols: []string{api.ChunkDownloadSubprotocol},
	}
	wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer wsConn.Close()

	// Write all requests concurrently
	for _, c := range chunks {
		err = wsConn.WriteMessage(websocket.BinaryMessage, downloadRequest(c.Address()))
		if err != nil {
			t.Fatal(err)
		}
	}

	received := make(map[string]bool)
	for range count {
		_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
		_, msg, err := wsConn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if msg[0] != api.WsChunkDeliverySuccess {
			t.Fatalf("expected success, got %d", msg[0])
		}
		respAddr := swarm.NewAddress(msg[1 : 1+swarm.HashSize])
		received[respAddr.String()] = true
	}

	if len(received) != count {
		t.Fatalf("expected %d distinct chunks received, got %d", count, len(received))
	}
}

func TestChunkDownloadStream_InvalidMessage(t *testing.T) {
	t.Parallel()

	storerMock := mockstorer.New()
	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	dialer := &websocket.Dialer{
		Subprotocols: []string{api.ChunkDownloadSubprotocol},
	}
	wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer wsConn.Close()

	err = wsConn.WriteMessage(websocket.BinaryMessage, []byte("too-short"))
	if err != nil {
		t.Fatal(err)
	}

	_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
	_, _, err = wsConn.ReadMessage()
	if err == nil {
		t.Fatal("expected failure on read")
	}
	var cerr *websocket.CloseError
	if !errors.As(err, &cerr) {
		t.Fatalf("expected close error, got %v", err)
	}
	if cerr.Text != "invalid message length" {
		t.Fatalf("expected 'invalid message length', got %q", cerr.Text)
	}
}

func TestChunkDownloadStream_MultiAddressBatch(t *testing.T) {
	t.Parallel()

	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	const count = 5
	chunks := make([]swarm.Chunk, count)
	addrs := make([]swarm.Address, count)
	for i := range count {
		chunks[i] = testingc.GenerateTestRandomChunk()
		err := cs.Put(context.Background(), chunks[i])
		if err != nil {
			t.Fatal(err)
		}
		addrs[i] = chunks[i].Address()
	}
	batchReq := downloadRequest(addrs...)

	dialer := &websocket.Dialer{
		Subprotocols: []string{api.ChunkDownloadSubprotocol},
	}
	wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer wsConn.Close()

	// Send all 5 chunk addresses in a single WebSocket message (5 * 32 = 160 bytes)
	err = wsConn.WriteMessage(websocket.BinaryMessage, batchReq)
	if err != nil {
		t.Fatal(err)
	}

	received := make(map[string]bool)
	for range count {
		_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
		_, msg, err := wsConn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if msg[0] != api.WsChunkDeliverySuccess {
			t.Fatalf("expected success, got %d", msg[0])
		}
		respAddr := swarm.NewAddress(msg[1 : 1+swarm.HashSize])
		received[respAddr.String()] = true

		// verify data matches
		for _, c := range chunks {
			if c.Address().Equal(respAddr) {
				if !bytes.Equal(msg[1+swarm.HashSize:], c.Data()) {
					t.Fatalf("chunk data mismatch for %s", respAddr)
				}
			}
		}
	}

	if len(received) != count {
		t.Fatalf("expected %d distinct chunks, got %d", count, len(received))
	}
}

func TestChunkDownloadStream_MixedBatchWithNotFound(t *testing.T) {
	t.Parallel()

	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	// 2 existing chunks
	ch1 := testingc.GenerateTestRandomChunk()
	ch2 := testingc.GenerateTestRandomChunk()
	_ = cs.Put(context.Background(), ch1)
	_ = cs.Put(context.Background(), ch2)

	// 2 non-existent addresses
	missing1 := swarm.RandAddress(t)
	missing2 := swarm.RandAddress(t)

	dialer := &websocket.Dialer{
		Subprotocols: []string{api.ChunkDownloadSubprotocol},
	}
	wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer wsConn.Close()

	// Send all 4 addresses in a single frame
	req := downloadRequest(ch1.Address(), missing1, ch2.Address(), missing2)

	err = wsConn.WriteMessage(websocket.BinaryMessage, req)
	if err != nil {
		t.Fatal(err)
	}

	successCount := 0
	notFoundCount := 0

	for range 4 {
		_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
		_, msg, err := wsConn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		switch msg[0] {
		case api.WsChunkDeliverySuccess:
			successCount++
		case api.WsChunkDeliveryNotFound:
			notFoundCount++
		default:
			t.Fatalf("unexpected status code: %d", msg[0])
		}
	}

	if successCount != 2 || notFoundCount != 2 {
		t.Fatalf("expected 2 successes and 2 not-founds, got %d and %d", successCount, notFoundCount)
	}
}

// nolint:paralleltest // measures runtime.NumGoroutine, must not run alongside other tests
func TestChunkDownloadStream_ClientDisconnectDuringFetch(t *testing.T) {
	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	seededChunks := make([]swarm.Chunk, 30)
	for i := range 30 {
		ch := testingc.GenerateTestRandomChunk()
		if err := cs.Put(context.Background(), ch); err != nil {
			t.Fatal(err)
		}
		seededChunks[i] = ch
	}

	runtime.GC()
	baseline := runtime.NumGoroutine()

	dialer := &websocket.Dialer{
		Subprotocols: []string{api.ChunkDownloadSubprotocol},
	}
	wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Blast 30 chunk requests for seeded chunks
	for i := range 30 {
		_ = wsConn.WriteMessage(websocket.BinaryMessage, downloadRequest(seededChunks[i].Address()))
	}

	// Abruptly close client connection while fetches are queued/running
	if err := wsConn.Close(); err != nil {
		t.Fatal(err)
	}

	// The read loop, the download workers and the connection must all be torn
	// down; if any of them leaks the goroutine count stays above the baseline.
	err = spinlock.Wait(10*time.Second, func() bool {
		return runtime.NumGoroutine() <= baseline+2
	})
	if err != nil {
		t.Fatalf("goroutines did not settle after disconnect: baseline %d, now %d", baseline, runtime.NumGoroutine())
	}
}

func TestChunkDownloadStream_TextMessageRejected(t *testing.T) {
	t.Parallel()

	storerMock := mockstorer.New()
	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	dialer := &websocket.Dialer{
		Subprotocols: []string{api.ChunkDownloadSubprotocol},
	}
	wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer wsConn.Close()

	err = wsConn.WriteMessage(websocket.TextMessage, []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}

	_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
	_, _, err = wsConn.ReadMessage()
	if err == nil {
		t.Fatal("expected failure on text message read")
	}
	var cerr *websocket.CloseError
	if !errors.As(err, &cerr) {
		t.Fatalf("expected close error, got %v", err)
	}
	if cerr.Text != "invalid message" {
		t.Fatalf("expected 'invalid message', got %q", cerr.Text)
	}
}

func TestChunkDownloadStream_OversizedFrameRejected(t *testing.T) {
	t.Parallel()

	storerMock := mockstorer.New()
	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	dial := func(t *testing.T) *websocket.Conn {
		t.Helper()
		dialer := &websocket.Dialer{
			Subprotocols: []string{api.ChunkDownloadSubprotocol},
		}
		wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		return wsConn
	}

	// A batch one address over the limit is still readable, so the server gets to
	// reject it with an explicit reason rather than the bare transport-level close.
	t.Run("batch over the address limit is rejected with a reason", func(t *testing.T) {
		t.Parallel()

		wsConn := dial(t)
		defer wsConn.Close()

		frame := make([]byte, 1+(api.MaxDownloadBatchSize+1)*swarm.HashSize)
		frame[0] = api.ChunkDownloadOpcode
		if err := wsConn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			t.Fatal(err)
		}

		_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
		_, _, err := wsConn.ReadMessage()
		if err == nil {
			t.Fatal("expected failure on over-sized batch")
		}
		var cerr *websocket.CloseError
		if !errors.As(err, &cerr) {
			t.Fatalf("expected close error, got %v", err)
		}
		if cerr.Code != websocket.CloseMessageTooBig {
			t.Fatalf("expected close code %d, got %d", websocket.CloseMessageTooBig, cerr.Code)
		}
		if cerr.Text != "batch size exceeds limit" {
			t.Fatalf("expected 'batch size exceeds limit', got %q", cerr.Text)
		}
	})

	// Beyond the frame size the transport read limit is the backstop.
	t.Run("frame over the read limit is dropped by the transport", func(t *testing.T) {
		t.Parallel()

		wsConn := dial(t)
		defer wsConn.Close()

		if err := wsConn.WriteMessage(websocket.BinaryMessage, make([]byte, api.MaxDownloadFrameSize+swarm.HashSize)); err != nil {
			t.Fatal(err)
		}

		_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
		_, _, err := wsConn.ReadMessage()
		if err == nil {
			t.Fatal("expected failure on over-sized frame")
		}
	})
}

func TestChunkStream_LargeUploadDownload(t *testing.T) {
	t.Parallel()

	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)

	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)
	_, wsUploadConn, addr, chanStorer := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    wsHeaders,
		DirectUpload: true,
	})
	var stored atomic.Int32
	chanStorer.Subscribe(func(chunk swarm.Chunk) {
		_ = cs.Put(context.Background(), chunk)
		stored.Add(1)
	})

	const chunkCount = 100
	chunks := make([]swarm.Chunk, chunkCount)
	chunkMap := make(map[string]swarm.Chunk, chunkCount)
	for i := range chunkCount {
		ch := testingc.GenerateTestRandomChunk()
		chunks[i] = ch
		chunkMap[ch.Address().String()] = ch
	}

	// 1. Upload all chunks sequentially over WebSocket
	for _, ch := range chunks {
		err := wsUploadConn.WriteMessage(websocket.BinaryMessage, ch.Data())
		if err != nil {
			t.Fatalf("upload write: %v", err)
		}
		mt, msg, err := wsUploadConn.ReadMessage()
		if err != nil {
			t.Fatalf("upload ack read: %v", err)
		}
		if mt != websocket.BinaryMessage || !bytes.Equal(msg, api.SuccessWsMsg) {
			t.Fatalf("unexpected ack: %v, %v", mt, msg)
		}
	}
	err := spinlock.Wait(5*time.Second, func() bool {
		return stored.Load() == int32(chunkCount)
	})
	if err != nil {
		t.Fatalf("timed out waiting for stored chunks: %v", err)
	}

	// 2. Connect to download stream using subprotocol
	dialer := &websocket.Dialer{
		Subprotocols: []string{api.ChunkDownloadSubprotocol},
	}
	wsDownloadConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
	if err != nil {
		t.Fatalf("download dial: %v", err)
	}
	defer wsDownloadConn.Close()

	// 3. Request all chunks using multi-address batch frames (50 chunks per frame)
	const batchSize = 50
	go func() {
		for i := 0; i < chunkCount; i += batchSize {
			end := i + batchSize
			if end > chunkCount {
				end = chunkCount
			}
			batchAddrs := make([]swarm.Address, 0, end-i)
			for j := i; j < end; j++ {
				batchAddrs = append(batchAddrs, chunks[j].Address())
			}
			err := wsDownloadConn.WriteMessage(websocket.BinaryMessage, downloadRequest(batchAddrs...))
			if err != nil {
				return
			}
		}
	}()

	// 4. Read all chunk responses and verify byte-for-byte
	receivedCount := 0
	for receivedCount < chunkCount {
		_ = wsDownloadConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
		mt, msg, err := wsDownloadConn.ReadMessage()
		if err != nil {
			t.Fatalf("download read message %d: %v", receivedCount, err)
		}
		if mt != websocket.BinaryMessage {
			t.Fatalf("expected binary message, got %d", mt)
		}
		if len(msg) < 33 {
			t.Fatalf("message too short: %d", len(msg))
		}
		status := msg[0]
		if status != api.WsChunkDeliverySuccess {
			t.Fatalf("expected status 0 (success), got %d", status)
		}
		chunkAddr := swarm.NewAddress(msg[1:33])
		expectedChunk, ok := chunkMap[chunkAddr.String()]
		if !ok {
			t.Fatalf("received unknown chunk address: %s", chunkAddr)
		}
		chunkData := msg[33:]
		if !bytes.Equal(chunkData, expectedChunk.Data()) {
			t.Fatalf("chunk data mismatch for %s: got %d bytes, want %d bytes", chunkAddr, len(chunkData), len(expectedChunk.Data()))
		}
		receivedCount++
	}
}

func TestChunkDownloadStream_UnknownOpcodeRejected(t *testing.T) {
	t.Parallel()

	storerMock := mockstorer.New()
	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	dialer := &websocket.Dialer{
		Subprotocols: []string{api.ChunkDownloadSubprotocol},
	}
	wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer wsConn.Close()

	// Correctly sized frame, but the command byte is not the download opcode.
	frame := make([]byte, 1+swarm.HashSize)
	frame[0] = 'X'
	if err := wsConn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatal(err)
	}

	_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
	_, _, err = wsConn.ReadMessage()
	if err == nil {
		t.Fatal("expected failure on an unknown command")
	}
	var cerr *websocket.CloseError
	if !errors.As(err, &cerr) {
		t.Fatalf("expected close error, got %v", err)
	}
	if cerr.Text != "unknown command" {
		t.Fatalf("expected 'unknown command', got %q", cerr.Text)
	}
}

func TestChunkDownloadStream_InvalidLengthRejected(t *testing.T) {
	t.Parallel()

	storerMock := mockstorer.New()
	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	for _, length := range []int{1, 31, 32, 34, 64, 66} {
		dialer := &websocket.Dialer{
			Subprotocols: []string{api.ChunkDownloadSubprotocol},
		}
		wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}

		frame := make([]byte, length)
		frame[0] = api.ChunkDownloadOpcode
		err = wsConn.WriteMessage(websocket.BinaryMessage, frame)
		if err != nil {
			t.Fatal(err)
		}

		_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
		_, _, err = wsConn.ReadMessage()
		if err == nil {
			t.Fatalf("expected failure on frame of invalid length %d", length)
		}
		var cerr *websocket.CloseError
		if !errors.As(err, &cerr) {
			t.Fatalf("expected close error for length %d, got %v", length, err)
		}
		if cerr.Text != "invalid message length" {
			t.Fatalf("expected 'invalid message length' for length %d, got %q", length, cerr.Text)
		}
		wsConn.Close()
	}
}

func TestChunkDownloadStream_CacheValidation(t *testing.T) {
	t.Parallel()

	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)
	client, _, addr, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
	})

	// dialStatus performs a real websocket handshake and returns the HTTP status
	// the server answered with. A rejected handshake yields ErrBadHandshake.
	dialStatus := func(t *testing.T, url string, header http.Header) int {
		t.Helper()
		dialer := &websocket.Dialer{
			Subprotocols: []string{api.ChunkDownloadSubprotocol},
		}
		conn, resp, err := dialer.Dial(url, header)
		if err == nil {
			conn.Close()
			t.Fatal("expected the handshake to be rejected")
		}
		if !errors.Is(err, websocket.ErrBadHandshake) {
			t.Fatalf("expected bad handshake, got %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	t.Run("invalid swarm-cache header rejects handshake", func(t *testing.T) {
		t.Parallel()
		header := http.Header{}
		header.Set("Swarm-Cache", "maybe")
		if code := dialStatus(t, "ws://"+addr+"/chunks/stream", header); code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid Swarm-Cache, got %d", code)
		}
	})

	t.Run("invalid cache query parameter rejects handshake", func(t *testing.T) {
		t.Parallel()
		if code := dialStatus(t, "ws://"+addr+"/chunks/stream?cache=notabool", nil); code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid cache query param, got %d", code)
		}
	})

	// The header takes precedence over the query parameter, but a malformed
	// query parameter must still be rejected rather than silently ignored.
	t.Run("invalid cache query parameter rejected even when header is set", func(t *testing.T) {
		t.Parallel()
		header := http.Header{}
		header.Set("Swarm-Cache", "false")
		if code := dialStatus(t, "ws://"+addr+"/chunks/stream?cache=notabool", header); code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid cache query param, got %d", code)
		}
	})

	t.Run("unknown mode rejects handshake", func(t *testing.T) {
		t.Parallel()
		if code := dialStatus(t, "ws://"+addr+"/chunks/stream?mode=downlaod", nil); code != http.StatusBadRequest {
			t.Fatalf("expected 400 for an unknown mode, got %d", code)
		}
	})

	t.Run("plain GET without upgrade returns a JSON 400", func(t *testing.T) {
		t.Parallel()
		jsonhttptest.Request(t, client, http.MethodGet, "/chunks/stream?mode=download", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "not a websocket upgrade request",
				Code:    http.StatusBadRequest,
			}),
		)
	})
}

// errStorer makes every Download().Get call fail with a fixed error, so that the
// delivery status the stream reports for it can be asserted.
type errStorer struct {
	api.Storer
	err error
}

func (e *errStorer) Download(bool) storage.Getter {
	return storage.GetterFunc(func(context.Context, swarm.Address) (swarm.Chunk, error) {
		return nil, e.err
	})
}

func TestChunkDownloadStream_DeliveryStatus(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		want byte
	}{
		{
			name: "chunk missing locally and on the network",
			err:  storage.ErrNotFound,
			want: api.WsChunkDeliveryNotFound,
		},
		{
			// No peer could serve the chunk. The HTTP download path reports
			// this as not found too, so the stream must agree with it.
			name: "no peers left to ask",
			err:  topology.ErrNotFound,
			want: api.WsChunkDeliveryNotFound,
		},
		{
			name: "retrieval failure",
			err:  errors.New("retrieval exploded"),
			want: api.WsChunkDeliveryError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storerMock := &errStorer{
				Storer: mockstorer.NewWithChunkStore(inmemchunkstore.New()),
				err:    tc.err,
			}
			_, _, addr, _ := newTestServer(t, testServerOptions{
				Storer: storerMock,
			})

			dialer := &websocket.Dialer{
				Subprotocols: []string{api.ChunkDownloadSubprotocol},
			}
			wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer wsConn.Close()

			want := swarm.RandAddress(t)
			if err := wsConn.WriteMessage(websocket.BinaryMessage, downloadRequest(want)); err != nil {
				t.Fatal(err)
			}

			_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
			_, msg, err := wsConn.ReadMessage()
			if err != nil {
				t.Fatalf("expected a status frame, got %v", err)
			}
			if len(msg) != 1+swarm.HashSize {
				t.Fatalf("expected a %d byte status frame, got %d", 1+swarm.HashSize, len(msg))
			}
			if msg[0] != tc.want {
				t.Fatalf("expected status 0x%02x, got 0x%02x", tc.want, msg[0])
			}
			if !bytes.Equal(msg[1:], want.Bytes()) {
				t.Fatal("status frame carries the wrong address")
			}
		})
	}
}

// cacheFlagStorer records the cache flag that every Download call is made with.
type cacheFlagStorer struct {
	api.Storer
	mu    sync.Mutex
	flags []bool
}

func (c *cacheFlagStorer) Download(cache bool) storage.Getter {
	c.mu.Lock()
	c.flags = append(c.flags, cache)
	c.mu.Unlock()
	return c.Storer.Download(cache)
}

func (c *cacheFlagStorer) recorded() []bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.flags)
}

func TestChunkDownloadStream_CachePropagation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		query  string
		header string
		want   bool
	}{
		{name: "default is cached", want: true},
		{name: "query disables cache", query: "?cache=false", want: false},
		{name: "query enables cache", query: "?cache=true", want: true},
		{name: "header disables cache", header: "false", want: false},
		{name: "header overrides query", query: "?cache=true", header: "false", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cs := inmemchunkstore.New()
			recorder := &cacheFlagStorer{Storer: mockstorer.NewWithChunkStore(cs)}
			_, _, addr, _ := newTestServer(t, testServerOptions{
				Storer: recorder,
			})

			ch := testingc.GenerateTestRandomChunk()
			if err := cs.Put(context.Background(), ch); err != nil {
				t.Fatal(err)
			}

			header := http.Header{}
			if tc.header != "" {
				header.Set("Swarm-Cache", tc.header)
			}
			dialer := &websocket.Dialer{
				Subprotocols: []string{api.ChunkDownloadSubprotocol},
			}
			wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream"+tc.query, header)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer wsConn.Close()

			if err := wsConn.WriteMessage(websocket.BinaryMessage, downloadRequest(ch.Address())); err != nil {
				t.Fatal(err)
			}

			_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
			_, msg, err := wsConn.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}
			if msg[0] != api.WsChunkDeliverySuccess {
				t.Fatalf("expected success, got %d", msg[0])
			}

			flags := recorder.recorded()
			if len(flags) != 1 {
				t.Fatalf("expected exactly one Download call, got %d", len(flags))
			}
			if flags[0] != tc.want {
				t.Fatalf("expected Download(%v), got Download(%v)", tc.want, flags[0])
			}
		})
	}
}

// A client that buffers ahead stops reading from the socket while its buffer
// drains. The delivery write deadline is what decides whether the node tolerates
// that or tears the whole stream down, so it must govern survival and be set
// generously enough for a lookahead-buffering client.
func TestChunkDownloadStream_SlowReaderTolerance(t *testing.T) {
	t.Parallel()

	const (
		chunkCount = 2000
		pause      = 2 * time.Second
	)

	for _, tc := range []struct {
		name     string
		deadline time.Duration
		survives bool
	}{
		{name: "deadline shorter than the pause tears the stream down", deadline: 500 * time.Millisecond, survives: false},
		{name: "deadline longer than the pause keeps the stream alive", deadline: 30 * time.Second, survives: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cs := inmemchunkstore.New()
			_, _, addr, _ := newTestServer(t, testServerOptions{
				Storer:                     mockstorer.NewWithChunkStore(cs),
				ChunkDeliveryWriteDeadline: tc.deadline,
			})

			addrs := make([]swarm.Address, chunkCount)
			for i := range addrs {
				ch := testingc.GenerateTestRandomChunk()
				if err := cs.Put(context.Background(), ch); err != nil {
					t.Fatal(err)
				}
				addrs[i] = ch.Address()
			}

			dialer := &websocket.Dialer{
				Subprotocols: []string{api.ChunkDownloadSubprotocol},
			}
			wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer wsConn.Close()

			// Queue far more than the node can hold in flight, then go quiet.
			// The node blocks writing once the client stops draining the socket.
			for i := 0; i < chunkCount; i += 100 {
				_ = wsConn.SetWriteDeadline(time.Now().Add(streamTestTimeout))
				if err := wsConn.WriteMessage(websocket.BinaryMessage, downloadRequest(addrs[i:min(i+100, chunkCount)]...)); err != nil {
					break // the node has stopped reading; enough is queued
				}
			}

			time.Sleep(pause)

			received := 0
			var readErr error
			for received < chunkCount {
				_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
				if _, _, err := wsConn.ReadMessage(); err != nil {
					readErr = err
					break
				}
				received++
			}

			if tc.survives {
				if readErr != nil {
					t.Fatalf("stream did not survive a %v pause: received %d of %d: %v", pause, received, chunkCount, readErr)
				}
			} else if readErr == nil {
				t.Fatalf("expected the stream to be torn down with a %v deadline, but all %d chunks arrived", tc.deadline, received)
			}
		})
	}
}

// blockingStorer never returns from a retrieval, standing in for a chunk whose
// peers are unreachable.
type blockingStorer struct {
	api.Storer
	entered chan struct{}
}

func (b *blockingStorer) Download(bool) storage.Getter {
	return storage.GetterFunc(func(ctx context.Context, _ swarm.Address) (swarm.Chunk, error) {
		select {
		case b.entered <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	})
}

// A retrieval that never completes must not park a worker indefinitely: the
// request times out on its own and still owes the client a status frame, so the
// rest of the queue keeps moving.
func TestChunkDownloadStream_RequestTimeout(t *testing.T) {
	t.Parallel()

	storerMock := &blockingStorer{
		Storer:  mockstorer.NewWithChunkStore(inmemchunkstore.New()),
		entered: make(chan struct{}, 1),
	}
	_, _, addr, _ := newTestServer(t, testServerOptions{
		Storer:                      storerMock,
		ChunkDownloadRequestTimeout: 500 * time.Millisecond,
	})

	dialer := &websocket.Dialer{
		Subprotocols: []string{api.ChunkDownloadSubprotocol},
	}
	wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer wsConn.Close()

	want := swarm.RandAddress(t)
	if err := wsConn.WriteMessage(websocket.BinaryMessage, downloadRequest(want)); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
	_, msg, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("expected a status frame rather than silence: %v", err)
	}
	elapsed := time.Since(start)

	if msg[0] != api.WsChunkDeliveryError {
		t.Fatalf("expected status 0x%02x, got 0x%02x", api.WsChunkDeliveryError, msg[0])
	}
	if !bytes.Equal(msg[1:], want.Bytes()) {
		t.Fatal("status frame carries the wrong address")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("request timeout did not bound the wait: took %v", elapsed)
	}

	// The worker must be free again, so a second request is answered too.
	if err := wsConn.WriteMessage(websocket.BinaryMessage, downloadRequest(want)); err != nil {
		t.Fatal(err)
	}
	_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
	if _, _, err := wsConn.ReadMessage(); err != nil {
		t.Fatalf("stream did not recover after a timed-out request: %v", err)
	}
}

// Shutting the node down must not wait on download streams, whatever state they
// are in. The hard case is a client that has stopped reading: a worker is then
// blocked in conn.WriteMessage, holding the websocket's write lock, and neither
// cancelling ctx nor sending a close frame releases it — only closing the
// socket does. Each case asserts Close directly rather than leaving it to
// cleanup, because Close is the thing under test.
func TestChunkDownloadStream_Shutdown(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// flood is how many chunks to request without ever reading a reply;
		// zero leaves the stream idle.
		flood int
		// protocolError sends a malformed frame once a worker is blocked, so
		// the read loop tears the stream down itself — with that writer still
		// stuck — before the node shuts down. The flood has to stay small
		// enough that the read loop is still reading and sees the frame.
		protocolError bool
	}{
		{name: "idle stream"},
		{name: "client stopped reading", flood: 2000},
		{name: "client stopped reading, then stream torn down", flood: 1200, protocolError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var svc *api.Service
			cs := inmemchunkstore.New()
			_, _, addr, _ := newTestServer(t, testServerOptions{
				Storer:  mockstorer.NewWithChunkStore(cs),
				Service: &svc,
			})
			closed := false
			t.Cleanup(func() {
				if !closed {
					_ = svc.Close()
				}
			})

			addrs := make([]swarm.Address, tc.flood)
			for i := range addrs {
				ch := testingc.GenerateTestRandomChunk()
				if err := cs.Put(context.Background(), ch); err != nil {
					t.Fatal(err)
				}
				addrs[i] = ch.Address()
			}

			dialer := &websocket.Dialer{
				Subprotocols: []string{api.ChunkDownloadSubprotocol},
			}
			wsConn, _, err := dialer.Dial("ws://"+addr+"/chunks/stream", nil)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			// Never read from wsConn: the socket must still be open, and
			// undrained, when the service shuts down.
			t.Cleanup(func() { _ = wsConn.Close() })

			for i := 0; i < tc.flood; i += 100 {
				_ = wsConn.SetWriteDeadline(time.Now().Add(streamTestTimeout))
				if err := wsConn.WriteMessage(websocket.BinaryMessage, downloadRequest(addrs[i:min(i+100, tc.flood)]...)); err != nil {
					break // the node has stopped reading; enough is queued
				}
			}
			// Let the workers fill the socket and block, and the read loop
			// settle into ReadMessage.
			time.Sleep(500 * time.Millisecond)

			// Only now, with a writer already stuck, tear the stream down. Sent
			// any earlier, the read loop exits before anything is blocked and the
			// teardown ordering is never exercised.
			if tc.protocolError {
				_ = wsConn.SetWriteDeadline(time.Now().Add(streamTestTimeout))
				if err := wsConn.WriteMessage(websocket.TextMessage, []byte("not a frame")); err != nil {
					t.Fatal(err)
				}
				time.Sleep(300 * time.Millisecond)
			}

			closed = true
			start := time.Now()
			if err := svc.Close(); err != nil {
				t.Fatalf("Close after %v: %v", time.Since(start), err)
			}
		})
	}
}
