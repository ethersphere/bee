// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/contracts/codec"
	"github.com/ethersphere/bee/v2/contracts/pb"
	contractserver "github.com/ethersphere/bee/v2/contracts/server"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/pusher"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestDirectUploadPutBatchStream(t *testing.T) {
	t.Parallel()

	session := new(recordingPutterSession)
	client := newDirectUploadServerClient(t, recordingNetStore{session: session})
	open, err := client.Open(context.Background(), &pb.OpenDirectUploadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.PutBatchStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	chunks := directUploadServerTestChunks()
	for _, indexes := range [][]int{{0, 1}, {2}} {
		messages := make([]*pb.ChunkMessage, len(indexes))
		for i, index := range indexes {
			messages[i], err = codec.ChunkToProto(chunks[index])
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := stream.Send(&pb.DirectUploadPutBatchRequest{
			SessionId: open.GetSessionId(),
			Chunks:    messages,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Done(context.Background(), &pb.DirectUploadDoneRequest{
		SessionId: open.GetSessionId(),
		Reference: chunks[0].Address().Bytes(),
	}); err != nil {
		t.Fatal(err)
	}

	got, reference := session.snapshot()
	if len(got) != len(chunks) {
		t.Fatalf("put count: got %d want %d", len(got), len(chunks))
	}
	for i := range chunks {
		if !got[i].Equal(chunks[i]) {
			t.Fatalf("chunk %d: got %s want %s", i, got[i].Address(), chunks[i].Address())
		}
	}
	if !reference.Equal(chunks[0].Address()) {
		t.Fatalf("reference: got %s want %s", reference, chunks[0].Address())
	}
}

func TestDirectUploadPutBatchStreamError(t *testing.T) {
	t.Parallel()

	session := &recordingPutterSession{putErr: errors.New("put error")}
	client := newDirectUploadServerClient(t, recordingNetStore{session: session})
	open, err := client.Open(context.Background(), &pb.OpenDirectUploadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.PutBatchStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	msg, err := codec.ChunkToProto(directUploadServerTestChunks()[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&pb.DirectUploadPutBatchRequest{
		SessionId: open.GetSessionId(),
		Chunks:    []*pb.ChunkMessage{msg},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.CloseAndRecv(); err == nil {
		t.Fatal("CloseAndRecv returned nil error")
	}
}

type recordingNetStore struct {
	session storer.PutterSession
}

func (s recordingNetStore) DirectUpload() storer.PutterSession {
	return s.session
}

func (recordingNetStore) Download(bool) storage.Getter {
	return storage.GetterFunc(func(context.Context, swarm.Address) (swarm.Chunk, error) {
		return nil, storage.ErrNotFound
	})
}

func (recordingNetStore) PusherFeed() <-chan *pusher.Op {
	return nil
}

type recordingPutterSession struct {
	mu        sync.Mutex
	chunks    []swarm.Chunk
	reference swarm.Address
	putErr    error
}

func (s *recordingPutterSession) Put(_ context.Context, ch swarm.Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.putErr != nil {
		return s.putErr
	}
	s.chunks = append(s.chunks, ch)
	return nil
}

func (s *recordingPutterSession) Done(reference swarm.Address) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reference = reference
	return nil
}

func (*recordingPutterSession) Cleanup() error {
	return nil
}

func (s *recordingPutterSession) snapshot() ([]swarm.Chunk, swarm.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]swarm.Chunk(nil), s.chunks...), s.reference
}

func newDirectUploadServerClient(t *testing.T, netStore storer.NetStore) pb.DirectUploadClient {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := contractserver.New(contractserver.Config{
		NetStore:          netStore,
		Logger:            log.Noop,
		DisableRPCLogging: true,
	})
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Error(err)
		}
	})

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	return pb.NewDirectUploadClient(conn)
}

func directUploadServerTestChunks() []swarm.Chunk {
	return []swarm.Chunk{
		swarm.NewChunk(swarm.NewAddress([]byte{1}), []byte{1}),
		swarm.NewChunk(swarm.NewAddress([]byte{2}), []byte{2}),
		swarm.NewChunk(swarm.NewAddress([]byte{3}), []byte{3}),
	}
}

var (
	_ storer.NetStore      = recordingNetStore{}
	_ storer.PutterSession = (*recordingPutterSession)(nil)
)
