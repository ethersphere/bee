// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/api/pb"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	mockbatchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	testingpostage "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/gorilla/websocket"
)

// streamTestTimeout bounds how long a test waits for a websocket response.
const streamTestTimeout = 10 * time.Second

// nolint:paralleltest
func TestChunkUploadStream(t *testing.T) {
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)

	var (
		storerMock                  = mockstorer.New()
		_, wsConn, _, chanStorer, _ = newTestServer(t, testServerOptions{
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
		storerMock                  = mockstorer.New()
		_, wsConn, _, chanStorer, _ = newTestServer(t, testServerOptions{
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
		storerMock         = mockstorer.New()
		_, wsConn, _, _, _ = newTestServer(t, testServerOptions{
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

func sendPBRequest(t *testing.T, conn *websocket.Conn, req *pb.Request) {
	t.Helper()
	data, err := req.Marshal()
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("write message: %v", err)
	}
}

func readPBResponse(t *testing.T, conn *websocket.Conn) *pb.Response {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(streamTestTimeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	mt, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("expected binary message, got %d", mt)
	}
	resp := &pb.Response{}
	if err := resp.Unmarshal(msg); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}

func getRequest(address []byte, cache pb.CacheOption) *pb.Request {
	return &pb.Request{Body: &pb.Request_Get{Get: &pb.GetRequest{Address: address, Cache: cache}}}
}

func putRequest(address, data, stamp, batchID []byte) *pb.Request {
	return &pb.Request{Body: &pb.Request_Put{Put: &pb.PutRequest{
		Address: address,
		Data:    data,
		Stamp:   stamp,
		BatchId: batchID,
	}}}
}

// chunkPut uploads ch under its own address, stamped with the connection's batch.
func chunkPut(ch swarm.Chunk) *pb.Request {
	return putRequest(ch.Address().Bytes(), ch.Data(), nil, nil)
}

// readGet reads the next response and fails unless it is a download reply.
func readGet(t *testing.T, conn *websocket.Conn) *pb.GetResponse {
	t.Helper()
	resp := readPBResponse(t, conn)
	g := resp.GetGet()
	if g == nil {
		t.Fatalf("expected a GetResponse, got %T", resp.GetBody())
	}
	return g
}

// readPut reads the next response and fails unless it is an upload reply.
func readPut(t *testing.T, conn *websocket.Conn) *pb.PutResponse {
	t.Helper()
	resp := readPBResponse(t, conn)
	p := resp.GetPut()
	if p == nil {
		t.Fatalf("expected a PutResponse, got %T", resp.GetBody())
	}
	return p
}

func streamHeaders(withBatch bool) http.Header {
	h := http.Header{}
	h.Set(api.ContentTypeHeader, "application/octet-stream")
	if withBatch {
		h.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	}
	h.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)
	return h
}

// nolint:paralleltest
func TestChunkBidirectionalStream_UploadAndDownload(t *testing.T) {
	var (
		cs                          = inmemchunkstore.New()
		storerMock                  = mockstorer.NewWithChunkStore(cs)
		_, wsConn, _, chanStorer, _ = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			WsPath:       "/chunks/stream",
			WsHeaders:    streamHeaders(true),
			DirectUpload: true,
		})
	)

	// 1. Upload chunks; replies are correlated by the address they carry.
	const numChunks = 10
	chunks := make(map[string]swarm.Chunk, numChunks)
	for range numChunks {
		ch := testingc.GenerateTestRandomChunk()
		chunks[ch.Address().ByteString()] = ch
		// Seed in cs for the downloads below.
		if err := cs.Put(context.Background(), ch); err != nil {
			t.Fatal(err)
		}
		sendPBRequest(t, wsConn, chunkPut(ch))
	}

	uploaded := make(map[string]bool, numChunks)
	for range numChunks {
		resp := readPut(t, wsConn)
		ch, ok := chunks[string(resp.Address)]
		if !ok {
			t.Fatalf("put reply for an address that was never uploaded: %x", resp.Address)
		}
		if resp.Status != pb.Status_STATUS_OK {
			t.Fatalf("expected STATUS_OK, got %v (err: %s)", resp.Status, resp.Error)
		}
		if err := spinlock.Wait(streamTestTimeout, func() bool { return chanStorer.Has(ch.Address()) }); err != nil {
			t.Fatalf("chunk %s not pushed: %v", ch.Address(), err)
		}
		uploaded[string(resp.Address)] = true
	}
	if len(uploaded) != numChunks {
		t.Fatalf("expected %d distinct put replies, got %d", numChunks, len(uploaded))
	}

	// 2. Download them on the same connection.
	for _, ch := range chunks {
		sendPBRequest(t, wsConn, getRequest(ch.Address().Bytes(), pb.CacheOption_CACHE_DEFAULT))
	}
	downloaded := make(map[string]bool, numChunks)
	for range numChunks {
		resp := readGet(t, wsConn)
		ch, ok := chunks[string(resp.Address)]
		if !ok {
			t.Fatalf("get reply for an address that was never requested: %x", resp.Address)
		}
		if resp.Status != pb.Status_STATUS_OK {
			t.Fatalf("expected STATUS_OK for get, got %v (err: %s)", resp.Status, resp.Error)
		}
		if !bytes.Equal(resp.Data, ch.Data()) {
			t.Fatalf("get response data mismatch for %s", ch.Address())
		}
		downloaded[string(resp.Address)] = true
	}
	if len(downloaded) != numChunks {
		t.Fatalf("expected %d distinct get replies, got %d", numChunks, len(downloaded))
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_Interleaved(t *testing.T) {
	var (
		cs                 = inmemchunkstore.New()
		storerMock         = mockstorer.NewWithChunkStore(cs)
		_, wsConn, _, _, _ = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			WsPath:       "/chunks/stream",
			WsHeaders:    streamHeaders(true),
			DirectUpload: true,
		})
	)

	const count = 10
	getChunks := make(map[string]swarm.Chunk, count)
	putChunks := make(map[string]swarm.Chunk, count)
	for range count {
		g := testingc.GenerateTestRandomChunk()
		if err := cs.Put(context.Background(), g); err != nil {
			t.Fatal(err)
		}
		getChunks[g.Address().ByteString()] = g
		p := testingc.GenerateTestRandomChunk()
		putChunks[p.Address().ByteString()] = p

		sendPBRequest(t, wsConn, chunkPut(p))
		sendPBRequest(t, wsConn, getRequest(g.Address().Bytes(), pb.CacheOption_CACHE_DEFAULT))
	}

	gets, puts := 0, 0
	for range count * 2 {
		resp := readPBResponse(t, wsConn)
		switch body := resp.GetBody().(type) {
		case *pb.Response_Get:
			ch, ok := getChunks[string(body.Get.Address)]
			if !ok || body.Get.Status != pb.Status_STATUS_OK || !bytes.Equal(body.Get.Data, ch.Data()) {
				t.Fatalf("unexpected get reply: %+v", body.Get)
			}
			gets++
		case *pb.Response_Put:
			if _, ok := putChunks[string(body.Put.Address)]; !ok || body.Put.Status != pb.Status_STATUS_OK {
				t.Fatalf("unexpected put reply: %+v", body.Put)
			}
			puts++
		default:
			t.Fatalf("response with no body")
		}
	}
	if gets != count || puts != count {
		t.Fatalf("expected %d gets and %d puts, got %d and %d", count, count, gets, puts)
	}
}

// A download and an upload of the same address can be in flight together: the
// response type tells them apart, since the address alone does not.
//
// nolint:paralleltest
func TestChunkBidirectionalStream_SameAddressGetAndPut(t *testing.T) {
	var (
		cs                 = inmemchunkstore.New()
		storerMock         = mockstorer.NewWithChunkStore(cs)
		_, wsConn, _, _, _ = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			WsPath:       "/chunks/stream",
			WsHeaders:    streamHeaders(true),
			DirectUpload: true,
		})
	)

	ch := testingc.GenerateTestRandomChunk()
	if err := cs.Put(context.Background(), ch); err != nil {
		t.Fatal(err)
	}

	sendPBRequest(t, wsConn, chunkPut(ch))
	sendPBRequest(t, wsConn, getRequest(ch.Address().Bytes(), pb.CacheOption_CACHE_DEFAULT))

	var gotGet, gotPut bool
	for range 2 {
		resp := readPBResponse(t, wsConn)
		switch body := resp.GetBody().(type) {
		case *pb.Response_Get:
			if !bytes.Equal(body.Get.Address, ch.Address().Bytes()) || body.Get.Status != pb.Status_STATUS_OK || !bytes.Equal(body.Get.Data, ch.Data()) {
				t.Fatalf("unexpected get reply: %+v", body.Get)
			}
			gotGet = true
		case *pb.Response_Put:
			if !bytes.Equal(body.Put.Address, ch.Address().Bytes()) || body.Put.Status != pb.Status_STATUS_OK {
				t.Fatalf("unexpected put reply: %+v", body.Put)
			}
			gotPut = true
		default:
			t.Fatalf("response with no body")
		}
	}
	if !gotGet || !gotPut {
		t.Fatalf("expected one get and one put reply, got get=%v put=%v", gotGet, gotPut)
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_PerRequestErrors(t *testing.T) {
	var (
		cs                 = inmemchunkstore.New()
		storerMock         = mockstorer.NewWithChunkStore(cs)
		_, wsConn, _, _, _ = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			WsPath:       "/chunks/stream",
			WsHeaders:    streamHeaders(true),
			DirectUpload: true,
		})
	)

	validChunk := testingc.GenerateTestRandomChunk()
	if err := cs.Put(context.Background(), validChunk); err != nil {
		t.Fatal(err)
	}
	otherChunk := testingc.GenerateTestRandomChunk()

	// Each failure must come back as a reply to that request, echoing its
	// address, and leave the connection usable.
	for _, tc := range []struct {
		name    string
		req     *pb.Request
		status  pb.Status
		errText string
	}{
		{
			name:   "get of a missing chunk",
			req:    getRequest(otherChunk.Address().Bytes(), pb.CacheOption_CACHE_DEFAULT),
			status: pb.Status_STATUS_NOT_FOUND,
		},
		{
			name:    "get with a short address",
			req:     getRequest([]byte("too-short"), pb.CacheOption_CACHE_DEFAULT),
			status:  pb.Status_STATUS_BAD_REQUEST,
			errText: "invalid chunk address length",
		},
		{
			name:    "put with a short address",
			req:     putRequest([]byte("too-short"), validChunk.Data(), nil, nil),
			status:  pb.Status_STATUS_BAD_REQUEST,
			errText: "invalid chunk address length",
		},
		{
			name:    "put with too little data",
			req:     putRequest(validChunk.Address().Bytes(), []byte{1, 2, 3}, nil, nil),
			status:  pb.Status_STATUS_BAD_REQUEST,
			errText: "insufficient data for chunk",
		},
		{
			name:    "put whose address does not match its data",
			req:     putRequest(otherChunk.Address().Bytes(), validChunk.Data(), nil, nil),
			status:  pb.Status_STATUS_BAD_REQUEST,
			errText: "address does not match chunk data",
		},
		{
			name:    "put with a short batch id",
			req:     putRequest(validChunk.Address().Bytes(), validChunk.Data(), nil, []byte("short")),
			status:  pb.Status_STATUS_BAD_REQUEST,
			errText: "invalid batch id",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sendPBRequest(t, wsConn, tc.req)

			var (
				address []byte
				status  pb.Status
				errText string
			)
			if tc.req.GetGet() != nil {
				resp := readGet(t, wsConn)
				address, status, errText = resp.Address, resp.Status, resp.Error
			} else {
				resp := readPut(t, wsConn)
				address, status, errText = resp.Address, resp.Status, resp.Error
			}

			wantAddress := tc.req.GetGet().GetAddress()
			if tc.req.GetPut() != nil {
				wantAddress = tc.req.GetPut().GetAddress()
			}
			if !bytes.Equal(address, wantAddress) {
				t.Fatalf("reply must echo the request address: got %x, want %x", address, wantAddress)
			}
			if status != tc.status {
				t.Fatalf("expected %v, got %v (err: %s)", tc.status, status, errText)
			}
			if tc.errText != "" && errText != tc.errText {
				t.Fatalf("expected error %q, got %q", tc.errText, errText)
			}
		})
	}

	// The connection is still alive after all of the above.
	sendPBRequest(t, wsConn, chunkPut(validChunk))
	if resp := readPut(t, wsConn); resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK after errors, got %v (err: %s)", resp.Status, resp.Error)
	}
	sendPBRequest(t, wsConn, getRequest(validChunk.Address().Bytes(), pb.CacheOption_CACHE_DEFAULT))
	if resp := readGet(t, wsConn); resp.Status != pb.Status_STATUS_OK || !bytes.Equal(resp.Data, validChunk.Data()) {
		t.Fatalf("expected the chunk after errors, got %v (err: %s)", resp.Status, resp.Error)
	}
}

