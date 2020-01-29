// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package handshake

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/ethersphere/bee/pkg/swarm"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
)

type StreamMock struct {
	readBuffer        *bytes.Buffer
	writeBuffer       *bytes.Buffer
	writeCounter      int
	readCounter       int
	readError         error
	writeError        error
	readErrCheckmark  int
	writeErrCheckmark int
}

func (s *StreamMock) setReadErr(err error, checkmark int) {
	s.readError = err
	s.readErrCheckmark = checkmark

}

func (s *StreamMock) setWriteErr(err error, checkmark int) {
	s.writeError = err
	s.writeErrCheckmark = checkmark

}

func (s *StreamMock) Read(p []byte) (n int, err error) {
	if s.readError != nil && s.readErrCheckmark <= s.readCounter {
		return 0, s.readError
	}

	s.readCounter++
	return s.readBuffer.Read(p)
}

func (s *StreamMock) Write(p []byte) (n int, err error) {
	if s.writeError != nil && s.writeErrCheckmark <= s.writeCounter {
		return 0, s.writeError
	}

	s.writeCounter++
	return s.writeBuffer.Write(p)
}

func (s *StreamMock) Close() error {
	return nil
}

func TestHandshake(t *testing.T) {
	node1Addr, _ := swarm.ParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	node2Addr, _ := swarm.ParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59b")
	logger := logging.New(ioutil.Discard, 0)
	info := Info{
		Address:   node1Addr,
		NetworkID: 0,
		Light:     false,
	}
	handshakeService := New(info.Address, info.NetworkID, logger)

	t.Run("OK", func(t *testing.T) {
		expectedInfo := Info{
			Address:   node2Addr,
			NetworkID: 1,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := &StreamMock{readBuffer: &buffer1, writeBuffer: &buffer2}
		stream2 := &StreamMock{readBuffer: &buffer2, writeBuffer: &buffer1}

		w, r := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.ShakeHandAck{
			ShakeHand: &pb.ShakeHand{
				Address:   expectedInfo.Address.Bytes(),
				NetworkID: expectedInfo.NetworkID,
				Light:     expectedInfo.Light,
			},
			Ack: &pb.Ack{Address: info.Address.Bytes()},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(stream1)
		if err != nil {
			t.Fatal(err)
		}

		if !res.Equal(expectedInfo) {
			t.Fatalf("got %+v, expected %+v", res, info)
		}

		if err := r.ReadMsg(&pb.Ack{}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ERROR - shakehand write error ", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("write message: %w", testErr)
		stream := &StreamMock{}
		stream.setWriteErr(testErr, 0)
		res, err := handshakeService.Handshake(stream)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("ERROR - shakehand read error ", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("read message: %w", testErr)
		stream := &StreamMock{writeBuffer: &bytes.Buffer{}}
		stream.setReadErr(testErr, 0)
		res, err := handshakeService.Handshake(stream)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("ERROR - ack write error ", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("ack: write message: %w", testErr)
		expectedInfo := Info{
			Address:   node2Addr,
			NetworkID: 1,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := &StreamMock{readBuffer: &buffer1, writeBuffer: &buffer2}
		stream1.setWriteErr(testErr, 1)
		stream2 := &StreamMock{readBuffer: &buffer2, writeBuffer: &buffer1}

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.ShakeHandAck{
			ShakeHand: &pb.ShakeHand{
				Address:   expectedInfo.Address.Bytes(),
				NetworkID: expectedInfo.NetworkID,
				Light:     expectedInfo.Light,
			},
			Ack: &pb.Ack{Address: info.Address.Bytes()},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(stream1)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})
}

func TestHandle(t *testing.T) {
	node1Addr, _ := swarm.ParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	node2Addr, _ := swarm.ParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59b")
	nodeInfo := Info{
		Address:   node1Addr,
		NetworkID: 0,
		Light:     false,
	}

	logger := logging.New(ioutil.Discard, 0)
	handshakeService := New(nodeInfo.Address, nodeInfo.NetworkID, logger)

	t.Run("OK", func(t *testing.T) {
		node2Info := Info{
			Address:   node2Addr,
			NetworkID: 1,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := &StreamMock{readBuffer: &buffer1, writeBuffer: &buffer2}
		stream2 := &StreamMock{readBuffer: &buffer2, writeBuffer: &buffer1}

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.ShakeHand{
			Address:   node2Info.Address.Bytes(),
			NetworkID: node2Info.NetworkID,
			Light:     node2Info.Light,
		}); err != nil {
			t.Fatal(err)
		}

		if err := w.WriteMsg(&pb.Ack{Address: node2Info.Address.Bytes()}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(stream1)
		if err != nil {
			t.Fatal(err)
		}

		if !res.Equal(node2Info) {
			t.Fatalf("got %+v, expected %+v", res, node2Info)
		}

		_, r := protobuf.NewWriterAndReader(stream2)
		var got pb.ShakeHandAck
		if err := r.ReadMsg(&got); err != nil {
			t.Fatal(err)
		}

		if !nodeInfo.Equal(Info{
			Address:   swarm.NewAddress(got.ShakeHand.Address),
			NetworkID: got.ShakeHand.NetworkID,
			Light:     got.ShakeHand.Light,
		}) {
			t.Fatalf("got %+v, expected %+v", got, node2Info)
		}
	})

	t.Run("ERROR - read error ", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("read message: %w", testErr)
		stream := &StreamMock{}
		stream.setReadErr(testErr, 0)
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
		expectedErr := fmt.Errorf("write message: %w", testErr)
		var buffer bytes.Buffer
		stream := &StreamMock{readBuffer: &buffer, writeBuffer: &buffer}
		stream.setWriteErr(testErr, 1)
		w, _ := protobuf.NewWriterAndReader(stream)
		if err := w.WriteMsg(&pb.ShakeHand{
			Address:   node1Addr,
			NetworkID: 0,
			Light:     false,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(stream)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("ERROR - ack read error ", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("ack: read message: %w", testErr)
		node2Info := Info{
			Address:   node2Addr,
			NetworkID: 1,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := &StreamMock{readBuffer: &buffer1, writeBuffer: &buffer2}
		stream2 := &StreamMock{readBuffer: &buffer2, writeBuffer: &buffer1}
		stream1.setReadErr(testErr, 1)
		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.ShakeHand{
			Address:   node2Info.Address.Bytes(),
			NetworkID: node2Info.NetworkID,
			Light:     node2Info.Light,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(stream1)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})
}
