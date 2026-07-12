// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/v2/contracts"
	"github.com/ethersphere/bee/v2/contracts/codec"
	"github.com/ethersphere/bee/v2/contracts/pb"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type directUploadSession struct {
	client    *Client
	sessionID uint64
}

func (c *Client) DirectUpload() (storer.PutterSession, error) {
	resp, err := c.directUploadClient.Open(context.Background(), &pb.OpenDirectUploadRequest{})
	if err != nil {
		return nil, fmt.Errorf("direct upload open: %w", err)
	}
	c.logger.Info(contracts.LogMarkerUpload, "transport", "grpc", "session", "direct-upload", "session_id", resp.GetSessionId())
	return &directUploadSession{client: c, sessionID: resp.GetSessionId()}, nil
}

func (s *directUploadSession) Put(ctx context.Context, ch swarm.Chunk) error {
	s.client.logger.Info(
		contracts.LogMarkerUpload,
		"transport", "grpc",
		"method", "DirectUpload.Put",
		"session_id", s.sessionID,
		"address", ch.Address(),
		"data_len", len(ch.Data()),
	)
	msg, err := codec.ChunkToProto(ch)
	if err != nil {
		return err
	}
	_, err = s.client.directUploadClient.Put(ctx, &pb.DirectUploadPutRequest{
		SessionId: s.sessionID,
		Chunk:     msg,
	})
	return mapErr(err)
}

func (s *directUploadSession) Done(ref swarm.Address) error {
	s.client.logger.Info(
		contracts.LogMarkerUpload,
		"transport", "grpc",
		"method", "DirectUpload.Done",
		"session_id", s.sessionID,
		"reference", ref,
	)
	_, err := s.client.directUploadClient.Done(context.Background(), &pb.DirectUploadDoneRequest{
		SessionId: s.sessionID,
		Reference: ref.Bytes(),
	})
	return mapErr(err)
}

func (s *directUploadSession) Cleanup() error {
	s.client.logger.Debug(
		contracts.LogPrefix+" client rpc",
		"method", "DirectUpload.Cleanup",
		"session_id", s.sessionID,
	)
	_, err := s.client.directUploadClient.Cleanup(context.Background(), &pb.DirectUploadCleanupRequest{
		SessionId: s.sessionID,
	})
	return mapErr(err)
}