// failingDirectUploadStorer mocks a storer where DirectUpload().Done() fails
type failingDirectUploadStorer struct {
	api.Storer
	failErr error
}

func (f *failingDirectUploadStorer) DirectUpload() storer.PutterSession {
	return &failingPutterSession{
		PutterSession: f.Storer.DirectUpload(),
		failErr:       f.failErr,
	}
}

type failingPutterSession struct {
	storer.PutterSession
	failErr error
}

func (f *failingPutterSession) Done(swarm.Address) error {
	return f.failErr
}

// nolint:paralleltest
func TestChunkBidirectionalStream_DirectUploadFailure(t *testing.T) {
	storerMock := &failingDirectUploadStorer{
		Storer:  mockstorer.New(),
		failErr: errors.New("network push failed"),
	}

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    streamHeaders(true),
		DirectUpload: true,
	})

	ch := testingc.GenerateTestRandomChunk()
	sendPBRequest(t, wsConn, chunkPut(ch))
	resp := readPut(t, wsConn)
	if !bytes.Equal(resp.Address, ch.Address().Bytes()) {
		t.Fatalf("expected reply for %s, got %x", ch.Address(), resp.Address)
	}
	if resp.Status != pb.Status_STATUS_ERROR {
		t.Fatalf("expected STATUS_ERROR when push fails, got %v", resp.Status)
	}
	if resp.Error != "chunk push failed" {
		t.Fatalf("expected sanitized error 'chunk push failed', got %q", resp.Error)
	}
}

