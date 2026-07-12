// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/ethersphere/bee/v2/contracts"
	"github.com/ethersphere/bee/v2/contracts/codec"
	"github.com/ethersphere/bee/v2/contracts/pb"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DefaultListenAddr is the default listen address for the storage contract gRPC server.
// Separate from the HTTP API (default 127.0.0.1:1633).
const DefaultListenAddr = "127.0.0.1:17337"

type Config struct {
	ListenAddr  string
	NetStore    storer.NetStore
	CacheStore  storer.CacheStore
	LocalStore  storer.LocalStore
	UploadStore storer.UploadStore
	PinStore    storer.PinStore
	StateStore  storage.StateStorer
	Logger      log.Logger
	// DisableRPCLogging disables per-RPC logging for allocation benchmarks.
	DisableRPCLogging bool
}

type Server struct {
	cfg      Config
	grpc     *grpc.Server
	listener net.Listener
}

func New(cfg Config) *Server {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = DefaultListenAddr
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Noop
	}
	return &Server{cfg: cfg}
}

func (s *Server) ListenAndServe() error {
	lis, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("contracts server listen: %w", err)
	}

	return s.Serve(lis)
}

// Serve starts the storage contract gRPC server on lis.
func (s *Server) Serve(lis net.Listener) error {
	s.listener = lis
	var opts []grpc.ServerOption
	if !s.cfg.DisableRPCLogging {
		opts = append(opts, grpc.UnaryInterceptor(unaryLogInterceptor(s.cfg.Logger)))
	}
	s.grpc = grpc.NewServer(opts...)
	pb.RegisterChunkStoreServer(s.grpc, &chunkService{cfg: s.cfg})
	pb.RegisterDirectUploadServer(s.grpc, &directUploadService{cfg: s.cfg})
	pb.RegisterUploadStoreServer(s.grpc, &uploadService{cfg: s.cfg})
	pb.RegisterPinStoreServer(s.grpc, &pinService{cfg: s.cfg})
	pb.RegisterStateStoreServer(s.grpc, &stateService{cfg: s.cfg})
	s.cfg.Logger.Info(contracts.LogPrefix+" server started", "addr", lis.Addr())
	return s.grpc.Serve(lis)
}

func (s *Server) Close() error {
	if s.grpc != nil {
		s.grpc.GracefulStop()
	}
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

type chunkService struct {
	pb.UnimplementedChunkStoreServer
	cfg Config
}

func (s *chunkService) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	s.cfg.Logger.Info(contracts.LogMarkerDownload, "transport", "grpc", "method", "ChunkStore.Get", "detail", rpcRequestDetail(req))
	addr := swarm.NewAddress(req.GetAddress())
	var (
		ch  swarm.Chunk
		err error
	)
	if req.GetLocalOnly() {
		ch, err = s.cfg.LocalStore.ChunkStore().Get(ctx, addr)
	} else {
		ch, err = s.cfg.NetStore.Download(req.GetCacheOnMiss()).Get(ctx, addr)
	}
	if err != nil {
		return nil, mapErr(err)
	}
	msg, err := codec.ChunkToProto(ch)
	if err != nil {
		return nil, err
	}
	return &pb.GetResponse{Chunk: msg}, nil
}

func (s *chunkService) Put(ctx context.Context, req *pb.PutRequest) (*pb.PutResponse, error) {
	ch, err := codec.ChunkFromProto(req.GetChunk())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.cfg.CacheStore.Cache().Put(ctx, ch); err != nil {
		return nil, mapErr(err)
	}
	return &pb.PutResponse{}, nil
}

func (s *chunkService) Delete(context.Context, *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	return nil, status.Error(codes.Unimplemented, "chunk Delete not implemented in MVP")
}

func (s *chunkService) Has(ctx context.Context, req *pb.HasRequest) (*pb.HasResponse, error) {
	has, err := s.cfg.LocalStore.ChunkStore().Has(ctx, swarm.NewAddress(req.GetAddress()))
	if err != nil {
		return nil, mapErr(err)
	}
	return &pb.HasResponse{Has: has}, nil
}

func (s *chunkService) Replace(context.Context, *pb.ReplaceRequest) (*pb.ReplaceResponse, error) {
	return nil, status.Error(codes.Unimplemented, "chunk Replace not implemented in MVP")
}

func (s *chunkService) Iterate(*pb.IterateRequest, grpc.ServerStreamingServer[pb.ChunkMessage]) error {
	return status.Error(codes.Unimplemented, "chunk Iterate not implemented in MVP")
}

type stateService struct {
	pb.UnimplementedStateStoreServer
	cfg Config
}

func (s *stateService) Get(ctx context.Context, req *pb.StateGetRequest) (*pb.StateGetResponse, error) {
	var raw json.RawMessage
	if err := s.cfg.StateStore.Get(req.GetKey(), &raw); err != nil {
		return nil, mapErr(err)
	}
	return &pb.StateGetResponse{Value: raw}, nil
}

func (s *stateService) Put(ctx context.Context, req *pb.StatePutRequest) (*pb.StatePutResponse, error) {
	if err := s.cfg.StateStore.Put(req.GetKey(), json.RawMessage(req.GetValue())); err != nil {
		return nil, mapErr(err)
	}
	return &pb.StatePutResponse{}, nil
}

