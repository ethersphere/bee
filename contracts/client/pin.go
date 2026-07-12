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

type pinSession struct {
	client    *Client
	sessionID uint64
}

func (c *Client) NewCollection(ctx context.Context) (storer.PutterSession, error) {
	resp, err := c.pinClient.Open(ctx, &pb.OpenPinRequest{})
	if err != nil {
		return nil, fmt.Errorf("pin collection open: %w", mapErr(err))
	}
	c.logger.Debug(contracts.LogPrefix+" client pin collection session opened", "session_id", resp.GetSessionId())
	return &pinSession{client: c, sessionID: resp.GetSessionId()}, nil
}

func (s *pinSession) Put(ctx context.Context, ch swarm.Chunk) error {
	msg, err := codec.ChunkToProto(ch)
	if err != nil {
		return err
	}
	_, err = s.client.pinClient.Put(ctx, &pb.PinPutRequest{
		SessionId: s.sessionID,
		Chunk:     msg,
	})
	return mapErr(err)
}

func (s *pinSession) Done(ref swarm.Address) error {
	_, err := s.client.pinClient.Done(context.Background(), &pb.PinDoneRequest{
		SessionId: s.sessionID,
		Reference: ref.Bytes(),
	})
	return mapErr(err)
}

func (s *pinSession) Cleanup() error {
	_, err := s.client.pinClient.Cleanup(context.Background(), &pb.PinCleanupRequest{
		SessionId: s.sessionID,
	})
	return mapErr(err)
}