// blockingDirectUploadStorer mocks a storer where DirectUpload().Put blocks until blockCh is closed or ctx is done.
type blockingDirectUploadStorer struct {
	api.Storer
	blockCh chan struct{}
}

func (b *blockingDirectUploadStorer) DirectUpload() storer.PutterSession {
	return &blockingPutterSession{
		PutterSession: b.Storer.DirectUpload(),
		blockCh:       b.blockCh,
	}
}

type blockingPutterSession struct {
	storer.PutterSession
	blockCh chan struct{}
}

func (b *blockingPutterSession) Put(ctx context.Context, ch swarm.Chunk) error {
	select {
	case <-b.blockCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	return b.PutterSession.Put(ctx, ch)
}

// nolint:paralleltest
func TestChunkBidirectionalStream_Fairness(t *testing.T) {
	blockCh := make(chan struct{})
	defer func() {
		select {
		case <-blockCh:
		default:
			close(blockCh)
		}
	}()

	cs := inmemchunkstore.New()
	storerMock := &blockingDirectUploadStorer{
		Storer:  mockstorer.NewWithChunkStore(cs),
		blockCh: blockCh,
	}

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    streamHeaders(true),
		DirectUpload: true,
	})

	targetChunk := testingc.GenerateTestRandomChunk()
	if err := cs.Put(context.Background(), targetChunk); err != nil {
		t.Fatal(err)
	}

	// numPuts saturates all upload workers and queues in putQueue, while
	// strictly ruling out a single shared pool of workers.
	numPuts := 2*api.DefaultStreamSubWorkers + 4
	for range numPuts {
		sendPBRequest(t, wsConn, chunkPut(testingc.GenerateTestRandomChunk()))
	}

	// Brief pause to ensure all upload workers have picked up the Puts and are blocked
	time.Sleep(50 * time.Millisecond)

	// A download sent behind the blocked uploads must still be answered.
	sendPBRequest(t, wsConn, getRequest(targetChunk.Address().Bytes(), pb.CacheOption_CACHE_DEFAULT))
	resp := readGet(t, wsConn)
	if !bytes.Equal(resp.Address, targetChunk.Address().Bytes()) {
		t.Fatalf("expected the get reply for %s, got %x", targetChunk.Address(), resp.Address)
	}
	if resp.Status != pb.Status_STATUS_OK || !bytes.Equal(resp.Data, targetChunk.Data()) {
		t.Fatalf("expected the chunk while uploads are blocked, got %v", resp.Status)
	}

	// Unblock the Puts and verify they all complete successfully
	close(blockCh)
	for range numPuts {
		if putResp := readPut(t, wsConn); putResp.Status != pb.Status_STATUS_OK {
			t.Fatalf("expected STATUS_OK for unblocked put %x, got %v", putResp.Address, putResp.Status)
		}
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_QueueBusy(t *testing.T) {
	blockCh := make(chan struct{})
	defer func() {
		select {
		case <-blockCh:
		default:
			close(blockCh)
		}
	}()

	cs := inmemchunkstore.New()
	storerMock := &blockingDirectUploadStorer{
		Storer:  mockstorer.NewWithChunkStore(cs),
		blockCh: blockCh,
	}

	_, wsConn, _, _, svc := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    streamHeaders(true),
		DirectUpload: true,
	})

	// Fill the upload workers and the putQueue buffer behind them.
	capacity := api.MaxStreamQueueSize + api.DefaultStreamSubWorkers
	for range capacity {
		sendPBRequest(t, wsConn, chunkPut(testingc.GenerateTestRandomChunk()))
	}

	// One more is rejected straight away, with its own address.
	overflow := testingc.GenerateTestRandomChunk()
	sendPBRequest(t, wsConn, chunkPut(overflow))

	busyResp := readPut(t, wsConn)
	if !bytes.Equal(busyResp.Address, overflow.Address().Bytes()) {
		t.Fatalf("expected the busy reply for %s, got %x", overflow.Address(), busyResp.Address)
	}
	if busyResp.Status != pb.Status_STATUS_BUSY {
		t.Fatalf("expected STATUS_BUSY, got %v (err: %s)", busyResp.Status, busyResp.Error)
	}
	if busyResp.Error != "request queue full" {
		t.Fatalf("expected error 'request queue full', got %q", busyResp.Error)
	}
	if got := svc.ChunkStreamDeliveryCount("busy"); got != 1 {
		t.Fatalf("expected 1 busy delivery counted, got %v", got)
	}

	// Unblock workers and drain remaining responses
	close(blockCh)
	for range capacity {
		if resp := readPut(t, wsConn); resp.Status != pb.Status_STATUS_OK {
			t.Fatalf("expected STATUS_OK for unblocked put, got %v", resp.Status)
		}
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_PerChunkStamp(t *testing.T) {
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	owner, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}

	ch := testingc.GenerateTestRandomChunk()
	stamp := testingpostage.MustNewValidStamp(signer, ch.Address())
	stampBytes, err := stamp.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	batchStore := mockbatchstore.New(
		mockbatchstore.WithAcceptAllExistsFunc(),
		mockbatchstore.WithBatch(&postage.Batch{
			Owner: owner.Bytes(),
		}),
	)

	var (
		storerMock         = mockstorer.New()
		_, wsConn, _, _, _ = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			BatchStore:   batchStore,
			WsPath:       "/chunks/stream",
			WsHeaders:    streamHeaders(false), // no connection batch: stamps must come per chunk
			DirectUpload: true,
		})
	)

	// Without a stamp, a batch on the request, or a batch on the connection, the
	// upload is rejected.
	sendPBRequest(t, wsConn, chunkPut(ch))
	if resp := readPut(t, wsConn); resp.Status != pb.Status_STATUS_BAD_REQUEST || resp.Error != "missing postage stamp or batch id" {
		t.Fatalf("expected STATUS_BAD_REQUEST without stamp, got %v (err: %s)", resp.Status, resp.Error)
	}

	// With a valid pre-signed stamp it succeeds, and the stamp outranks a
	// BatchId the node could not use anyway.
	sendPBRequest(t, wsConn, putRequest(ch.Address().Bytes(), ch.Data(), stampBytes, []byte("not-a-batch")))
	resp := readPut(t, wsConn)
	if resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK with stamp, got %v (err: %s)", resp.Status, resp.Error)
	}
	if !bytes.Equal(resp.Address, ch.Address().Bytes()) {
		t.Fatalf("address mismatch: got %x, want %x", resp.Address, ch.Address().Bytes())
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_QueryParamMode(t *testing.T) {
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)

	var (
		storerMock       = mockstorer.New()
		_, _, addr, _, _ = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			DirectUpload: true,
		})
	)

	wsConn, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/chunks/stream?mode=stream", wsHeaders)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = wsConn.Close() })

	ch := testingc.GenerateTestRandomChunk()
	sendPBRequest(t, wsConn, chunkPut(ch))
	if resp := readPut(t, wsConn); resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK via ?mode=stream, got %v (err: %s)", resp.Status, resp.Error)
	}

	// Invalid mode query parameter returns 400
	jsonhttptest.Request(t, http.DefaultClient, http.MethodGet, "http://"+addr+"/chunks/stream?mode=invalid", http.StatusBadRequest)
}

