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

type uploadSession struct {
	client    *Client
	sessionID uint64
}

func (c *Client) Upload(ctx context.Context, pin bool, tagID uint64) (storer.PutterSession, error) {
	resp, err := c.uploadClient.Open(ctx, &pb.OpenUploadRequest{
		Pin:   pin,
		TagId: tagID,
	})
	if err != nil {
		return nil, fmt.Errorf("upload open: %w", mapErr(err))
	}
	c.logger.Info(contracts.LogMarkerUpload, "transport", "grpc", "session", "upload-store", "session_id", resp.GetSessionId(), "pin", pin, "tag_id", tagID)
	return &uploadSession{client: c, sessionID: resp.GetSessionId()}, nil
}

func (s *uploadSession) Put(ctx context.Context, ch swarm.Chunk) error {
	msg, err := codec.ChunkToProto(ch)
	if err != nil {
		return err
	}
	_, err = s.client.uploadClient.Put(ctx, &pb.UploadPutRequest{
		SessionId: s.sessionID,
		Chunk:     msg,
	})
	return mapErr(err)
}

func (s *uploadSession) Done(ref swarm.Address) error {
	_, err := s.client.uploadClient.Done(context.Background(), &pb.UploadDoneRequest{
		SessionId: s.sessionID,
		Reference: ref.Bytes(),
	})
	return mapErr(err)
}

func (s *uploadSession) Cleanup() error {
	_, err := s.client.uploadClient.Cleanup(context.Background(), &pb.UploadCleanupRequest{
		SessionId: s.sessionID,
	})
	return mapErr(err)
}
