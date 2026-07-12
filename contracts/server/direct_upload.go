// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/ethersphere/bee/v2/contracts"
	"github.com/ethersphere/bee/v2/contracts/codec"
	"github.com/ethersphere/bee/v2/contracts/pb"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type directUploadService struct {
	pb.UnimplementedDirectUploadServer
	cfg Config

	mu       sync.Mutex
	nextID   uint64
	sessions map[uint64]storer.PutterSession
}

func (s *directUploadService) Open(context.Context, *pb.OpenDirectUploadRequest) (*pb.OpenDirectUploadResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = make(map[uint64]storer.PutterSession)
	}
	id := s.nextID
	s.nextID++
	s.sessions[id] = s.cfg.NetStore.DirectUpload()
	if !s.cfg.DisableRPCLogging {
		s.cfg.Logger.Info(contracts.LogMarkerUpload, "transport", "grpc", "method", "DirectUpload.Open", "session_id", id)
	}
	return &pb.OpenDirectUploadResponse{SessionId: id}, nil
}

func (s *directUploadService) Put(ctx context.Context, req *pb.DirectUploadPutRequest) (*pb.PutResponse, error) {
	if err := s.put(ctx, req.GetSessionId(), req.GetChunk()); err != nil {
		return nil, err
	}
	if !s.cfg.DisableRPCLogging {
		s.cfg.Logger.Info(contracts.LogMarkerUpload, "transport", "grpc", "method", "DirectUpload.Put", "detail", rpcRequestDetail(req))
	}
	return &pb.PutResponse{}, nil
}

func (s *directUploadService) PutStream(stream grpc.ClientStreamingServer[pb.DirectUploadPutRequest, pb.PutResponse]) error {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&pb.PutResponse{})
		}
		if err != nil {
			return err
		}
		if err := s.put(stream.Context(), req.GetSessionId(), req.GetChunk()); err != nil {
			return err
		}
	}
}

func (s *directUploadService) PutBatch(ctx context.Context, req *pb.DirectUploadPutBatchRequest) (*pb.PutResponse, error) {
	if err := s.putBatch(ctx, req); err != nil {
		return nil, err
	}
	return &pb.PutResponse{}, nil
}

func (s *directUploadService) PutBatchStream(stream grpc.ClientStreamingServer[pb.DirectUploadPutBatchRequest, pb.PutResponse]) error {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&pb.PutResponse{})
		}
		if err != nil {
			return err
		}
		if err := s.putBatch(stream.Context(), req); err != nil {
			return err
		}
	}
}

func (s *directUploadService) putBatch(ctx context.Context, req *pb.DirectUploadPutBatchRequest) error {
	for _, msg := range req.GetChunks() {
		if err := s.put(ctx, req.GetSessionId(), msg); err != nil {
			return err
		}
	}
	return nil
}

func (s *directUploadService) put(ctx context.Context, sessionID uint64, msg *pb.ChunkMessage) error {
	sess, err := s.session(sessionID)
	if err != nil {
		return err
	}
	ch, err := codec.ChunkFromProto(msg)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if err := sess.Put(context.WithoutCancel(ctx), ch); err != nil {
		return mapErr(err)
	}
	return nil
}

func (s *directUploadService) Done(_ context.Context, req *pb.DirectUploadDoneRequest) (*pb.DirectUploadDoneResponse, error) {
	s.mu.Lock()
	sess, ok := s.sessions[req.GetSessionId()]
	if ok {
		delete(s.sessions, req.GetSessionId())
	}
	s.mu.Unlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "direct upload session not found")
	}
	if err := sess.Done(swarm.NewAddress(req.GetReference())); err != nil {
		return nil, mapErr(err)
	}
	return &pb.DirectUploadDoneResponse{}, nil
}

func (s *directUploadService) Cleanup(_ context.Context, req *pb.DirectUploadCleanupRequest) (*pb.DirectUploadCleanupResponse, error) {
	s.mu.Lock()
	sess, ok := s.sessions[req.GetSessionId()]
	if ok {
		delete(s.sessions, req.GetSessionId())
	}
	s.mu.Unlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "direct upload session not found")
	}
	if err := sess.Cleanup(); err != nil {
		return nil, mapErr(err)
	}
	return &pb.DirectUploadCleanupResponse{}, nil
}

func (s *directUploadService) session(id uint64) (storer.PutterSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, status.Error(codes.NotFound, "direct upload session not found")
	}
	return sess, nil
}