// cacheRecordingStorer records the cache flag of each download.
type cacheRecordingStorer struct {
	api.Storer
	caches chan bool
}

func (c *cacheRecordingStorer) Download(cache bool) storage.Getter {
	c.caches <- cache
	return c.Storer.Download(cache)
}

// The connection default comes from Swarm-Cache, then ?cache=, then true, and
// a request's CacheOption overrides it.
// nolint:paralleltest
func TestChunkBidirectionalStream_CacheOption(t *testing.T) {
	cs := inmemchunkstore.New()
	ch := testingc.GenerateTestRandomChunk()
	if err := cs.Put(context.Background(), ch); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		header string
		query  string
		option pb.CacheOption
		want   bool
	}{
		{name: "default", option: pb.CacheOption_CACHE_DEFAULT, want: true},
		{name: "header false", header: "false", option: pb.CacheOption_CACHE_DEFAULT, want: false},
		{name: "query false", query: "false", option: pb.CacheOption_CACHE_DEFAULT, want: false},
		{name: "header wins over query", header: "true", query: "false", option: pb.CacheOption_CACHE_DEFAULT, want: true},
		{name: "request disables", option: pb.CacheOption_CACHE_DISABLE, want: false},
		{name: "request enables over header", header: "false", option: pb.CacheOption_CACHE_ENABLE, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storerMock := &cacheRecordingStorer{
				Storer: mockstorer.NewWithChunkStore(cs),
				caches: make(chan bool, 1),
			}
			h := streamHeaders(false)
			if tc.header != "" {
				h.Set("Swarm-Cache", tc.header)
			}
			q := url.Values{}
			if tc.query != "" {
				q.Set("cache", tc.query)
			}
			_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
				Storer:    storerMock,
				Post:      mockpost.New(mockpost.WithAcceptAll()),
				WsPath:    "/chunks/stream",
				WsQuery:   q,
				WsHeaders: h,
			})

			sendPBRequest(t, wsConn, getRequest(ch.Address().Bytes(), tc.option))
			if resp := readGet(t, wsConn); resp.Status != pb.Status_STATUS_OK {
				t.Fatalf("expected STATUS_OK, got %v: %s", resp.Status, resp.Error)
			}
			if got := <-storerMock.caches; got != tc.want {
				t.Fatalf("downloaded with cache %v, want %v", got, tc.want)
			}
		})
	}
}

// Messages the node cannot answer — because they are not protobuf, or carry no
// request body and so no address to reply with — close the connection.
//
// nolint:paralleltest
func TestChunkBidirectionalStream_ProtocolViolation(t *testing.T) {
	emptyRequest, err := (&pb.Request{}).Marshal()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		msgType   int
		data      []byte
		closeText string
	}{
		{name: "text frame", msgType: websocket.TextMessage, data: []byte("invalid text message"), closeText: "invalid message type"},
		{name: "not protobuf", msgType: websocket.BinaryMessage, data: []byte{0xff, 0xff, 0xff}, closeText: "invalid protobuf request"},
		{name: "request with no body", msgType: websocket.BinaryMessage, data: emptyRequest, closeText: "missing request body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
				Storer:       mockstorer.New(),
				Post:         mockpost.New(mockpost.WithAcceptAll()),
				WsPath:       "/chunks/stream",
				WsHeaders:    streamHeaders(true),
				DirectUpload: true,
			})

			if err := wsConn.WriteMessage(tc.msgType, tc.data); err != nil {
				t.Fatal(err)
			}

			_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
			_, _, err := wsConn.ReadMessage()
			var cerr *websocket.CloseError
			if !errors.As(err, &cerr) {
				t.Fatalf("expected a close error, got %v", err)
			}
			if cerr.Code != websocket.CloseUnsupportedData {
				t.Fatalf("expected close code %d, got %d", websocket.CloseUnsupportedData, cerr.Code)
			}
			if cerr.Text != tc.closeText {
				t.Fatalf("expected close text %q, got %q", tc.closeText, cerr.Text)
			}
		})
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_Shutdown(t *testing.T) {
	var svc *api.Service
	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    streamHeaders(true),
		Service:      &svc,
		DirectUpload: true,
	})

	sendPBRequest(t, wsConn, chunkPut(testingc.GenerateTestRandomChunk()))
	if resp := readPut(t, wsConn); resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK, got %v", resp.Status)
	}

	// Trigger shutdown while stream was active
	start := time.Now()
	if err := svc.Close(); err != nil {
		t.Fatalf("Close after %v: %v", time.Since(start), err)
	}

	// Verify websocket receives close or is closed
	_ = wsConn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := wsConn.ReadMessage(); err == nil {
		t.Fatal("expected error reading from closed connection, got nil")
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_FeedSizedSOC(t *testing.T) {
	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    streamHeaders(true),
		DirectUpload: true,
	})

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	id := make([]byte, swarm.HashSize)
	copy(id, []byte("feed-topic-test-id-0000000000000"))

	// Feed updates are small: 40 bytes payload
	smallPayload := []byte("feed update 40 bytes test payload data!!")
	cacChunk, err := cac.New(smallPayload)
	if err != nil {
		t.Fatal(err)
	}
	socChunk, err := soc.New(id, cacChunk).Sign(signer)
	if err != nil {
		t.Fatal(err)
	}
	socAddress := socChunk.Address().Bytes()

	// Small SOC data also parses as a valid CAC. The address settles which one
	// it is, so the SOC is stored under its SOC address, not its BMT hash.
	sendPBRequest(t, wsConn, putRequest(socAddress, socChunk.Data(), nil, nil))
	resp := readPut(t, wsConn)
	if resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK, got %v: %s", resp.Status, resp.Error)
	}
	if !bytes.Equal(resp.Address, socAddress) {
		t.Fatalf("expected SOC address %s, got %x", socChunk.Address(), resp.Address)
	}

	// Seed SOC in cs so Lookup().Get can find it during retrieval assertion
	if err := cs.Put(context.Background(), socChunk); err != nil {
		t.Fatal(err)
	}

	sendPBRequest(t, wsConn, getRequest(socAddress, pb.CacheOption_CACHE_DEFAULT))
	getResp := readGet(t, wsConn)
	if getResp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK on get SOC, got %v: %s", getResp.Status, getResp.Error)
	}
	if !bytes.Equal(getResp.Data, socChunk.Data()) {
		t.Fatalf("retrieved SOC data does not match uploaded SOC data")
	}

	// SOC data under an address it does not produce is rejected.
	unrelated := testingc.GenerateTestRandomChunk().Address().Bytes()
	sendPBRequest(t, wsConn, putRequest(unrelated, socChunk.Data(), nil, nil))
	if resp := readPut(t, wsConn); resp.Status != pb.Status_STATUS_BAD_REQUEST || resp.Error != "address does not match chunk data" {
		t.Fatalf("expected SOC data under the wrong address to be rejected, got %v (err: %s)", resp.Status, resp.Error)
	}

	// Corrupted SOC data (an invalid signature recovery byte) is rejected.
	corruptedData := make([]byte, len(socChunk.Data()))
	copy(corruptedData, socChunk.Data())
	corruptedData[swarm.HashSize+swarm.SocSignatureSize-1] = 99
	sendPBRequest(t, wsConn, putRequest(socAddress, corruptedData, nil, nil))
	if badResp := readPut(t, wsConn); badResp.Status != pb.Status_STATUS_BAD_REQUEST {
		t.Fatalf("expected STATUS_BAD_REQUEST for corrupted SOC, got %v", badResp.Status)
	}
}