func (s *stateService) Delete(ctx context.Context, req *pb.StateDeleteRequest) (*pb.StateDeleteResponse, error) {
	if err := s.cfg.StateStore.Delete(req.GetKey()); err != nil {
		return nil, mapErr(err)
	}
	return &pb.StateDeleteResponse{}, nil
}

func (s *stateService) Iterate(req *pb.StateIterateRequest, stream grpc.ServerStreamingServer[pb.StateEntry]) error {
	return s.cfg.StateStore.Iterate(req.GetPrefix(), func(key, val []byte) (bool, error) {
		if err := stream.Send(&pb.StateEntry{Key: string(key), Value: val}); err != nil {
			return true, err
		}
		return false, nil
	})
}

func (s *stateService) Close(context.Context, *pb.StateCloseRequest) (*pb.StateCloseResponse, error) {
	if err := s.cfg.StateStore.Close(); err != nil {
		return nil, mapErr(err)
	}
	return &pb.StateCloseResponse{}, nil
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, storage.ErrNotFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	return err
}

func unaryLogInterceptor(logger log.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		logger.Debug(
			contracts.LogPrefix+" server rpc",
			"method", info.FullMethod,
			"detail", rpcRequestDetail(req),
		)
		resp, err := handler(ctx, req)
		if err != nil {
			logger.Debug(
				contracts.LogPrefix+" server rpc error",
				"method", info.FullMethod,
				"error", err,
			)
		}
		return resp, err
	}
}

func rpcRequestDetail(req any) string {
	switch r := req.(type) {
	case *pb.GetRequest:
		if r.GetLocalOnly() {
			return fmt.Sprintf("Get address=%s local_only=true", swarm.NewAddress(r.GetAddress()).String())
		}
		return fmt.Sprintf("Get address=%s cache_on_miss=%t", swarm.NewAddress(r.GetAddress()).String(), r.GetCacheOnMiss())
	case *pb.PutRequest:
		if ch := r.GetChunk(); ch != nil {
			return fmt.Sprintf("Put address=%s data_len=%d stamp_len=%d", swarm.NewAddress(ch.GetAddress()).String(), len(ch.GetData()), len(ch.GetStamp()))
		}
		return "Put"
	case *pb.HasRequest:
		return fmt.Sprintf("Has address=%s", swarm.NewAddress(r.GetAddress()).String())
	case *pb.DeleteRequest:
		return fmt.Sprintf("Delete address=%s", swarm.NewAddress(r.GetAddress()).String())
	case *pb.DirectUploadPutRequest:
		if ch := r.GetChunk(); ch != nil {
			return fmt.Sprintf("DirectUpload.Put session_id=%d address=%s data_len=%d", r.GetSessionId(), swarm.NewAddress(ch.GetAddress()).String(), len(ch.GetData()))
		}
		return fmt.Sprintf("DirectUpload.Put session_id=%d", r.GetSessionId())
	case *pb.DirectUploadDoneRequest:
		return fmt.Sprintf("DirectUpload.Done session_id=%d reference=%s", r.GetSessionId(), swarm.NewAddress(r.GetReference()).String())
	case *pb.DirectUploadCleanupRequest:
		return fmt.Sprintf("DirectUpload.Cleanup session_id=%d", r.GetSessionId())
	case *pb.OpenDirectUploadRequest:
		return "DirectUpload.Open"
	case *pb.OpenUploadRequest:
		return fmt.Sprintf("UploadStore.Open pin=%t tag_id=%d", r.GetPin(), r.GetTagId())
	case *pb.UploadPutRequest:
		if ch := r.GetChunk(); ch != nil {
			return fmt.Sprintf("UploadStore.Put session_id=%d address=%s data_len=%d", r.GetSessionId(), swarm.NewAddress(ch.GetAddress()).String(), len(ch.GetData()))
		}
		return fmt.Sprintf("UploadStore.Put session_id=%d", r.GetSessionId())
	case *pb.UploadDoneRequest:
		return fmt.Sprintf("UploadStore.Done session_id=%d reference=%s", r.GetSessionId(), swarm.NewAddress(r.GetReference()).String())
	case *pb.UploadCleanupRequest:
		return fmt.Sprintf("UploadStore.Cleanup session_id=%d", r.GetSessionId())
	case *pb.OpenPinRequest:
		return "PinStore.Open"
	case *pb.PinPutRequest:
		if ch := r.GetChunk(); ch != nil {
			return fmt.Sprintf("PinStore.Put session_id=%d address=%s data_len=%d", r.GetSessionId(), swarm.NewAddress(ch.GetAddress()).String(), len(ch.GetData()))
		}
		return fmt.Sprintf("PinStore.Put session_id=%d", r.GetSessionId())
	case *pb.PinDoneRequest:
		return fmt.Sprintf("PinStore.Done session_id=%d reference=%s", r.GetSessionId(), swarm.NewAddress(r.GetReference()).String())
	case *pb.PinCleanupRequest:
		return fmt.Sprintf("PinStore.Cleanup session_id=%d", r.GetSessionId())
	case *pb.StateGetRequest:
		return fmt.Sprintf("StateGet key=%q", r.GetKey())
	case *pb.StatePutRequest:
		return fmt.Sprintf("StatePut key=%q value_len=%d", r.GetKey(), len(r.GetValue()))
	case *pb.StateDeleteRequest:
		return fmt.Sprintf("StateDelete key=%q", r.GetKey())
	default:
		return fmt.Sprintf("%T", req)
	}
}
