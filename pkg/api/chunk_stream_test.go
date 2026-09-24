// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strconv"
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

// nolint:paralleltest
func TestChunkBidirectionalStream_UploadAndDownload(t *testing.T) {
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

	var (
		cs                          = inmemchunkstore.New()
		storerMock                  = mockstorer.NewWithChunkStore(cs)
		_, wsConn, _, chanStorer, _ = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			WsPath:       "/chunks/stream",
			WsHeaders:    wsHeaders,
			DirectUpload: true,
		})
	)

	// 1. Upload chunks via PutRequest
	const numChunks = 10
	chunks := make([]swarm.Chunk, numChunks)
	for i := range numChunks {
		chunks[i] = testingc.GenerateTestRandomChunk()
		// Seed in cs for subsequent Get assertions
		if err := cs.Put(context.Background(), chunks[i]); err != nil {
			t.Fatal(err)
		}
		sendPBRequest(t, wsConn, &pb.Request{
			Id: uint64(i + 1),
			Body: &pb.Request_Put{
				Put: &pb.PutRequest{
					Data: chunks[i].Data(),
					Type: pb.ChunkType_CHUNK_TYPE_CAC,
				},
			},
		})
	}

	// Collect Put responses
	putResponses := make(map[uint64]*pb.Response)
	for range numChunks {
		resp := readPBResponse(t, wsConn)
		putResponses[resp.Id] = resp
	}

	for i := range numChunks {
		reqId := uint64(i + 1)
		resp, ok := putResponses[reqId]
		if !ok {
			t.Fatalf("missing response for put request id %d", reqId)
		}
		if resp.Status != pb.Status_STATUS_OK {
			t.Fatalf("expected STATUS_OK, got %v (err: %s)", resp.Status, resp.Error)
		}
		if !bytes.Equal(resp.Address, chunks[i].Address().Bytes()) {
			t.Fatalf("put response address mismatch: got %x, want %x", resp.Address, chunks[i].Address().Bytes())
		}
		if !chanStorer.Has(chunks[i].Address()) {
			t.Fatalf("chunk %s not found in chan store", chunks[i].Address())
		}
	}

	// 2. Download chunks via GetRequest on the SAME connection
	for i := range numChunks {
		sendPBRequest(t, wsConn, &pb.Request{
			Id: uint64(100 + i + 1),
			Body: &pb.Request_Get{
				Get: &pb.GetRequest{
					Address: chunks[i].Address().Bytes(),
				},
			},
		})
	}

	// Collect Get responses
	getResponses := make(map[uint64]*pb.Response)
	for range numChunks {
		resp := readPBResponse(t, wsConn)
		getResponses[resp.Id] = resp
	}

	for i := range numChunks {
		reqId := uint64(100 + i + 1)
		resp, ok := getResponses[reqId]
		if !ok {
			t.Fatalf("missing response for get request id %d", reqId)
		}
		if resp.Status != pb.Status_STATUS_OK {
			t.Fatalf("expected STATUS_OK for get, got %v (err: %s)", resp.Status, resp.Error)
		}
		if !bytes.Equal(resp.Address, chunks[i].Address().Bytes()) {
			t.Fatalf("get response address mismatch: got %x, want %x", resp.Address, chunks[i].Address().Bytes())
		}
		if !bytes.Equal(resp.Data, chunks[i].Data()) {
			t.Fatalf("get response data mismatch")
		}
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_Interleaved(t *testing.T) {
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

	var (
		cs                 = inmemchunkstore.New()
		storerMock         = mockstorer.NewWithChunkStore(cs)
		_, wsConn, _, _, _ = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			WsPath:       "/chunks/stream",
			WsHeaders:    wsHeaders,
			DirectUpload: true,
		})
	)

	// Pre-seed 10 chunks to get
	const count = 10
	getChunks := make([]swarm.Chunk, count)
	for i := range count {
		getChunks[i] = testingc.GenerateTestRandomChunk()
		if err := cs.Put(context.Background(), getChunks[i]); err != nil {
			t.Fatal(err)
		}
	}

	// Prepare 10 chunks to put
	putChunks := make([]swarm.Chunk, count)
	for i := range count {
		putChunks[i] = testingc.GenerateTestRandomChunk()
	}

	// Interleave Get and Put requests
	for i := range count {
		// Send Put
		sendPBRequest(t, wsConn, &pb.Request{
			Id: uint64(1000 + i),
			Body: &pb.Request_Put{
				Put: &pb.PutRequest{
					Data: putChunks[i].Data(),
					Type: pb.ChunkType_CHUNK_TYPE_CAC,
				},
			},
		})
		// Send Get
		sendPBRequest(t, wsConn, &pb.Request{
			Id: uint64(2000 + i),
			Body: &pb.Request_Get{
				Get: &pb.GetRequest{
					Address: getChunks[i].Address().Bytes(),
				},
			},
		})
	}

	// Collect all 20 responses
	responses := make(map[uint64]*pb.Response)
	for range count * 2 {
		resp := readPBResponse(t, wsConn)
		responses[resp.Id] = resp
	}

	for i := range count {
		putResp := responses[uint64(1000+i)]
		if putResp == nil || putResp.Status != pb.Status_STATUS_OK {
			t.Fatalf("put request %d failed: %v", 1000+i, putResp)
		}
		if !bytes.Equal(putResp.Address, putChunks[i].Address().Bytes()) {
			t.Fatalf("put address mismatch for %d", 1000+i)
		}

		getResp := responses[uint64(2000+i)]
		if getResp == nil || getResp.Status != pb.Status_STATUS_OK {
			t.Fatalf("get request %d failed: %v", 2000+i, getResp)
		}
		if !bytes.Equal(getResp.Data, getChunks[i].Data()) {
			t.Fatalf("get data mismatch for %d", 2000+i)
		}
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_PerRequestErrors(t *testing.T) {
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

	var (
		cs                 = inmemchunkstore.New()
		storerMock         = mockstorer.NewWithChunkStore(cs)
		_, wsConn, _, _, _ = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			WsPath:       "/chunks/stream",
			WsHeaders:    wsHeaders,
			DirectUpload: true,
		})
	)

	// 1. Get non-existent chunk -> STATUS_NOT_FOUND
	nonExistentAddr := testingc.GenerateTestRandomChunk().Address()
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 1,
		Body: &pb.Request_Get{
			Get: &pb.GetRequest{
				Address: nonExistentAddr.Bytes(),
			},
		},
	})
	resp1 := readPBResponse(t, wsConn)
	if resp1.Id != 1 || resp1.Status != pb.Status_STATUS_NOT_FOUND {
		t.Fatalf("expected STATUS_NOT_FOUND, got %v (err: %s)", resp1.Status, resp1.Error)
	}

	// 2. Get with bad address length -> STATUS_BAD_REQUEST
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 2,
		Body: &pb.Request_Get{
			Get: &pb.GetRequest{
				Address: []byte("too-short"),
			},
		},
	})
	resp2 := readPBResponse(t, wsConn)
	if resp2.Id != 2 || resp2.Status != pb.Status_STATUS_BAD_REQUEST {
		t.Fatalf("expected STATUS_BAD_REQUEST, got %v (err: %s)", resp2.Status, resp2.Error)
	}

	// 3. Put with insufficient data (< span size) -> STATUS_BAD_REQUEST
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 3,
		Body: &pb.Request_Put{
			Put: &pb.PutRequest{
				Data: []byte{1, 2, 3},
				Type: pb.ChunkType_CHUNK_TYPE_CAC,
			},
		},
	})
	resp3 := readPBResponse(t, wsConn)
	if resp3.Id != 3 || resp3.Status != pb.Status_STATUS_BAD_REQUEST {
		t.Fatalf("expected STATUS_BAD_REQUEST, got %v (err: %s)", resp3.Status, resp3.Error)
	}

	// 4. Put with unspecified chunk type -> STATUS_BAD_REQUEST
	validChunk := testingc.GenerateTestRandomChunk()
	if err := cs.Put(context.Background(), validChunk); err != nil {
		t.Fatal(err)
	}
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 4,
		Body: &pb.Request_Put{
			Put: &pb.PutRequest{
				Data: validChunk.Data(),
				Type: pb.ChunkType_CHUNK_TYPE_UNSPECIFIED,
			},
		},
	})
	resp4 := readPBResponse(t, wsConn)
	if resp4.Id != 4 || resp4.Status != pb.Status_STATUS_BAD_REQUEST {
		t.Fatalf("expected STATUS_BAD_REQUEST for unspecified chunk type, got %v (err: %s)", resp4.Status, resp4.Error)
	}
	if resp4.Error != "unspecified or invalid chunk type" {
		t.Fatalf("expected 'unspecified or invalid chunk type', got %q", resp4.Error)
	}

	// 5. Empty request (neither get nor put) -> STATUS_BAD_REQUEST
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 5,
	})
	resp5 := readPBResponse(t, wsConn)
	if resp5.Id != 5 || resp5.Status != pb.Status_STATUS_BAD_REQUEST {
		t.Fatalf("expected STATUS_BAD_REQUEST, got %v (err: %s)", resp5.Status, resp5.Error)
	}

	// 6. CRITICAL: Connection is STILL ALIVE. A valid Put succeeds!
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 6,
		Body: &pb.Request_Put{
			Put: &pb.PutRequest{
				Data: validChunk.Data(),
				Type: pb.ChunkType_CHUNK_TYPE_CAC,
			},
		},
	})
	resp6 := readPBResponse(t, wsConn)
	if resp6.Id != 6 || resp6.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK, got %v (err: %s)", resp6.Status, resp6.Error)
	}
	if !bytes.Equal(resp6.Address, validChunk.Address().Bytes()) {
		t.Fatalf("address mismatch: got %x, want %x", resp6.Address, validChunk.Address().Bytes())
	}

	// 7. And valid Get succeeds!
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 7,
		Body: &pb.Request_Get{
			Get: &pb.GetRequest{
				Address: validChunk.Address().Bytes(),
			},
		},
	})
	resp7 := readPBResponse(t, wsConn)
	if resp7.Id != 7 || resp7.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK, got %v (err: %s)", resp7.Status, resp7.Error)
	}
	if !bytes.Equal(resp7.Data, validChunk.Data()) {
		t.Fatalf("data mismatch on get after error recovery")
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
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

	injectedErr := errors.New("network push failed")
	storerMock := &failingDirectUploadStorer{
		Storer:  mockstorer.New(),
		failErr: injectedErr,
	}

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    wsHeaders,
		DirectUpload: true,
	})

	ch := testingc.GenerateTestRandomChunk()
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 101,
		Body: &pb.Request_Put{
			Put: &pb.PutRequest{
				Data: ch.Data(),
				Type: pb.ChunkType_CHUNK_TYPE_CAC,
			},
		},
	})
	resp := readPBResponse(t, wsConn)
	if resp.Id != 101 {
		t.Fatalf("expected id 101, got %d", resp.Id)
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
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

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
		WsHeaders:    wsHeaders,
		DirectUpload: true,
	})

	targetChunk := testingc.GenerateTestRandomChunk()
	if err := cs.Put(context.Background(), targetChunk); err != nil {
		t.Fatal(err)
	}

	// Send Put requests that will be blocked in DirectUpload().Put
	// numPuts saturates all upload workers and queues in putQueue,
	// while strictly ruling out a single shared pool of workers.
	numPuts := 2*api.DefaultStreamSubWorkers + 4
	for i := range numPuts {
		ch := testingc.GenerateTestRandomChunk()
		sendPBRequest(t, wsConn, &pb.Request{
			Id: uint64(i + 1),
			Body: &pb.Request_Put{
				Put: &pb.PutRequest{
					Data: ch.Data(),
					Type: pb.ChunkType_CHUNK_TYPE_CAC,
				},
			},
		})
	}

	// Brief pause to ensure all upload workers have picked up the Puts and are blocked
	time.Sleep(50 * time.Millisecond)

	// Send 1 Get request behind the blocked Puts
	const targetGetID = 9999
	sendPBRequest(t, wsConn, &pb.Request{
		Id: targetGetID,
		Body: &pb.Request_Get{
			Get: &pb.GetRequest{
				Address: targetChunk.Address().Bytes(),
			},
		},
	})

	// The Get request MUST complete and receive STATUS_OK while Puts are still blocked
	resp := readPBResponse(t, wsConn)
	if resp.Id != targetGetID {
		t.Fatalf("expected target get ID %d, got %d", targetGetID, resp.Id)
	}
	if resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK for fast get, got %v", resp.Status)
	}
	if !bytes.Equal(resp.Data, targetChunk.Data()) {
		t.Fatalf("corrupted chunk data on get")
	}

	// Unblock the Puts and verify they all complete successfully
	close(blockCh)
	for range numPuts {
		putResp := readPBResponse(t, wsConn)
		if putResp.Status != pb.Status_STATUS_OK {
			t.Fatalf("expected STATUS_OK for unblocked put %d, got %v", putResp.Id, putResp.Status)
		}
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_QueueBusy(t *testing.T) {
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

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
		WsHeaders:    wsHeaders,
		DirectUpload: true,
	})

	// api.DefaultStreamSubWorkers upload workers + api.MaxStreamQueueSize putQueue buffer
	capacity := api.MaxStreamQueueSize + api.DefaultStreamSubWorkers
	ch := testingc.GenerateTestRandomChunk()
	for i := range capacity {
		sendPBRequest(t, wsConn, &pb.Request{
			Id: uint64(i + 1),
			Body: &pb.Request_Put{
				Put: &pb.PutRequest{
					Data: ch.Data(),
					Type: pb.ChunkType_CHUNK_TYPE_CAC,
				},
			},
		})
	}

	// Send request capacity+1 which exceeds queue capacity -> immediately rejected with STATUS_BUSY
	sendPBRequest(t, wsConn, &pb.Request{
		Id: uint64(capacity + 1),
		Body: &pb.Request_Put{
			Put: &pb.PutRequest{
				Data: ch.Data(),
				Type: pb.ChunkType_CHUNK_TYPE_CAC,
			},
		},
	})

	busyResp := readPBResponse(t, wsConn)
	if busyResp.Id != uint64(capacity+1) || busyResp.Status != pb.Status_STATUS_BUSY {
		t.Fatalf("expected STATUS_BUSY, got %v (err: %s)", busyResp.Status, busyResp.Error)
	}
	if busyResp.Error != "request queue full" {
		t.Fatalf("expected error 'request queue full', got %q", busyResp.Error)
	}

	// Unblock workers and drain remaining responses
	close(blockCh)
	for range capacity {
		resp := readPBResponse(t, wsConn)
		if resp.Status != pb.Status_STATUS_OK {
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

	// No Swarm-Postage-Batch-Id header
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

	var (
		storerMock         = mockstorer.New()
		_, wsConn, _, _, _ = newTestServer(t, testServerOptions{
			Storer:       storerMock,
			Post:         mockpost.New(mockpost.WithAcceptAll()),
			BatchStore:   batchStore,
			WsPath:       "/chunks/stream",
			WsHeaders:    wsHeaders,
			DirectUpload: true,
		})
	)

	// 1. Put without stamp should fail with STATUS_BAD_REQUEST
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 1,
		Body: &pb.Request_Put{
			Put: &pb.PutRequest{
				Data: ch.Data(),
				Type: pb.ChunkType_CHUNK_TYPE_CAC,
			},
		},
	})
	resp1 := readPBResponse(t, wsConn)
	if resp1.Id != 1 || resp1.Status != pb.Status_STATUS_BAD_REQUEST {
		t.Fatalf("expected STATUS_BAD_REQUEST without stamp, got %v (err: %s)", resp1.Status, resp1.Error)
	}

	// 2. Put with valid stamp succeeds
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 2,
		Body: &pb.Request_Put{
			Put: &pb.PutRequest{
				Data:  ch.Data(),
				Stamp: stampBytes,
				Type:  pb.ChunkType_CHUNK_TYPE_CAC,
			},
		},
	})
	resp2 := readPBResponse(t, wsConn)
	if resp2.Id != 2 || resp2.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK with stamp, got %v (err: %s)", resp2.Status, resp2.Error)
	}
	if !bytes.Equal(resp2.Address, ch.Address().Bytes()) {
		t.Fatalf("address mismatch: got %x, want %x", resp2.Address, ch.Address().Bytes())
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
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 42,
		Body: &pb.Request_Put{
			Put: &pb.PutRequest{
				Data: ch.Data(),
				Type: pb.ChunkType_CHUNK_TYPE_CAC,
			},
		},
	})
	resp := readPBResponse(t, wsConn)
	if resp.Id != 42 || resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK via ?mode=stream, got %v (err: %s)", resp.Status, resp.Error)
	}

	// Invalid mode query parameter returns 400
	jsonhttptest.Request(t, http.DefaultClient, http.MethodGet, "http://"+addr+"/chunks/stream?mode=invalid", http.StatusBadRequest)
}