// Uploads on the stream are always direct, so a tag is rejected at the
// handshake rather than silently ignored.
//
// nolint:paralleltest
func TestChunkBidirectionalStream_TagRejected(t *testing.T) {
	_, _, addr, _, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Post:   mockpost.New(mockpost.WithAcceptAll()),
	})

	for _, tc := range []struct {
		name   string
		query  string
		header bool
	}{
		{name: "tag header", header: true},
		{name: "tag query parameter", query: "&swarm-tag=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := streamHeaders(true)
			if tc.header {
				h.Set(api.SwarmTagHeader, "1")
			}
			conn, resp, err := websocket.DefaultDialer.Dial("ws://"+addr+"/chunks/stream?mode=stream"+tc.query, h)
			if err == nil {
				_ = conn.Close()
				t.Fatal("expected the handshake to be rejected")
			}
			if resp == nil || resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %v (err: %v)", resp, err)
			}
		})
	}
}

// One connection can stamp with several batches: a request's BatchId outranks
// the connection's Swarm-Postage-Batch-Id, which applies when a request names
// none. The stamp on each pushed chunk shows which batch the node used.
//
// nolint:paralleltest
func TestChunkBidirectionalStream_BatchPerRequest(t *testing.T) {
	batchA := bytes.Repeat([]byte{0xaa}, swarm.HashSize)
	batchB := bytes.Repeat([]byte{0xbb}, swarm.HashSize)
	unknown := bytes.Repeat([]byte{0xcc}, swarm.HashSize)

	// newStream opens a stream on a node that has issuers for batches A and B,
	// and returns it with a function that reports the batch a pushed chunk was
	// stamped with.
	newStream := func(t *testing.T, connBatch []byte) (*websocket.Conn, func(swarm.Chunk) []byte) {
		t.Helper()

		post := mockpost.New(mockpost.WithIssuer(postage.NewStampIssuer("a", "a", batchA, big.NewInt(3), 24, 6, 1000, false)))
		if err := post.Add(postage.NewStampIssuer("b", "b", batchB, big.NewInt(3), 24, 6, 1000, false)); err != nil {
			t.Fatal(err)
		}

		h := streamHeaders(false)
		if connBatch != nil {
			h.Set(api.SwarmPostageBatchIdHeader, hex.EncodeToString(connBatch))
		}

		_, wsConn, _, chanStorer, _ := newTestServer(t, testServerOptions{
			Storer: mockstorer.New(),
			Post:   post,
			// A stored batch lets a pre-signed stamp get as far as its
			// signature check.
			BatchStore:   mockbatchstore.New(mockbatchstore.WithAcceptAllExistsFunc(), mockbatchstore.WithBatch(testingpostage.MustNewBatch())),
			WsPath:       "/chunks/stream",
			WsHeaders:    h,
			DirectUpload: true,
		})

		var (
			mu         sync.Mutex
			stampBatch = make(map[string][]byte)
		)
		chanStorer.Subscribe(func(ch swarm.Chunk) {
			mu.Lock()
			stampBatch[ch.Address().ByteString()] = ch.Stamp().BatchID()
			mu.Unlock()
		})

		pushedBatch := func(ch swarm.Chunk) []byte {
			t.Helper()
			var got []byte
			err := spinlock.Wait(streamTestTimeout, func() bool {
				mu.Lock()
				defer mu.Unlock()
				got = stampBatch[ch.Address().ByteString()]
				return got != nil
			})
			if err != nil {
				t.Fatal("chunk was not pushed")
			}
			return got
		}
		return wsConn, pushedBatch
	}

	type upload struct {
		name      string
		batchID   []byte
		stamp     []byte
		wantBatch []byte // nil when the upload is rejected
		wantError string
	}
	run := func(t *testing.T, connBatch []byte, uploads []upload) {
		t.Helper()

		wsConn, pushedBatch := newStream(t, connBatch)
		for _, u := range uploads {
			ch := testingc.GenerateTestRandomChunk()
			sendPBRequest(t, wsConn, putRequest(ch.Address().Bytes(), ch.Data(), u.stamp, u.batchID))
			resp := readPut(t, wsConn)
			if u.wantBatch == nil {
				if resp.Status != pb.Status_STATUS_BAD_REQUEST || resp.Error != u.wantError {
					t.Fatalf("%s: expected STATUS_BAD_REQUEST %q, got %v (err: %s)", u.name, u.wantError, resp.Status, resp.Error)
				}
				continue
			}
			if resp.Status != pb.Status_STATUS_OK {
				t.Fatalf("%s: expected STATUS_OK, got %v (err: %s)", u.name, resp.Status, resp.Error)
			}
			if got := pushedBatch(ch); !bytes.Equal(got, u.wantBatch) {
				t.Fatalf("%s: stamped with batch %x, want %x", u.name, got, u.wantBatch)
			}
		}
	}

	// Without a connection batch, every request picks its own, and a failing
	// request leaves the connection usable.
	t.Run("no connection batch", func(t *testing.T) {
		run(t, nil, []upload{
			{name: "batch A", batchID: batchA, wantBatch: batchA},
			{name: "batch B", batchID: batchB, wantBatch: batchB},
			{name: "no batch", wantError: "missing postage stamp or batch id"},
			{name: "unknown batch", batchID: unknown, wantError: "postage batch not found"},
			{name: "batch A again", batchID: batchA, wantBatch: batchA},
		})
	})

	// A serialised stamp starts with its batch id; the rest is never looked at
	// before the batch is checked.
	stampB := append(bytes.Clone(batchB), make([]byte, postage.StampSize-swarm.HashSize)...)

	// A connection batch pins it, so that a proxy setting the header decides
	// the batch of every upload, pre-signed ones included.
	t.Run("connection batch", func(t *testing.T) {
		run(t, batchA, []upload{
			{name: "no batch uses the connection's", wantBatch: batchA},
			{name: "the connection's batch", batchID: batchA, wantBatch: batchA},
			{name: "a different batch", batchID: batchB, wantError: "batch id differs from the connection batch"},
			{name: "a stamp from a different batch", stamp: stampB, wantError: "stamp batch differs from the connection batch"},
			{name: "connection still usable", wantBatch: batchA},
		})
	})
}

