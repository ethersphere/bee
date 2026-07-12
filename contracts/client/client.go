// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/ethersphere/bee/v2/contracts"
	"github.com/ethersphere/bee/v2/contracts/codec"
	"github.com/ethersphere/bee/v2/contracts/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// DefaultAddr is the default bee storage contract gRPC listen address.
const DefaultAddr = "127.0.0.1:17337"

// Client exposes storage interfaces backed by the bee gRPC storage contract.
type Client struct {
	conn               *grpc.ClientConn
	chunkClient        pb.ChunkStoreClient
	directUploadClient pb.DirectUploadClient
	uploadClient       pb.UploadStoreClient
	pinClient          pb.PinStoreClient
	stateClient        pb.StateStoreClient
	logger             log.Logger
}

func Dial(addr string, logger log.Logger) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("contracts client dial: %w", err)
	}
	client := NewFromConn(conn, logger)
	client.logger.Info(contracts.LogPrefix+" client connected", "addr", addr)
	return client, nil
}

// NewFromConn creates a storage contract client using conn.
func NewFromConn(conn *grpc.ClientConn, logger log.Logger) *Client {
	if logger == nil {
		logger = log.Noop
	}
	return &Client{
		conn:               conn,
		chunkClient:        pb.NewChunkStoreClient(conn),
		directUploadClient: pb.NewDirectUploadClient(conn),
		uploadClient:       pb.NewUploadStoreClient(conn),
		pinClient:          pb.NewPinStoreClient(conn),
		stateClient:        pb.NewStateStoreClient(conn),
		logger:             logger,
	}
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) Download(cache bool) storage.Getter {
	return &getter{client: c, cache: cache}
}

func (c *Client) Cache() storage.Putter {
	return &putter{client: c}
}

func (c *Client) ChunkStore() storage.ReadOnlyChunkStore {
	return &readOnlyChunkStore{client: c}
}

func (c *Client) StateStore() storage.StateStorer {
	return &stateStore{client: c}
}

type getter struct {
	client *Client
	cache  bool
}

func (g *getter) Get(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
	g.client.logger.Info(
		contracts.LogMarkerDownload,
		"transport", "grpc",
		"method", "ChunkStore.Get",
		"address", address,
		"cache_on_miss", g.cache,
	)
	resp, err := g.client.chunkClient.Get(ctx, &pb.GetRequest{
		Address:     address.Bytes(),
		CacheOnMiss: g.cache,
		LocalOnly:   false,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return codec.ChunkFromProto(resp.GetChunk())
}

type putter struct {
	client *Client
}

func (p *putter) Put(ctx context.Context, ch swarm.Chunk) error {
	p.client.logger.Debug(
		contracts.LogPrefix+" client rpc",
		"method", "ChunkStore.Put",
		"address", ch.Address(),
		"data_len", len(ch.Data()),
	)
	msg, err := codec.ChunkToProto(ch)
	if err != nil {
		return err
	}
	_, err = p.client.chunkClient.Put(ctx, &pb.PutRequest{Chunk: msg})
	return mapErr(err)
}

type readOnlyChunkStore struct {
	client *Client
}

func (s *readOnlyChunkStore) Get(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
	s.client.logger.Debug(
		contracts.LogPrefix+" client rpc",
		"method", "ChunkStore.Get",
		"address", address,
		"local_only", true,
	)
	resp, err := s.client.chunkClient.Get(ctx, &pb.GetRequest{
		Address:   address.Bytes(),
		LocalOnly: true,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return codec.ChunkFromProto(resp.GetChunk())
}

func (s *readOnlyChunkStore) Has(ctx context.Context, address swarm.Address) (bool, error) {
	s.client.logger.Debug(
		contracts.LogPrefix+" client rpc",
		"method", "ChunkStore.Has",
		"address", address,
	)
	resp, err := s.client.chunkClient.Has(ctx, &pb.HasRequest{Address: address.Bytes()})
	if err != nil {
		return false, mapErr(err)
	}
	return resp.GetHas(), nil
}

type stateStore struct {
	client *Client
}

func (s *stateStore) Get(key string, obj any) error {
	resp, err := s.client.stateClient.Get(context.Background(), &pb.StateGetRequest{Key: key})
	if err != nil {
		return mapErr(err)
	}
	switch m := obj.(type) {
	case storage.Unmarshaler:
		return m.Unmarshal(resp.GetValue())
	default:
		return json.Unmarshal(resp.GetValue(), obj)
	}
}

func (s *stateStore) Put(key string, obj any) error {
	var (
		data []byte
		err  error
	)
	switch m := obj.(type) {
	case storage.Marshaler:
		data, err = m.Marshal()
	default:
		data, err = json.Marshal(obj)
	}
	if err != nil {
		return err
	}
	_, err = s.client.stateClient.Put(context.Background(), &pb.StatePutRequest{
		Key:   key,
		Value: data,
	})
	return mapErr(err)
}

func (s *stateStore) Delete(key string) error {
	_, err := s.client.stateClient.Delete(context.Background(), &pb.StateDeleteRequest{Key: key})
	return mapErr(err)
}

func (s *stateStore) Iterate(prefix string, iterFunc storage.StateIterFunc) error {
	stream, err := s.client.stateClient.Iterate(context.Background(), &pb.StateIterateRequest{Prefix: prefix})
	if err != nil {
		return mapErr(err)
	}
	for {
		entry, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return mapErr(err)
		}
		stop, err := iterFunc([]byte(entry.GetKey()), entry.GetValue())
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
}

func (s *stateStore) Close() error {
	_, err := s.client.stateClient.Close(context.Background(), &pb.StateCloseRequest{})
	return mapErr(err)
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return storage.ErrNotFound
	}
	return err
}
