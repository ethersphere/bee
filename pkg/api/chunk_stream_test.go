// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"net/http"
	"sync"
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
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
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

// nolint:paralleltest
func TestChunkBidirectionalStream_CacheOption(t *testing.T) {
	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    streamHeaders(true),
		DirectUpload: true,
	})

	ch := testingc.GenerateTestRandomChunk()
	if err := cs.Put(context.Background(), ch); err != nil {
		t.Fatal(err)
	}

	for _, opt := range []pb.CacheOption{pb.CacheOption_CACHE_DISABLE, pb.CacheOption_CACHE_ENABLE} {
		sendPBRequest(t, wsConn, getRequest(ch.Address().Bytes(), opt))
		if resp := readGet(t, wsConn); resp.Status != pb.Status_STATUS_OK {
			t.Fatalf("expected STATUS_OK with %v, got %v", opt, resp.Status)
		}
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

	post := mockpost.New(mockpost.WithIssuer(postage.NewStampIssuer("a", "a", batchA, big.NewInt(3), 24, 6, 1000, false)))
	if err := post.Add(postage.NewStampIssuer("b", "b", batchB, big.NewInt(3), 24, 6, 1000, false)); err != nil {
		t.Fatal(err)
	}

	h := streamHeaders(false)
	h.Set(api.SwarmPostageBatchIdHeader, hex.EncodeToString(batchA))

	_, wsConn, _, chanStorer, _ := newTestServer(t, testServerOptions{
		Storer:       mockstorer.New(),
		Post:         post,
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

	for _, tc := range []struct {
		name      string
		batchID   []byte
		wantBatch []byte
	}{
		{name: "no batch on the request uses the connection's", wantBatch: batchA},
		{name: "a batch on the request overrides it", batchID: batchB, wantBatch: batchB},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := testingc.GenerateTestRandomChunk()
			sendPBRequest(t, wsConn, putRequest(ch.Address().Bytes(), ch.Data(), nil, tc.batchID))
			if resp := readPut(t, wsConn); resp.Status != pb.Status_STATUS_OK {
				t.Fatalf("expected STATUS_OK, got %v (err: %s)", resp.Status, resp.Error)
			}
			err := spinlock.Wait(streamTestTimeout, func() bool {
				mu.Lock()
				defer mu.Unlock()
				return stampBatch[ch.Address().ByteString()] != nil
			})
			if err != nil {
				t.Fatal("chunk was not pushed")
			}
			mu.Lock()
			got := stampBatch[ch.Address().ByteString()]
			mu.Unlock()
			if !bytes.Equal(got, tc.wantBatch) {
				t.Fatalf("stamped with batch %x, want %x", got, tc.wantBatch)
			}
		})
	}

	// A batch the node has no issuer for is rejected for that request only.
	ch := testingc.GenerateTestRandomChunk()
	sendPBRequest(t, wsConn, putRequest(ch.Address().Bytes(), ch.Data(), nil, unknown))
	if resp := readPut(t, wsConn); resp.Status != pb.Status_STATUS_BAD_REQUEST || resp.Error != "postage batch not found" {
		t.Fatalf("expected an unknown batch to be rejected, got %v (err: %s)", resp.Status, resp.Error)
	}
	sendPBRequest(t, wsConn, chunkPut(ch))
	if resp := readPut(t, wsConn); resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected the connection to stay usable, got %v (err: %s)", resp.Status, resp.Error)
	}
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