// nolint:paralleltest
func TestChunkBidirectionalStream_ShutdownWithPendingWrites(t *testing.T) {
	var svc *api.Service

	blockCh := make(chan struct{})
	defer func() {
		select {
		case <-blockCh:
		default:
			close(blockCh)
		}
	}()

	cs := inmemchunkstore.New()
	storerMock := &blockingDirectUploadStorer{
		Storer:  mockstorer.NewWithChunkStore(cs),
		blockCh: blockCh,
	}

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    streamHeaders(true),
		Service:      &svc,
		DirectUpload: true,
	})

	// Send several Puts that block in the storer
	for range 5 {
		sendPBRequest(t, wsConn, chunkPut(testingc.GenerateTestRandomChunk()))
	}

	// Trigger node shutdown while upload workers are actively blocked
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- svc.Close()
	}()

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown hung with pending writes")
	}

	// Verify websocket receives close error
	_ = wsConn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := wsConn.ReadMessage(); err == nil {
		t.Fatal("expected error reading from closed connection, got nil")
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_ShutdownClientStoppedReading(t *testing.T) {
	var svc *api.Service
	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:    storerMock,
		Post:      mockpost.New(mockpost.WithAcceptAll()),
		WsPath:    "/chunks/stream",
		WsHeaders: streamHeaders(false),
		Service:   &svc,
	})

	const flood = 1000
	chunks := make([]swarm.Chunk, flood)
	for i := range flood {
		chunks[i] = testingc.GenerateTestRandomChunk()
		if err := cs.Put(context.Background(), chunks[i]); err != nil {
			t.Fatal(err)
		}
	}

	// Flood download requests without ever reading from wsConn
	for i := range flood {
		_ = wsConn.SetWriteDeadline(time.Now().Add(streamTestTimeout))
		data, err := getRequest(chunks[i].Address().Bytes(), pb.CacheOption_CACHE_DEFAULT).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if err := wsConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			break // socket write buffer filled, enough is queued
		}
	}

	// Wait for workers to fill the OS socket buffer and block on WriteMessage
	time.Sleep(300 * time.Millisecond)

	closedCh := make(chan error, 1)
	start := time.Now()
	go func() {
		closedCh <- svc.Close()
	}()

	select {
	case err := <-closedCh:
		if err != nil {
			t.Fatalf("Close after %v: %v", time.Since(start), err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("svc.Close() hung: workers stuck in WriteMessage were not unblocked by socket closure")
	}
}

// A stream that ends on its own, here on a protocol violation, while its
// workers are blocked writing to a client that has stopped reading must still
// release them, which only closing the socket does.
// nolint:paralleltest
func TestChunkBidirectionalStream_ProtocolViolationClientStoppedReading(t *testing.T) {
	var svc *api.Service
	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:    storerMock,
		Post:      mockpost.New(mockpost.WithAcceptAll()),
		WsPath:    "/chunks/stream",
		WsHeaders: streamHeaders(false),
		Service:   &svc,
	})

	const flood = 1000
	chunks := make([]swarm.Chunk, flood)
	for i := range flood {
		chunks[i] = testingc.GenerateTestRandomChunk()
		if err := cs.Put(context.Background(), chunks[i]); err != nil {
			t.Fatal(err)
		}
	}

	// Flood download requests without ever reading from wsConn
	for i := range flood {
		_ = wsConn.SetWriteDeadline(time.Now().Add(streamTestTimeout))
		data, err := getRequest(chunks[i].Address().Bytes(), pb.CacheOption_CACHE_DEFAULT).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if err := wsConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			break // socket write buffer filled, enough is queued
		}
	}

	// Wait for workers to fill the OS socket buffer and block on WriteMessage
	time.Sleep(300 * time.Millisecond)

	// A text frame is a protocol violation that makes the server end the stream.
	_ = wsConn.SetWriteDeadline(time.Now().Add(streamTestTimeout))
	if err := wsConn.WriteMessage(websocket.TextMessage, []byte("not a request")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	closedCh := make(chan error, 1)
	start := time.Now()
	go func() {
		closedCh <- svc.Close()
	}()

	select {
	case err := <-closedCh:
		if err != nil {
			t.Fatalf("Close after %v: %v", time.Since(start), err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("svc.Close() hung: workers stuck in WriteMessage were not unblocked by socket closure")
	}
}

// A response write that times out ends the stream while the read loop may still
// be blocked reading. The stream must close its socket then, or node shutdown
// finds it still open.
// nolint:paralleltest
func TestChunkBidirectionalStream_ShutdownAfterWriteTimeout(t *testing.T) {
	var svc *api.Service
	cs := inmemchunkstore.New()

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:                     mockstorer.NewWithChunkStore(cs),
		Post:                       mockpost.New(mockpost.WithAcceptAll()),
		WsPath:                     "/chunks/stream",
		WsHeaders:                  streamHeaders(false),
		Service:                    &svc,
		ChunkDeliveryWriteDeadline: 200 * time.Millisecond,
	})

	// Flood download requests without ever reading from wsConn, so that a
	// response write times out.
	for range 1000 {
		ch := testingc.GenerateTestRandomChunk()
		if err := cs.Put(context.Background(), ch); err != nil {
			t.Fatal(err)
		}
		_ = wsConn.SetWriteDeadline(time.Now().Add(streamTestTimeout))
		data, err := getRequest(ch.Address().Bytes(), pb.CacheOption_CACHE_DEFAULT).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if err := wsConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			break // socket write buffer filled, enough is queued
		}
	}

	// Wait for a response write to time out and end the stream.
	time.Sleep(time.Second)

	if err := svc.Close(); err != nil {
		t.Fatalf("Close after a response write timed out: %v", err)
	}
}

