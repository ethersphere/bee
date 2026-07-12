// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"slices"
	"sync"
	"testing"

	"github.com/ethersphere/bee/v2/contracts/client"
	"github.com/ethersphere/bee/v2/contracts/codec"
	"github.com/ethersphere/bee/v2/contracts/pb"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestDirectUploadStreamingSessions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		open            func(context.Context, *client.Client) (storer.PutterSession, error)
		wantBatchSizes  []int
		wantSingleCount int
	}{
		{
			name: "stream",
			open: func(ctx context.Context, c *client.Client) (storer.PutterSession, error) {
				return c.DirectUploadStream(ctx)
			},
			wantSingleCount: 18,
		},
		{
			name: "batch_stream",
			open: func(ctx context.Context, c *client.Client) (storer.PutterSession, error) {
				return c.DirectUploadBatchStream(ctx, 16)
			},
			wantBatchSizes: []int{16, 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := &recordingDirectUploadServer{}
			c := newRecordingDirectUploadClient(t, server)
			chunks := directUploadStreamingTestChunks(t, 18)
			session, err := tc.open(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			for _, ch := range chunks {
				if err := session.Put(context.Background(), ch); err != nil {
					t.Fatal(err)
				}
			}
			if err := session.Done(chunks[0].Address()); err != nil {
				t.Fatal(err)
			}

			messages, batchSizes, reference, openCount, doneCount := server.snapshot()
			if openCount != 1 {
				t.Fatalf("open count: got %d want 1", openCount)
			}
			if doneCount != 1 {
				t.Fatalf("done count: got %d want 1", doneCount)
			}
			if !reference.Equal(chunks[0].Address()) {
				t.Fatalf("reference: got %s want %s", reference, chunks[0].Address())
			}
			if len(messages) != len(chunks) {
				t.Fatalf("message count: got %d want %d", len(messages), len(chunks))
			}
			for i, ch := range chunks {
				want, err := codec.ChunkToProto(ch)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(messages[i].GetAddress(), want.GetAddress()) ||
					!bytes.Equal(messages[i].GetData(), want.GetData()) ||
					!bytes.Equal(messages[i].GetStamp(), want.GetStamp()) {
					t.Fatalf("message %d does not match input chunk", i)
				}
			}
			if tc.wantSingleCount > 0 && len(batchSizes) != tc.wantSingleCount {
				t.Fatalf("stream message count: got %d want %d", len(batchSizes), tc.wantSingleCount)
			}
			if tc.wantBatchSizes != nil && !slices.Equal(batchSizes, tc.wantBatchSizes) {
				t.Fatalf("batch sizes: got %v want %v", batchSizes, tc.wantBatchSizes)
			}
		})
	}
}

func TestDirectUploadStreamingError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		open func(context.Context, *client.Client) (storer.PutterSession, error)
	}{
		{
			name: "stream",
			open: func(ctx context.Context, c *client.Client) (storer.PutterSession, error) {
				return c.DirectUploadStream(ctx)
			},
		},
		{
			name: "batch_stream",
			open: func(ctx context.Context, c *client.Client) (storer.PutterSession, error) {
				return c.DirectUploadBatchStream(ctx, 1)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := &recordingDirectUploadServer{streamErr: errors.New("put error")}
			c := newRecordingDirectUploadClient(t, server)
			session, err := tc.open(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			chunks := directUploadStreamingTestChunks(t, 1)
			if err := session.Put(context.Background(), chunks[0]); err != nil {
				t.Fatal(err)
			}
			if err := session.Done(chunks[0].Address()); err == nil {
				t.Fatal("Done returned nil error")
			}
		})
	}
}

func TestDirectUploadBatchStreamInvalidSize(t *testing.T) {
	t.Parallel()

	c := newRecordingDirectUploadClient(t, &recordingDirectUploadServer{})
	if _, err := c.DirectUploadBatchStream(context.Background(), 0); err == nil {
		t.Fatal("DirectUploadBatchStream returned nil error")
	}
}

type recordingDirectUploadServer struct {
	pb.UnimplementedDirectUploadServer

	mu         sync.Mutex
	messages   []*pb.ChunkMessage
	batchSizes []int
	reference  swarm.Address
	openCount  int
	doneCount  int
	streamErr  error
	sessionID  uint64
}

func (s *recordingDirectUploadServer) Open(context.Context, *pb.OpenDirectUploadRequest) (*pb.OpenDirectUploadResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openCount++
	s.sessionID++
	return &pb.OpenDirectUploadResponse{SessionId: s.sessionID}, nil
}

func (s *recordingDirectUploadServer) PutStream(stream grpc.ClientStreamingServer[pb.DirectUploadPutRequest, pb.PutResponse]) error {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&pb.PutResponse{})
		}
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.messages = append(s.messages, req.GetChunk())
		s.batchSizes = append(s.batchSizes, 1)
		streamErr := s.streamErr
		s.mu.Unlock()
		if streamErr != nil {
			return streamErr
		}
	}
}

func (s *recordingDirectUploadServer) PutBatchStream(stream grpc.ClientStreamingServer[pb.DirectUploadPutBatchRequest, pb.PutResponse]) error {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&pb.PutResponse{})
		}
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.messages = append(s.messages, req.GetChunks()...)
		s.batchSizes = append(s.batchSizes, len(req.GetChunks()))
		streamErr := s.streamErr
		s.mu.Unlock()
		if streamErr != nil {
			return streamErr
		}
	}
}

func (s *recordingDirectUploadServer) Done(_ context.Context, req *pb.DirectUploadDoneRequest) (*pb.DirectUploadDoneResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.doneCount++
	s.reference = swarm.NewAddress(req.GetReference())
	return &pb.DirectUploadDoneResponse{}, nil
}

func (s *recordingDirectUploadServer) Cleanup(context.Context, *pb.DirectUploadCleanupRequest) (*pb.DirectUploadCleanupResponse, error) {
	return &pb.DirectUploadCleanupResponse{}, nil
}

func (s *recordingDirectUploadServer) snapshot() ([]*pb.ChunkMessage, []int, swarm.Address, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.messages), slices.Clone(s.batchSizes), s.reference, s.openCount, s.doneCount
}

func newRecordingDirectUploadClient(t *testing.T, service pb.DirectUploadServer) *client.Client {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterDirectUploadServer(server, service)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.GracefulStop)

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
	return client.NewFromConn(conn, log.Noop)
}

func directUploadStreamingTestChunks(t *testing.T, count int) []swarm.Chunk {
	t.Helper()

	chunks := make([]swarm.Chunk, count)
	for i := range chunks {
		ch, err := cac.New([]byte{byte(i), byte(i + 1)})
		if err != nil {
			t.Fatal(err)
		}
		chunks[i] = ch.WithStamp(postage.NewStamp(
			bytes.Repeat([]byte{1}, swarm.HashSize),
			bytes.Repeat([]byte{byte(i)}, postage.IndexSize),
			bytes.Repeat([]byte{byte(i + 1)}, postage.IndexSize),
			bytes.Repeat([]byte{4}, swarm.SocSignatureSize),
		))
	}
	return chunks
}

var _ pb.DirectUploadServer = (*recordingDirectUploadServer)(nil)
