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

type pinService struct {
	pb.UnimplementedPinStoreServer
	cfg Config

	mu       sync.Mutex
	nextID   uint64
	sessions map[uint64]storer.PutterSession
}

func (s *pinService) Open(ctx context.Context, _ *pb.OpenPinRequest) (*pb.OpenPinResponse, error) {
	sess, err := s.cfg.PinStore.NewCollection(ctx)
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
	s.cfg.Logger.Debug("storage contract grpc pin collection session opened", "session_id", id)
	return &pb.OpenPinResponse{SessionId: id}, nil
}

func (s *pinService) Put(ctx context.Context, req *pb.PinPutRequest) (*pb.PutResponse, error) {
	sess, err := s.pinSession(req.GetSessionId())
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

func (s *pinService) Done(_ context.Context, req *pb.PinDoneRequest) (*pb.PinDoneResponse, error) {
	s.mu.Lock()
	sess, ok := s.sessions[req.GetSessionId()]
	if ok {
		delete(s.sessions, req.GetSessionId())
	}
	s.mu.Unlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "pin collection session not found")
	}
	if err := sess.Done(swarm.NewAddress(req.GetReference())); err != nil {
		return nil, mapErr(err)
	}
	return &pb.PinDoneResponse{}, nil
}

func (s *pinService) Cleanup(_ context.Context, req *pb.PinCleanupRequest) (*pb.PinCleanupResponse, error) {
	s.mu.Lock()
	sess, ok := s.sessions[req.GetSessionId()]
	if ok {
		delete(s.sessions, req.GetSessionId())
	}
	s.mu.Unlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "pin collection session not found")
	}
	if err := sess.Cleanup(); err != nil {
		return nil, mapErr(err)
	}
	return &pb.PinCleanupResponse{}, nil
}

func (s *pinService) pinSession(id uint64) (storer.PutterSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, status.Error(codes.NotFound, "pin collection session not found")
	}
	return sess, nil
}
