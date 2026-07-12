// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/v2/contracts"
	"github.com/ethersphere/bee/v2/contracts/codec"
	"github.com/ethersphere/bee/v2/contracts/pb"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var errDirectUploadSessionClosed = errors.New("contracts client: direct upload session closed")

// DirectUploadStream opens a client-streaming DirectUpload session.
func (c *Client) DirectUploadStream(ctx context.Context) (storer.PutterSession, error) {
	sessionID, err := c.openDirectUpload(ctx, "direct-upload-stream")
	if err != nil {
		return nil, err
	}
	stream, err := c.directUploadClient.PutStream(ctx)
	if err != nil {
		c.cleanupDirectUpload(sessionID)
		return nil, fmt.Errorf("direct upload stream: %w", err)
	}
	return &directUploadStreamSession{
		client:    c,
		sessionID: sessionID,
		stream:    stream,
	}, nil
}

// DirectUploadBatchStream opens a client-streaming DirectUpload session that
// sends chunks in batches. batchSize must be greater than zero.
func (c *Client) DirectUploadBatchStream(ctx context.Context, batchSize int) (storer.PutterSession, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("direct upload batch stream: invalid batch size %d", batchSize)
	}
	sessionID, err := c.openDirectUpload(ctx, "direct-upload-batch-stream")
	if err != nil {
		return nil, err
	}
	stream, err := c.directUploadClient.PutBatchStream(ctx)
	if err != nil {
		c.cleanupDirectUpload(sessionID)
		return nil, fmt.Errorf("direct upload batch stream: %w", err)
	}
	return &directUploadBatchStreamSession{
		client:    c,
		sessionID: sessionID,
		batchSize: batchSize,
		stream:    stream,
		pending:   make([]*pb.ChunkMessage, 0, batchSize),
	}, nil
}

func (c *Client) openDirectUpload(ctx context.Context, session string) (uint64, error) {
	resp, err := c.directUploadClient.Open(ctx, &pb.OpenDirectUploadRequest{})
	if err != nil {
		return 0, fmt.Errorf("direct upload open: %w", err)
	}
	c.logger.Info(
		contracts.LogMarkerUpload,
		"transport", "grpc",
		"session", session,
		"session_id", resp.GetSessionId(),
	)
	return resp.GetSessionId(), nil
}

func (c *Client) cleanupDirectUpload(sessionID uint64) {
	_, _ = c.directUploadClient.Cleanup(context.Background(), &pb.DirectUploadCleanupRequest{SessionId: sessionID})
}

type directUploadStreamSession struct {
	client    *Client
	sessionID uint64
	stream    pb.DirectUpload_PutStreamClient

	mu     sync.Mutex
	closed bool
}

func (s *directUploadStreamSession) Put(ctx context.Context, ch swarm.Chunk) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	msg, err := codec.ChunkToProto(ch)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errDirectUploadSessionClosed
	}
	s.client.logger.Info(
		contracts.LogMarkerUpload,
		"transport", "grpc",
		"method", "DirectUpload.PutStream",
		"session_id", s.sessionID,
		"address", ch.Address(),
		"data_len", len(ch.Data()),
	)
	if err := s.stream.Send(&pb.DirectUploadPutRequest{SessionId: s.sessionID, Chunk: msg}); err != nil {
		return mapErr(err)
	}
	return nil
}

func (s *directUploadStreamSession) Done(ref swarm.Address) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errDirectUploadSessionClosed
	}
	s.closed = true
	if _, err := s.stream.CloseAndRecv(); err != nil {
		s.client.cleanupDirectUpload(s.sessionID)
		return mapErr(err)
	}
	_, err := s.client.directUploadClient.Done(context.Background(), &pb.DirectUploadDoneRequest{
		SessionId: s.sessionID,
		Reference: ref.Bytes(),
	})
	return mapErr(err)
}

func (s *directUploadStreamSession) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	_ = s.stream.CloseSend()
	_, err := s.client.directUploadClient.Cleanup(context.Background(), &pb.DirectUploadCleanupRequest{
		SessionId: s.sessionID,
	})
	return mapErr(err)
}

type directUploadBatchStreamSession struct {
	client    *Client
	sessionID uint64
	batchSize int
	stream    pb.DirectUpload_PutBatchStreamClient

	mu      sync.Mutex
	closed  bool
	pending []*pb.ChunkMessage
}

func (s *directUploadBatchStreamSession) Put(ctx context.Context, ch swarm.Chunk) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	msg, err := codec.ChunkToProto(ch)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errDirectUploadSessionClosed
	}
	s.client.logger.Info(
		contracts.LogMarkerUpload,
		"transport", "grpc",
		"method", "DirectUpload.PutBatchStream",
		"session_id", s.sessionID,
		"address", ch.Address(),
		"data_len", len(ch.Data()),
	)
	s.pending = append(s.pending, msg)
	if len(s.pending) < s.batchSize {
		return nil
	}
	return s.flush()
}

func (s *directUploadBatchStreamSession) Done(ref swarm.Address) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errDirectUploadSessionClosed
	}
	s.closed = true
	if err := s.flush(); err != nil {
		s.client.cleanupDirectUpload(s.sessionID)
		return err
	}
	if _, err := s.stream.CloseAndRecv(); err != nil {
		s.client.cleanupDirectUpload(s.sessionID)
		return mapErr(err)
	}
	_, err := s.client.directUploadClient.Done(context.Background(), &pb.DirectUploadDoneRequest{
		SessionId: s.sessionID,
		Reference: ref.Bytes(),
	})
	return mapErr(err)
}

func (s *directUploadBatchStreamSession) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.pending = nil
	_ = s.stream.CloseSend()
	_, err := s.client.directUploadClient.Cleanup(context.Background(), &pb.DirectUploadCleanupRequest{
		SessionId: s.sessionID,
	})
	return mapErr(err)
}

func (s *directUploadBatchStreamSession) flush() error {
	if len(s.pending) == 0 {
		return nil
	}
	req := &pb.DirectUploadPutBatchRequest{
		SessionId: s.sessionID,
		Chunks:    s.pending,
	}
	s.pending = make([]*pb.ChunkMessage, 0, s.batchSize)
	if err := s.stream.Send(req); err != nil {
		return mapErr(err)
	}
	return nil
}

var (
	_ storer.PutterSession = (*directUploadStreamSession)(nil)
	_ storer.PutterSession = (*directUploadBatchStreamSession)(nil)
)