// The largest valid request fits within the frame size limit, and a larger
// message closes the stream.
// nolint:paralleltest
func TestChunkBidirectionalStream_FrameSizeLimit(t *testing.T) {
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	cacChunk, err := cac.New(bytes.Repeat([]byte{1}, swarm.ChunkSize))
	if err != nil {
		t.Fatal(err)
	}
	socChunk, err := soc.New(make([]byte, swarm.HashSize), cacChunk).Sign(crypto.NewDefaultSigner(key))
	if err != nil {
		t.Fatal(err)
	}
	largest := putRequest(socChunk.Address().Bytes(), socChunk.Data(), make([]byte, postage.StampSize), batchOk)
	if size := largest.Size(); size > api.MaxStreamFrameSize {
		t.Fatalf("largest valid request is %d bytes, over the %d byte limit", size, api.MaxStreamFrameSize)
	}
	// The limit bounds what a full request queue holds, so it must stay close to
	// the largest valid request.
	if size := largest.Size(); api.MaxStreamFrameSize > 2*size {
		t.Fatalf("frame size limit %d is more than twice the largest valid request (%d bytes)", api.MaxStreamFrameSize, size)
	}

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       mockstorer.New(),
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    streamHeaders(true),
		DirectUpload: true,
	})

	// The full-size SOC goes through, stamped by the node from the batch id.
	sendPBRequest(t, wsConn, putRequest(socChunk.Address().Bytes(), socChunk.Data(), nil, batchOk))
	if resp := readPut(t, wsConn); resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK, got %v: %s", resp.Status, resp.Error)
	}

	if err := wsConn.WriteMessage(websocket.BinaryMessage, make([]byte, api.MaxStreamFrameSize+1)); err != nil {
		t.Fatal(err)
	}
	_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
	_, _, err = wsConn.ReadMessage()
	var cerr *websocket.CloseError
	if !errors.As(err, &cerr) {
		t.Fatalf("expected a close error, got %v", err)
	}
	if cerr.Code != websocket.CloseMessageTooBig {
		t.Fatalf("expected close code %d, got %d", websocket.CloseMessageTooBig, cerr.Code)
	}
}

// A client that sends nothing while it waits for a reply must not be dropped
// by the read timeout as long as it answers pings.
// nolint:paralleltest
func TestChunkBidirectionalStream_PongsKeepStreamOpen(t *testing.T) {
	blockCh := make(chan struct{})
	storerMock := &blockingDirectUploadStorer{
		Storer:  mockstorer.NewWithChunkStore(inmemchunkstore.New()),
		blockCh: blockCh,
	}

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:                 storerMock,
		Post:                   mockpost.New(mockpost.WithAcceptAll()),
		WsPath:                 "/chunks/stream",
		WsHeaders:              streamHeaders(true),
		DirectUpload:           true,
		WsPingPeriod:           100 * time.Millisecond,
		ChunkStreamReadTimeout: 500 * time.Millisecond,
	})

	ch := testingc.GenerateTestRandomChunk()
	sendPBRequest(t, wsConn, chunkPut(ch))

	// Let the upload outlast the read timeout several times over.
	time.AfterFunc(2*time.Second, func() { close(blockCh) })

	// readPut answers the node's pings while it waits.
	resp := readPut(t, wsConn)
	if resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK, got %v: %s", resp.Status, resp.Error)
	}
	if !bytes.Equal(resp.Address, ch.Address().Bytes()) {
		t.Fatalf("expected a reply for %s, got %x", ch.Address(), resp.Address)
	}
}

// Requests still queued when the stream ends must not be stamped: their chunks
// will never be pushed.
// nolint:paralleltest
func TestChunkBidirectionalStream_NoStampAfterStreamEnds(t *testing.T) {
	blockCh := make(chan struct{})
	defer close(blockCh)
	storerMock := &blockingDirectUploadStorer{
		Storer:  mockstorer.NewWithChunkStore(inmemchunkstore.New()),
		blockCh: blockCh,
	}
	issuer := postage.NewStampIssuer("", "", batchOk, big.NewInt(3), 20, 16, 1000, true)

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithIssuer(issuer)),
		WsPath:       "/chunks/stream",
		WsHeaders:    streamHeaders(true),
		DirectUpload: true,
	})

	stamped := func() int {
		n := 0
		for _, b := range issuer.Buckets() {
			n += int(b)
		}
		return n
	}

	// Every upload worker stamps a chunk and blocks pushing it; the rest queue.
	for range api.DefaultStreamSubWorkers + 100 {
		sendPBRequest(t, wsConn, chunkPut(testingc.GenerateTestRandomChunk()))
	}
	if err := spinlock.Wait(streamTestTimeout, func() bool { return stamped() == api.DefaultStreamSubWorkers }); err != nil {
		t.Fatalf("expected %d stamped chunks, got %d", api.DefaultStreamSubWorkers, stamped())
	}

	// End the stream with a protocol violation.
	if err := wsConn.WriteMessage(websocket.TextMessage, []byte("not a request")); err != nil {
		t.Fatal(err)
	}
	_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
	if _, _, err := wsConn.ReadMessage(); err == nil {
		t.Fatal("expected the stream to close")
	}

	// Give the released workers time to drain the queue.
	time.Sleep(500 * time.Millisecond)
	if got := stamped(); got != api.DefaultStreamSubWorkers {
		t.Fatalf("expected no stamps after the stream ended, got %d more", got-api.DefaultStreamSubWorkers)
	}
}

// downloadStorer serves downloads with get instead of the chunk store.
type downloadStorer struct {
	api.Storer
	get storage.GetterFunc
}

func (d *downloadStorer) Download(bool) storage.Getter {
	return d.get
}

