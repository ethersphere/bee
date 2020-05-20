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

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/mock"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/libp2p/go-libp2p-core/peer"

	ma "github.com/multiformats/go-multiaddr"
)

func TestHandshake(t *testing.T) {
	node1Underlay := []byte("underlay1")
	node2Underlay := []byte("underlay2")
	logger := logging.New(ioutil.Discard, 0)

	privateKey1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	privateKey2, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	node1Overlay := crypto.NewOverlayAddress(privateKey1.PublicKey, 0)
	node2Overlay := crypto.NewOverlayAddress(privateKey2.PublicKey, 0)
	signer1 := crypto.NewDefaultSigner(privateKey1)
	signer2 := crypto.NewDefaultSigner(privateKey2)

	signature1, err := signer1.Sign([]byte("underlay10"))
	if err != nil {
		t.Fatal(err)
	}

	signature2, err := signer2.Sign([]byte("underlay20"))
	if err != nil {
		t.Fatal(err)
	}

	node1Info := Info{
		Overlay:   node1Overlay,
		Underlay:  node1Underlay,
		NetworkID: 0,
		Light:     false,
	}

	node1BzzAddress := &pb.BzzAddress{
		Overlay:   node1Info.Overlay.Bytes(),
		Underlay:  node1Info.Underlay,
		Signature: signature1,
	}

	node2Info := Info{
		Overlay:   node2Overlay,
		Underlay:  node2Underlay,
		NetworkID: 0,
		Light:     false,
	}

	node2BzzAddress := &pb.BzzAddress{
		Overlay:   node2Info.Overlay.Bytes(),
		Underlay:  node2Info.Underlay,
		Signature: signature2,
	}

	handshakeService, err := New(node1Info.Overlay, string(node1Info.Underlay), signer1, 0, logger)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("OK", func(t *testing.T) {
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, r := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				BzzAddress: node2BzzAddress,
				NetworkID:  node2Info.NetworkID,
				Light:      node2Info.Light,
			},
			Ack: &pb.Ack{BzzAddress: node1BzzAddress},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(stream1)
		if err != nil {
			t.Fatal(err)
		}

		testInfo(t, *res, node2Info)
		if err := r.ReadMsg(&pb.Ack{}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ERROR - Syn write error", func(t *testing.T) {
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

	t.Run("ERROR - Syn read error", func(t *testing.T) {
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

	t.Run("ERROR - ack write error", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("write ack message: %w", testErr)
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream1.SetWriteErr(testErr, 1)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				BzzAddress: node2BzzAddress,
				NetworkID:  node2Info.NetworkID,
				Light:      node2Info.Light,
			},
			Ack: &pb.Ack{BzzAddress: node1BzzAddress},
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

	t.Run("ERROR - networkID mismatch", func(t *testing.T) {
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				BzzAddress: node2BzzAddress,
				NetworkID:  5,
				Light:      node2Info.Light,
			},
			Ack: &pb.Ack{BzzAddress: node1BzzAddress},
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

	t.Run("ERROR - invalid ack", func(t *testing.T) {
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				BzzAddress: node2BzzAddress,
				NetworkID:  node2Info.NetworkID,
				Light:      node2Info.Light,
			},
			Ack: &pb.Ack{BzzAddress: node2BzzAddress},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(stream1)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if err != ErrInvalidAck {
			t.Fatalf("expected %s, got %s", ErrInvalidAck, err)
		}
	})

	t.Run("ERROR - invalid signature", func(t *testing.T) {
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				BzzAddress: &pb.BzzAddress{
					Underlay:  node2BzzAddress.Underlay,
					Signature: []byte("wrong signature"),
					Overlay:   node2BzzAddress.Overlay,
				},
				NetworkID: node2Info.NetworkID,
				Light:     node2Info.Light,
			},
			Ack: &pb.Ack{BzzAddress: node1BzzAddress},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(stream1)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if err != ErrInvalidSignature {
			t.Fatalf("expected %s, got %s", ErrInvalidSignature, err)
		}
	})
}

func TestHandle(t *testing.T) {
	privateKey1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	privateKey2, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	node1Overlay := crypto.NewOverlayAddress(privateKey1.PublicKey, 0)
	node2Overlay := crypto.NewOverlayAddress(privateKey2.PublicKey, 0)
	node2ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7070/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS")
	if err != nil {
		t.Fatal(err)
	}

	node2AddrInfo, err := peer.AddrInfoFromP2pAddr(node2ma)
	if err != nil {
		t.Fatal(err)
	}

	node1Underlay := []byte("underlay1")
	node2Underlay := []byte("16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS")
	node1Info := Info{
		Overlay:   node1Overlay,
		Underlay:  node1Underlay,
		NetworkID: 0,
		Light:     false,
	}

	signer1 := crypto.NewDefaultSigner(privateKey1)
	signer2 := crypto.NewDefaultSigner(privateKey2)
	signature1, err := signer1.Sign([]byte("underlay10"))
	if err != nil {
		t.Fatal(err)
	}

	signature2, err := signer2.Sign([]byte("16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS0"))
	if err != nil {
		t.Fatal(err)
	}

	node2Info := Info{
		Overlay:   node2Overlay,
		Underlay:  node2Underlay,
		NetworkID: 0,
		Light:     false,
	}

	node1BzzAddress := &pb.BzzAddress{
		Overlay:   node1Info.Overlay.Bytes(),
		Underlay:  node1Info.Underlay,
		Signature: signature1,
	}

	node2BzzAddress := &pb.BzzAddress{
		Overlay:   node2Info.Overlay.Bytes(),
		Underlay:  node2Info.Underlay,
		Signature: signature2,
	}

	logger := logging.New(ioutil.Discard, 0)
	t.Run("OK", func(t *testing.T) {
		handshakeService, err := New(node1Info.Overlay, string(node1Info.Underlay), signer1, 0, logger)
		if err != nil {
			t.Fatal(err)
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.Syn{
			BzzAddress: node2BzzAddress,
			NetworkID:  node2Info.NetworkID,
			Light:      node2Info.Light,
		}); err != nil {
			t.Fatal(err)
		}

		if err := w.WriteMsg(&pb.Ack{BzzAddress: node1BzzAddress}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(stream1, node2AddrInfo.ID)
		if err != nil {
			t.Fatal(err)
		}

		testInfo(t, *res, node2Info)

		_, r := protobuf.NewWriterAndReader(stream2)
		var got pb.SynAck
		if err := r.ReadMsg(&got); err != nil {
			t.Fatal(err)
		}

		testInfo(t, node1Info, Info{
			Overlay:   swarm.NewAddress(got.Syn.BzzAddress.Overlay),
			Underlay:  got.Syn.BzzAddress.Underlay,
			NetworkID: got.Syn.NetworkID,
			Light:     got.Syn.Light,
		})
	})

	t.Run("ERROR - read error ", func(t *testing.T) {
		handshakeService, err := New(node1Info.Overlay, string(node1Info.Underlay), signer1, 0, logger)
		if err != nil {
			t.Fatal(err)
		}

		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("read syn message: %w", testErr)
		stream := &mock.Stream{}
		stream.SetReadErr(testErr, 0)
		res, err := handshakeService.Handle(stream, node2AddrInfo.ID)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handle returned non-nil res")
		}
	})

	t.Run("ERROR - write error ", func(t *testing.T) {
		handshakeService, err := New(node1Info.Overlay, string(node1Info.Underlay), signer1, 0, logger)
		if err != nil {
			t.Fatal(err)
		}

		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("write synack message: %w", testErr)
		var buffer bytes.Buffer
		stream := mock.NewStream(&buffer, &buffer)
		stream.SetWriteErr(testErr, 1)
		w, _ := protobuf.NewWriterAndReader(stream)
		if err := w.WriteMsg(&pb.Syn{
			BzzAddress: node2BzzAddress,
			NetworkID:  node2Info.NetworkID,
			Light:      node2Info.Light,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(stream, node2AddrInfo.ID)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("ERROR - ack read error ", func(t *testing.T) {
		handshakeService, err := New(node1Info.Overlay, string(node1Info.Underlay), signer1, 0, logger)
		if err != nil {
			t.Fatal(err)
		}

		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("read ack message: %w", testErr)
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)
		stream1.SetReadErr(testErr, 1)
		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.Syn{
			BzzAddress: node2BzzAddress,
			NetworkID:  node2Info.NetworkID,
			Light:      node2Info.Light,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(stream1, node2AddrInfo.ID)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("ERROR - networkID mismatch ", func(t *testing.T) {
		handshakeService, err := New(node1Info.Overlay, string(node1Info.Underlay), signer1, 0, logger)
		if err != nil {
			t.Fatal(err)
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.Syn{
			BzzAddress: node2BzzAddress,
			NetworkID:  5,
			Light:      node2Info.Light,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(stream1, node2AddrInfo.ID)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if err != ErrNetworkIDIncompatible {
			t.Fatalf("expected %s, got %s", ErrNetworkIDIncompatible, err)
		}
	})

	t.Run("ERROR - duplicate handshake", func(t *testing.T) {
		handshakeService, err := New(node1Info.Overlay, string(node1Info.Underlay), signer1, 0, logger)
		if err != nil {
			t.Fatal(err)
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.Syn{
			BzzAddress: node2BzzAddress,
			NetworkID:  node2Info.NetworkID,
			Light:      node2Info.Light,
		}); err != nil {
			t.Fatal(err)
		}

		if err := w.WriteMsg(&pb.Ack{BzzAddress: node1BzzAddress}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(stream1, node2AddrInfo.ID)
		if err != nil {
			t.Fatal(err)
		}

		testInfo(t, *res, node2Info)

		_, r := protobuf.NewWriterAndReader(stream2)
		var got pb.SynAck
		if err := r.ReadMsg(&got); err != nil {
			t.Fatal(err)
		}

		testInfo(t, node1Info, Info{
			Overlay:   swarm.NewAddress(got.Syn.BzzAddress.Overlay),
			Underlay:  got.Syn.BzzAddress.Underlay,
			NetworkID: got.Syn.NetworkID,
			Light:     got.Syn.Light,
		})

		_, err = handshakeService.Handle(stream1, node2AddrInfo.ID)
		if err != ErrHandshakeDuplicate {
			t.Fatalf("expected %s, got %s", ErrHandshakeDuplicate, err)
		}
	})

	t.Run("Error - invalid ack", func(t *testing.T) {
		handshakeService, err := New(node1Info.Overlay, string(node1Info.Underlay), signer1, 0, logger)
		if err != nil {
			t.Fatal(err)
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.Syn{
			BzzAddress: node2BzzAddress,
			NetworkID:  node2Info.NetworkID,
			Light:      node2Info.Light,
		}); err != nil {
			t.Fatal(err)
		}

		if err := w.WriteMsg(&pb.Ack{BzzAddress: node2BzzAddress}); err != nil {
			t.Fatal(err)
		}

		_, err = handshakeService.Handle(stream1, node2AddrInfo.ID)
		if err != ErrInvalidAck {
			t.Fatalf("expected %s, got %s", ErrInvalidAck, err)
		}
	})

	t.Run("ERROR - invalid signature ", func(t *testing.T) {
		handshakeService, err := New(node1Info.Overlay, string(node1Info.Underlay), signer1, 0, logger)
		if err != nil {
			t.Fatal(err)
		}

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.Syn{
			BzzAddress: &pb.BzzAddress{
				Underlay:  node2BzzAddress.Underlay,
				Signature: []byte("wrong signature"),
				Overlay:   node2BzzAddress.Overlay,
			},
			NetworkID: node2Info.NetworkID,
			Light:     node2Info.Light,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(stream1, node2AddrInfo.ID)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if err != ErrInvalidSignature {
			t.Fatalf("expected %s, got %s", ErrInvalidSignature, err)
		}
	})
}

// testInfo validates if two Info instances are equal.
func testInfo(t *testing.T, got, want Info) {
	t.Helper()

	if !got.Overlay.Equal(want.Overlay) || !bytes.Equal(got.Underlay, want.Underlay) || got.NetworkID != want.NetworkID || got.Light != want.Light {
		t.Fatalf("got info %+v, want %+v", got, want)
	}
}
