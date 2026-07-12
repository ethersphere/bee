// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/v2/contracts/codec"
	"github.com/ethersphere/bee/v2/contracts/pb"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type uploadService struct {
	pb.UnimplementedUploadStoreServer
	cfg Config

	mu       sync.Mutex
	nextID   uint64
	sessions map[uint64]storer.PutterSession
}

func (s *uploadService) Open(ctx context.Context, req *pb.OpenUploadRequest) (*pb.OpenUploadResponse, error) {
	sess, err := s.cfg.UploadStore.Upload(ctx, req.GetPin(), req.GetTagId())
	if err != nil {
		return nil, mapErr(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = make(map[uint64]storer.PutterSession)
	}
	id := s.nextID
	s.nextID++
	s.sessions[id] = sess
	s.cfg.Logger.Debug("storage contract grpc upload session opened", "session_id", id, "pin", req.GetPin(), "tag_id", req.GetTagId())
	return &pb.OpenUploadResponse{SessionId: id}, nil
}

func (s *uploadService) Put(ctx context.Context, req *pb.UploadPutRequest) (*pb.PutResponse, error) {
	sess, err := s.uploadSession(req.GetSessionId())
	if err != nil {
		return nil, err
	}
	ch, err := codec.ChunkFromProto(req.GetChunk())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := sess.Put(context.WithoutCancel(ctx), ch); err != nil {
		return nil, mapErr(err)
	}
	return &pb.PutResponse{}, nil
}

func (s *uploadService) Done(_ context.Context, req *pb.UploadDoneRequest) (*pb.UploadDoneResponse, error) {
	s.mu.Lock()
	sess, ok := s.sessions[req.GetSessionId()]
	if ok {
		delete(s.sessions, req.GetSessionId())
	}
	s.mu.Unlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "upload session not found")
	}
	if err := sess.Done(swarm.NewAddress(req.GetReference())); err != nil {
		return nil, mapErr(err)
	}
	return &pb.UploadDoneResponse{}, nil
}

func (s *uploadService) Cleanup(_ context.Context, req *pb.UploadCleanupRequest) (*pb.UploadCleanupResponse, error) {
	s.mu.Lock()
	sess, ok := s.sessions[req.GetSessionId()]
	if ok {
		delete(s.sessions, req.GetSessionId())
	}
	s.mu.Unlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "upload session not found")
	}
	if err := sess.Cleanup(); err != nil {
		return nil, mapErr(err)
	}
	return &pb.UploadCleanupResponse{}, nil
}

func (s *uploadService) uploadSession(id uint64) (storer.PutterSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, status.Error(codes.NotFound, "upload session not found")
	}
	return sess, nil
}