// A chunk that no peer could be asked for may still exist, so it is reported
// as unavailable rather than not found.
// nolint:paralleltest
func TestChunkBidirectionalStream_DownloadErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		wantStatus pb.Status
		wantError  string
	}{
		{name: "not found", err: storage.ErrNotFound, wantStatus: pb.Status_STATUS_NOT_FOUND, wantError: "chunk not found"},
		{name: "no usable peer", err: topology.ErrNotFound, wantStatus: pb.Status_STATUS_UNAVAILABLE, wantError: "no peer could serve the chunk"},
		{name: "wrapped no usable peer", err: fmt.Errorf("retrieve: %w", topology.ErrNotFound), wantStatus: pb.Status_STATUS_UNAVAILABLE, wantError: "no peer could serve the chunk"},
		{name: "other failure", err: errors.New("disk on fire"), wantStatus: pb.Status_STATUS_ERROR, wantError: "chunk read error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storerMock := &downloadStorer{
				Storer: mockstorer.New(),
				get: func(context.Context, swarm.Address) (swarm.Chunk, error) {
					return nil, tc.err
				},
			}
			_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
				Storer:    storerMock,
				Post:      mockpost.New(mockpost.WithAcceptAll()),
				WsPath:    "/chunks/stream",
				WsHeaders: streamHeaders(false),
			})

			addr := testingc.GenerateTestRandomChunk().Address()
			sendPBRequest(t, wsConn, getRequest(addr.Bytes(), pb.CacheOption_CACHE_DEFAULT))
			resp := readGet(t, wsConn)
			if resp.Status != tc.wantStatus || resp.Error != tc.wantError {
				t.Fatalf("expected %v %q, got %v %q", tc.wantStatus, tc.wantError, resp.Status, resp.Error)
			}
			if !bytes.Equal(resp.Address, addr.Bytes()) {
				t.Fatalf("expected a reply for %s, got %x", addr, resp.Address)
			}
		})
	}
}

// Download requests still queued when the stream ends must not start a
// retrieval: nobody is left to read the chunk.
// nolint:paralleltest
func TestChunkBidirectionalStream_NoDownloadAfterStreamEnds(t *testing.T) {
	var started atomic.Int64
	storerMock := &downloadStorer{
		Storer: mockstorer.New(),
		get: func(ctx context.Context, _ swarm.Address) (swarm.Chunk, error) {
			started.Add(1)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:    storerMock,
		Post:      mockpost.New(mockpost.WithAcceptAll()),
		WsPath:    "/chunks/stream",
		WsHeaders: streamHeaders(false),
	})

	// Every download worker blocks in a retrieval; the rest queue.
	for range api.DefaultStreamSubWorkers + 100 {
		addr := testingc.GenerateTestRandomChunk().Address()
		sendPBRequest(t, wsConn, getRequest(addr.Bytes(), pb.CacheOption_CACHE_DEFAULT))
	}
	if err := spinlock.Wait(streamTestTimeout, func() bool { return started.Load() == api.DefaultStreamSubWorkers }); err != nil {
		t.Fatalf("expected %d retrievals, got %d", api.DefaultStreamSubWorkers, started.Load())
	}

	// End the stream with a protocol violation.
	if err := wsConn.WriteMessage(websocket.TextMessage, []byte("not a request")); err != nil {
		t.Fatal(err)
	}
	_ = wsConn.SetReadDeadline(time.Now().Add(streamTestTimeout))
	if _, _, err := wsConn.ReadMessage(); err == nil {
		t.Fatal("expected the stream to close")
	}

	// Give the released workers time to drain the queue.
	time.Sleep(500 * time.Millisecond)
	if got := started.Load(); got != api.DefaultStreamSubWorkers {
		t.Fatalf("expected no retrievals after the stream ended, got %d more", got-api.DefaultStreamSubWorkers)
	}
}

// A client that pauses reading must keep its stream when a keepalive, in either
// direction, waits behind replies the client has not read yet: gorilla treats a
// control frame write that times out as fatal to the connection.
// nolint:paralleltest
func TestChunkBidirectionalStream_KeepaliveWhileClientPausesReading(t *testing.T) {
	for _, tc := range []struct {
		name        string
		serverPings bool
	}{
		{name: "node pings", serverPings: true},
		{name: "client pings", serverPings: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := inmemchunkstore.New()
			opts := testServerOptions{
				Storer:    mockstorer.NewWithChunkStore(cs),
				Post:      mockpost.New(mockpost.WithAcceptAll()),
				WsPath:    "/chunks/stream",
				WsHeaders: streamHeaders(false),
			}
			if tc.serverPings {
				opts.WsPingPeriod = time.Millisecond
			}
			_, wsConn, _, _, _ := newTestServer(t, opts)

			// The client pings often, so that one lands while the node's
			// socket buffer is full.
			if !tc.serverPings {
				stop := make(chan struct{})
				defer close(stop)
				go func() {
					for {
						select {
						case <-stop:
							return
						default:
						}
						_ = wsConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second))
						time.Sleep(time.Millisecond)
					}
				}()
			}

			// Fewer requests than the queue holds, so none is answered BUSY,
			// but enough replies to fill the socket buffers.
			const requests = 1000
			for range requests {
				ch := testingc.GenerateTestRandomChunk()
				if err := cs.Put(context.Background(), ch); err != nil {
					t.Fatal(err)
				}
				sendPBRequest(t, wsConn, getRequest(ch.Address().Bytes(), pb.CacheOption_CACHE_DEFAULT))
				time.Sleep(2 * time.Millisecond)
			}

			// Pause for longer than any control frame deadline, a few times:
			// the replies read in between let the node fill the socket buffer
			// again, which is when a keepalive can get stuck.
			const rounds, perRound = 3, 150
			for range rounds {
				time.Sleep(1100 * time.Millisecond)
				for range perRound {
					if resp := readGet(t, wsConn); resp.Status != pb.Status_STATUS_OK {
						t.Fatalf("expected STATUS_OK, got %v: %s", resp.Status, resp.Error)
					}
				}
			}

			for i := range requests - rounds*perRound {
				if resp := readGet(t, wsConn); resp.Status != pb.Status_STATUS_OK {
					t.Fatalf("reply %d: expected STATUS_OK, got %v: %s", i, resp.Status, resp.Error)
				}
			}
		})
	}
}

// A client that keeps the connection alive with its own pings, rather than
// by answering the node's, must not be dropped by the read timeout.
// nolint:paralleltest
func TestChunkBidirectionalStream_ClientPingsKeepStreamOpen(t *testing.T) {
	cs := inmemchunkstore.New()
	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:                 mockstorer.NewWithChunkStore(cs),
		Post:                   mockpost.New(mockpost.WithAcceptAll()),
		WsPath:                 "/chunks/stream",
		WsHeaders:              streamHeaders(false),
		ChunkStreamReadTimeout: 500 * time.Millisecond,
	})

	// Ping for several read timeouts without sending a request. The node's
	// own pings are a minute apart, so only these keep the stream open.
	for range 20 {
		if err := wsConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	ch := testingc.GenerateTestRandomChunk()
	if err := cs.Put(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	sendPBRequest(t, wsConn, getRequest(ch.Address().Bytes(), pb.CacheOption_CACHE_DEFAULT))
	if resp := readGet(t, wsConn); resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK, got %v: %s", resp.Status, resp.Error)
	}
}
