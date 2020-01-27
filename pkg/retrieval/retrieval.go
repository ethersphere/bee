// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate sh -c "protoc -I . -I \"$(go list -f '{{ .Dir }}' -m github.com/gogo/protobuf)/protobuf\" --gogofaster_out=. retrieval.proto"

package retrieval

import (
	"context"
	"fmt"
	"io"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/storage"
)

const (
	protocolName  = "retrieval"
	streamName    = "retrieval"
	streamVersion = "1.0.0"
)

type Service struct {
	streamer p2p.Streamer
	storer   storage.Storer
	logger   Logger
}

type Options struct {
	Streamer p2p.Streamer
	Storer   storage.Storer
	Logger   Logger
}

type Logger interface {
	Debugf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

type Storer interface {
}

func New(o Options) *Service {
	return &Service{
		streamer: o.Streamer,
		storer:   o.Storer,
		logger:   o.Logger,
	}
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name: protocolName,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Version: streamVersion,
				Handler: s.Handler,
			},
		},
	}
}

func (s *Service) RetrieveChunk(ctx context.Context, peerID string, addr []byte) (data []byte, err error) {
	stream, err := s.streamer.NewStream(ctx, peerID, protocolName, streamName, streamVersion)
	if err != nil {
		return nil, fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	w, r := protobuf.NewWriterAndReader(stream)

	if err := w.WriteMsg(&Request{
		Addr: addr,
	}); err != nil {
		return nil, fmt.Errorf("stream write: %w", err)
	}

	var d Delivery
	if err := r.ReadMsg(&d); err != nil {
		if err == io.EOF {
			return nil, err
		}
		return nil, err
	}

	return d.Data, nil
}

func (s *Service) Handler(p p2p.Peer) {
	w, r := protobuf.NewWriterAndReader(p.Stream)
	defer p.Stream.Close()

	var req Request
	if err := r.ReadMsg(&req); err != nil {
		if err == io.EOF {
			return
		}
		// log this
		return
	}

	data, err := s.storer.Get(context.TODO(), req.Addr)
	if err != nil {
		return
	}
	if err := w.WriteMsg(&Delivery{
		Data: data,
	}); err != nil {
		return
	}
}

// should be replaced by protobuf compilation
//type Request struct {
//Addr []byte
//}

//type Delivery struct {
//Data []byte
//}

//func (r *Request) Reset() {

//}
//func (r *Request) String() string {
//return fmt.Sprintf("%x", r.Addr)
//}
//func (r *Request) ProtoMessage() {

//}

//func (d *Delivery) Reset() {

//}
//func (d *Delivery) String() string {
//return fmt.Sprintf("%x", d.Data)
//}
//func (d *Delivery) ProtoMessage() {

//}
