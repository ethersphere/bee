// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package handshake

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
)

type StreamMock struct {
	readBuffer  *bytes.Buffer
	writeBuffer *bytes.Buffer
	readError   error
	writeError  error
}

func (s *StreamMock) Read(p []byte) (n int, err error) {
	if s.readError != nil {
		return 0, s.readError
	}

	return s.readBuffer.Read(p)
}

func (s *StreamMock) Write(p []byte) (n int, err error) {
	if s.writeError != nil {
		return 0, s.writeError
	}

	return s.writeBuffer.Write(p)
}

func (s *StreamMock) Close() error {
	return nil
}

func TestHandshake(t *testing.T) {
	logger := logging.New(ioutil.Discard)
	info := Info{
		Address:   "node1",
		NetworkID: 0,
		Light:     false,
	}
	handshakeService := New(info.Address, info.NetworkID, logger)

	t.Run("OK", func(t *testing.T) {
		expectedInfo := Info{
			Address:   "node2",
			NetworkID: 1,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := &StreamMock{readBuffer: &buffer1, writeBuffer: &buffer2}
		stream2 := &StreamMock{readBuffer: &buffer2, writeBuffer: &buffer1}

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.ShakeHand{
			Address:   expectedInfo.Address,
			NetworkID: expectedInfo.NetworkID,
			Light:     expectedInfo.Light,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(stream1)
		if err != nil {
			t.Fatal(err)
		}

		if *res != expectedInfo {
			t.Fatalf("got %+v, expected %+v", res, info)
		}
	})

	t.Run("ERROR - write error ", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("handshake write message: %w", testErr)
		stream := &StreamMock{writeError: testErr}
		res, err := handshakeService.Handshake(stream)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("ERROR - read error ", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("handshake read message: %w", testErr)
		stream := &StreamMock{writeBuffer: &bytes.Buffer{}, readError: testErr}
		res, err := handshakeService.Handshake(stream)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})
}

func TestHandle(t *testing.T) {
	nodeInfo := Info{
		Address:   "node1",
		NetworkID: 0,
		Light:     false,
	}

	logger := logging.New(ioutil.Discard)
	handshakeService := New(nodeInfo.Address, nodeInfo.NetworkID, logger)

	t.Run("OK", func(t *testing.T) {
		node2Info := Info{
			Address:   "node2",
			NetworkID: 1,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := &StreamMock{readBuffer: &buffer1, writeBuffer: &buffer2}
		stream2 := &StreamMock{readBuffer: &buffer2, writeBuffer: &buffer1}

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.ShakeHand{
			Address:   node2Info.Address,
			NetworkID: node2Info.NetworkID,
			Light:     node2Info.Light,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(stream1)
		if err != nil {
			t.Fatal(err)
		}

		if *res != node2Info {
			t.Fatalf("got %+v, expected %+v", res, node2Info)
		}

		_, r := protobuf.NewWriterAndReader(stream2)
		var got pb.ShakeHand
		if err := r.ReadMsg(&got); err != nil {
			t.Fatal(err)
		}

		if nodeInfo != Info(got) {
			t.Fatalf("got %+v, expected %+v", got, node2Info)
		}
	})

	t.Run("ERROR - read error ", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("handshake handler read message: %w", testErr)
		stream := &StreamMock{readError: testErr}
		res, err := handshakeService.Handle(stream)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handle returned non-nil res")
		}
	})

	t.Run("ERROR - write error ", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("handshake handler write message: %w", testErr)
		var buffer bytes.Buffer
		stream := &StreamMock{readBuffer: &buffer, writeBuffer: &buffer}

		w, _ := protobuf.NewWriterAndReader(stream)
		if err := w.WriteMsg(&pb.ShakeHand{
			Address:   "node1",
			NetworkID: 0,
			Light:     false,
		}); err != nil {
			t.Fatal(err)
		}

		stream.writeError = testErr
		res, err := handshakeService.Handle(stream)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})
}