// nolint:paralleltest
func TestChunkBidirectionalStream_CacheOption(t *testing.T) {
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    wsHeaders,
		DirectUpload: true,
	})

	ch := testingc.GenerateTestRandomChunk()
	if err := cs.Put(context.Background(), ch); err != nil {
		t.Fatal(err)
	}

	// Get with CACHE_DISABLE
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 1,
		Body: &pb.Request_Get{
			Get: &pb.GetRequest{
				Address: ch.Address().Bytes(),
				Cache:   pb.CacheOption_CACHE_DISABLE,
			},
		},
	})
	resp1 := readPBResponse(t, wsConn)
	if resp1.Id != 1 || resp1.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK with CACHE_DISABLE, got %v", resp1.Status)
	}

	// Get with CACHE_ENABLE
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 2,
		Body: &pb.Request_Get{
			Get: &pb.GetRequest{
				Address: ch.Address().Bytes(),
				Cache:   pb.CacheOption_CACHE_ENABLE,
			},
		},
	})
	resp2 := readPBResponse(t, wsConn)
	if resp2.Id != 2 || resp2.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK with CACHE_ENABLE, got %v", resp2.Status)
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_ProtocolViolation(t *testing.T) {
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

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

	// Send text frame instead of binary
	if err := wsConn.WriteMessage(websocket.TextMessage, []byte("invalid text message")); err != nil {
		t.Fatal(err)
	}

	_, _, err := wsConn.ReadMessage()
	if err == nil {
		t.Fatal("expected error on protocol violation, got nil")
	}
	var cerr *websocket.CloseError
	if errors.As(err, &cerr) {
		if cerr.Code != websocket.CloseUnsupportedData {
			t.Fatalf("expected close code %d, got %d", websocket.CloseUnsupportedData, cerr.Code)
		}
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_Shutdown(t *testing.T) {
	var svc *api.Service
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    wsHeaders,
		Service:      &svc,
		DirectUpload: true,
	})

	ch := testingc.GenerateTestRandomChunk()
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 1,
		Body: &pb.Request_Put{
			Put: &pb.PutRequest{
				Data: ch.Data(),
				Type: pb.ChunkType_CHUNK_TYPE_CAC,
			},
		},
	})
	resp := readPBResponse(t, wsConn)
	if resp.Id != 1 || resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK, got %v", resp.Status)
	}

	// Trigger shutdown while stream was active
	start := time.Now()
	if err := svc.Close(); err != nil {
		t.Fatalf("Close after %v: %v", time.Since(start), err)
	}

	// Verify websocket receives close or is closed
	_ = wsConn.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err := wsConn.ReadMessage()
	if err == nil {
		t.Fatal("expected error reading from closed connection, got nil")
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_FeedSizedSOC(t *testing.T) {
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:       storerMock,
		Post:         mockpost.New(mockpost.WithAcceptAll()),
		WsPath:       "/chunks/stream",
		WsHeaders:    wsHeaders,
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

	// Upload feed-sized SOC with explicit Type: CHUNK_TYPE_SOC
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 1,
		Body: &pb.Request_Put{
			Put: &pb.PutRequest{
				Data: socChunk.Data(),
				Type: pb.ChunkType_CHUNK_TYPE_SOC,
			},
		},
	})

	resp := readPBResponse(t, wsConn)
	if resp.Id != 1 {
		t.Fatalf("expected ID 1, got %d", resp.Id)
	}
	if resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK, got %v: %s", resp.Status, resp.Error)
	}
	if !bytes.Equal(resp.Address, socChunk.Address().Bytes()) {
		t.Fatalf("expected SOC address %s, got %x", socChunk.Address(), resp.Address)
	}

	// Seed SOC in cs so Lookup().Get can find it during retrieval assertion
	if err := cs.Put(context.Background(), socChunk); err != nil {
		t.Fatal(err)
	}

	// Retrieve the SOC chunk using GetRequest and verify its data matches
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 2,
		Body: &pb.Request_Get{
			Get: &pb.GetRequest{
				Address: socChunk.Address().Bytes(),
			},
		},
	})

	getResp := readPBResponse(t, wsConn)
	if getResp.Id != 2 {
		t.Fatalf("expected ID 2, got %d", getResp.Id)
	}
	if getResp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK on get SOC, got %v: %s", getResp.Status, getResp.Error)
	}
	if !bytes.Equal(getResp.Data, socChunk.Data()) {
		t.Fatalf("retrieved SOC data does not match uploaded SOC data")
	}

	// Verify corrupted SOC data (e.g. invalid signature recovery ID) is rejected with STATUS_BAD_REQUEST
	corruptedData := make([]byte, len(socChunk.Data()))
	copy(corruptedData, socChunk.Data())
	corruptedData[swarm.HashSize+swarm.SocSignatureSize-1] = 99 // invalid recovery byte
	sendPBRequest(t, wsConn, &pb.Request{
		Id: 3,
		Body: &pb.Request_Put{
			Put: &pb.PutRequest{
				Data: corruptedData,
				Type: pb.ChunkType_CHUNK_TYPE_SOC,
			},
		},
	})
	badResp := readPBResponse(t, wsConn)
	if badResp.Id != 3 {
		t.Fatalf("expected ID 3, got %d", badResp.Id)
	}
	if badResp.Status != pb.Status_STATUS_BAD_REQUEST {
		t.Fatalf("expected STATUS_BAD_REQUEST for corrupted SOC, got %v", badResp.Status)
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_TagWithPerChunkStamp(t *testing.T) {
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	owner, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}

	batchStore := mockbatchstore.New(
		mockbatchstore.WithAcceptAllExistsFunc(),
		mockbatchstore.WithBatch(&postage.Batch{
			Owner: owner.Bytes(),
		}),
	)

	storerMock := mockstorer.New()
	tagSession, err := storerMock.NewSession()
	if err != nil {
		t.Fatal(err)
	}

	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmTagHeader, strconv.FormatUint(tagSession.TagID, 10))
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)
	// SwarmPostageBatchIdHeader is intentionally omitted to verify tag support with per-chunk stamps

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:     storerMock,
		Post:       mockpost.New(mockpost.WithAcceptAll()),
		BatchStore: batchStore,
		WsPath:     "/chunks/stream",
		WsHeaders:  wsHeaders,
	})

	ch := testingc.GenerateTestRandomChunk()
	stamp := testingpostage.MustNewValidStamp(signer, ch.Address())
	stampBytes, err := stamp.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	sendPBRequest(t, wsConn, &pb.Request{
		Id: 1,
		Body: &pb.Request_Put{
			Put: &pb.PutRequest{
				Data:  ch.Data(),
				Stamp: stampBytes,
				Type:  pb.ChunkType_CHUNK_TYPE_CAC,
			},
		},
	})

	resp := readPBResponse(t, wsConn)
	if resp.Id != 1 || resp.Status != pb.Status_STATUS_OK {
		t.Fatalf("expected STATUS_OK, got %v: %s", resp.Status, resp.Error)
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_ShutdownWithPendingWrites(t *testing.T) {
	var svc *api.Service
	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set(api.SwarmPostageBatchIdHeader, batchOkStr)
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

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
		WsHeaders:    wsHeaders,
		Service:      &svc,
		DirectUpload: true,
	})

	// Send several Puts that block in the storer
	for i := range 5 {
		ch := testingc.GenerateTestRandomChunk()
		sendPBRequest(t, wsConn, &pb.Request{
			Id: uint64(i + 1),
			Body: &pb.Request_Put{
				Put: &pb.PutRequest{
					Data: ch.Data(),
					Type: pb.ChunkType_CHUNK_TYPE_CAC,
				},
			},
		})
	}

	// Trigger node shutdown while upload workers are actively blocked
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- svc.Close()
	}()

	// svc.Close() should terminate promptly without deadlock even with pending writes
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
	_, _, err := wsConn.ReadMessage()
	if err == nil {
		t.Fatal("expected error reading from closed connection, got nil")
	}
}

// nolint:paralleltest
func TestChunkBidirectionalStream_ShutdownClientStoppedReading(t *testing.T) {
	var svc *api.Service
	cs := inmemchunkstore.New()
	storerMock := mockstorer.NewWithChunkStore(cs)

	wsHeaders := http.Header{}
	wsHeaders.Set(api.ContentTypeHeader, "application/octet-stream")
	wsHeaders.Set("Sec-WebSocket-Protocol", api.ChunkStreamSubprotocol)

	_, wsConn, _, _, _ := newTestServer(t, testServerOptions{
		Storer:    storerMock,
		Post:      mockpost.New(mockpost.WithAcceptAll()),
		WsPath:    "/chunks/stream",
		WsHeaders: wsHeaders,
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
		req := &pb.Request{
			Id: uint64(i + 1),
			Body: &pb.Request_Get{
				Get: &pb.GetRequest{
					Address: chunks[i].Address().Bytes(),
				},
			},
		}
		data, err := req.Marshal()
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
