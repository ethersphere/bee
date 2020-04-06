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
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/mock"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestHandshake(t *testing.T) {
	node1Addr := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	node2Addr := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59b")
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
			NetworkID: 0,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, r := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
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

		testInfo(t, *res, expectedInfo)
		if err := r.ReadMsg(&pb.Ack{}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ERROR - Syn write error ", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("write syn message: %w", testErr)
		stream := &mock.Stream{}
		stream.SetWriteErr(testErr, 0)
		res, err := handshakeService.Handshake(stream)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("ERROR - Syn read error ", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("read synack message: %w", testErr)
		stream := mock.NewStream(nil, &bytes.Buffer{})
		stream.SetReadErr(testErr, 0)
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
		expectedErr := fmt.Errorf("write ack message: %w", testErr)
		expectedInfo := Info{
			Address:   node2Addr,
			NetworkID: 0,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream1.SetWriteErr(testErr, 1)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
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

	t.Run("ERROR - networkID mismatch ", func(t *testing.T) {
		node2Info := Info{
			Address:   node2Addr,
			NetworkID: 2,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				Address:   node2Info.Address.Bytes(),
				NetworkID: node2Info.NetworkID,
				Light:     node2Info.Light,
			},
			Ack: &pb.Ack{Address: info.Address.Bytes()},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(stream1)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if err != ErrNetworkIDIncompatible {
			t.Fatalf("expected %s, got %s", ErrNetworkIDIncompatible, err)
		}
	})
}

func TestHandle(t *testing.T) {
	node1Addr := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")
	node2Addr := swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59b")
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
			NetworkID: 0,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.Syn{
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

		testInfo(t, *res, node2Info)

		_, r := protobuf.NewWriterAndReader(stream2)
		var got pb.SynAck
		if err := r.ReadMsg(&got); err != nil {
			t.Fatal(err)
		}

		testInfo(t, nodeInfo, Info{
			Address:   swarm.NewAddress(got.Syn.Address),
			NetworkID: got.Syn.NetworkID,
			Light:     got.Syn.Light,
		})
	})

	t.Run("ERROR - read error ", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("read syn message: %w", testErr)
		stream := &mock.Stream{}
		stream.SetReadErr(testErr, 0)
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
		expectedErr := fmt.Errorf("write synack message: %w", testErr)
		var buffer bytes.Buffer
		stream := mock.NewStream(&buffer, &buffer)
		stream.SetWriteErr(testErr, 1)
		w, _ := protobuf.NewWriterAndReader(stream)
		if err := w.WriteMsg(&pb.Syn{
			Address:   node1Addr.Bytes(),
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
		expectedErr := fmt.Errorf("read ack message: %w", testErr)
		node2Info := Info{
			Address:   node2Addr,
			NetworkID: 0,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)
		stream1.SetReadErr(testErr, 1)
		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.Syn{
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

	t.Run("ERROR - networkID mismatch ", func(t *testing.T) {
		node2Info := Info{
			Address:   node2Addr,
			NetworkID: 2,
			Light:     false,
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.Syn{
			Address:   node2Info.Address.Bytes(),
			NetworkID: node2Info.NetworkID,
			Light:     node2Info.Light,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(stream1)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if err != ErrNetworkIDIncompatible {
			t.Fatalf("expected %s, got %s", ErrNetworkIDIncompatible, err)
		}
	})
}

// testInfo validates if two Info instances are equal.
func testInfo(t *testing.T, got, want Info) {
	t.Helper()

	if !got.Address.Equal(want.Address) || got.NetworkID != want.NetworkID || got.Light != want.Light {
		t.Fatalf("got info %+v, want %+v", got, want)
	}
}
