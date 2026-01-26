// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake/mock"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

//nolint:paralleltest
func TestHandshake(t *testing.T) {
	const (
		testWelcomeMessage = "HelloWorld"
	)

	logger := log.Noop
	networkID := uint64(3)

	node1ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}

	node1ma2, err := ma.NewMultiaddr("/ip6/::1/tcp/46881/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}

	node2ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS")
	if err != nil {
		t.Fatal(err)
	}

	node2ma2, err := ma.NewMultiaddr("/ip4/10.34.35.60/tcp/35315/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS")
	if err != nil {
		t.Fatal(err)
	}

	node1mas := []ma.Multiaddr{node1ma, node1ma2}
	node2mas := []ma.Multiaddr{node2ma, node2ma2}

	node1maBinary := bzz.SerializeUnderlays(node1mas)
	node2maBinary := bzz.SerializeUnderlays(node2mas)

	node1AddrInfos, err := libp2ppeer.AddrInfosFromP2pAddrs(node1ma, node1ma2)
	if err != nil {
		t.Fatal(err)
	}
	if len(node1AddrInfos) != 1 {
		t.Fatal("must be same peer")
	}
	node1AddrInfo := node1AddrInfos[0]

	node2AddrInfos, err := libp2ppeer.AddrInfosFromP2pAddrs(node2ma, node2ma2)
	if err != nil {
		t.Fatal(err)
	}
	if len(node2AddrInfos) != 1 {
		t.Fatal("must be same peer")
	}
	privateKey1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	privateKey2, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	nonce := common.HexToHash("0x1").Bytes()

	signer1 := crypto.NewDefaultSigner(privateKey1)
	signer2 := crypto.NewDefaultSigner(privateKey2)
	addr, err := crypto.NewOverlayAddress(privateKey1.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	node1BzzAddress, err := bzz.NewAddress(signer1, []ma.Multiaddr{node1ma, node1ma2}, addr, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	addr2, err := crypto.NewOverlayAddress(privateKey2.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	node2BzzAddress, err := bzz.NewAddress(signer2, []ma.Multiaddr{node2ma, node2ma2}, addr2, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	node1Info := handshake.Info{
		BzzAddress: node1BzzAddress,
		FullNode:   true,
	}
	node2Info := handshake.Info{
		BzzAddress: node2BzzAddress,
		FullNode:   true,
	}

	aaddresser := &AdvertisableAddresserMock{}

	handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, testWelcomeMessage, true, node1AddrInfo.ID, logger)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Handshake - OK", func(t *testing.T) {
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w, r := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				ObservedUnderlay: node1maBinary,
			},
			Ack: &pb.Ack{
				Address: &pb.BzzAddress{
					Underlay:  node2maBinary,
					Overlay:   node2BzzAddress.Overlay.Bytes(),
					Signature: node2BzzAddress.Signature,
				},
				NetworkID:      networkID,
				FullNode:       true,
				Nonce:          nonce,
				WelcomeMessage: testWelcomeMessage,
			},
		}); err != nil {
			t.Fatal(err)
		}
		if err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2mas)
		if err != nil {
			t.Fatal(err)
		}

		testInfo(t, *res, node2Info)

		var syn pb.Syn
		if err := r.ReadMsg(&syn); err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(syn.ObservedUnderlay, node2maBinary) {
			t.Fatal("bad syn")
		}

		var ack pb.Ack
		if err := r.ReadMsg(&ack); err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(ack.Address.Overlay, node1BzzAddress.Overlay.Bytes()) {
			t.Fatal("bad ack - overlay")
		}
		if !bytes.Equal(ack.Address.Underlay, node1maBinary) {
			t.Fatal("bad ack - underlay")
		}
		if !bytes.Equal(ack.Address.Signature, node1BzzAddress.Signature) {
			t.Fatal("bad ack - signature")
		}
		if ack.NetworkID != networkID {
			t.Fatal("bad ack - networkID")
		}
		if ack.FullNode != true {
			t.Fatal("bad ack - full node")
		}

		if ack.WelcomeMessage != testWelcomeMessage {
			t.Fatalf("Bad ack welcome message: want %s, got %s", testWelcomeMessage, ack.WelcomeMessage)
		}
	})

	t.Run("Handshake - picker error", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, "", true, node1AddrInfo.ID, logger)
		if err != nil {
			t.Fatal(err)
		}

		handshakeService.SetPicker(mockPicker(func(p p2p.Peer) bool { return false }))

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		if err := w.WriteMsg(&pb.Ack{
			Address: &pb.BzzAddress{
				Underlay:  node2maBinary,
				Overlay:   node2BzzAddress.Overlay.Bytes(),
				Signature: node2BzzAddress.Signature,
			},
			NetworkID: networkID,
			Nonce:     nonce,
			FullNode:  true,
		}); err != nil {
			t.Fatal(err)
		}

		_, err = handshakeService.Handle(context.Background(), stream1, node2mas)
		expectedErr := handshake.ErrPicker
		if !errors.Is(err, expectedErr) {
			t.Fatal("expected:", expectedErr, "got:", err)
		}
	})

	t.Run("Handshake - welcome message too long", func(t *testing.T) {
		const LongMessage = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Morbi consectetur urna ut lorem sollicitudin posuere. Donec sagittis laoreet sapien."

		expectedErr := handshake.ErrWelcomeMessageLength
		_, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nil, nil, LongMessage, true, node1AddrInfo.ID, logger)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}
	})

	t.Run("Handshake - dynamic welcome message too long", func(t *testing.T) {
		const LongMessage = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Morbi consectetur urna ut lorem sollicitudin posuere. Donec sagittis laoreet sapien."

		expectedErr := handshake.ErrWelcomeMessageLength
		err := handshakeService.SetWelcomeMessage(LongMessage)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}
	})

	t.Run("Handshake - set welcome message", func(t *testing.T) {
		const TestMessage = "Hi im the new test message"

		err := handshakeService.SetWelcomeMessage(TestMessage)
		if err != nil {
			t.Fatal("Got error:", err)
		}
		got := handshakeService.GetWelcomeMessage()
		if got != TestMessage {
			t.Fatal("expected:", TestMessage, ", got:", got)
		}
	})

	t.Run("Handshake - Syn write error", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("write syn message: %w", testErr)
		stream := &mock.Stream{}
		stream.SetWriteErr(testErr, 0)
		res, err := handshakeService.Handshake(context.Background(), stream, node2mas)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("Handshake - Syn read error", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("read synack message: %w", testErr)
		stream := mock.NewStream(nil, &bytes.Buffer{})
		stream.SetReadErr(testErr, 0)
		res, err := handshakeService.Handshake(context.Background(), stream, node2mas)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("Handshake - ack write error", func(t *testing.T) {
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("write ack message: %w", testErr)
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream1.SetWriteErr(testErr, 1)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				ObservedUnderlay: node1maBinary,
			},
			Ack: &pb.Ack{
				Address: &pb.BzzAddress{
					Underlay:  node2maBinary,
					Overlay:   node2BzzAddress.Overlay.Bytes(),
					Signature: node2BzzAddress.Signature,
				},
				Nonce:     nonce,
				NetworkID: networkID,
				FullNode:  true,
			},
		},
		); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2mas)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("Handshake - networkID mismatch", func(t *testing.T) {
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				ObservedUnderlay: node1maBinary,
			},
			Ack: &pb.Ack{
				Address: &pb.BzzAddress{
					Underlay:  node2maBinary,
					Overlay:   node2BzzAddress.Overlay.Bytes(),
					Signature: node2BzzAddress.Signature,
				},
				NetworkID: 5,
				FullNode:  true,
			},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2mas)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if !errors.Is(err, handshake.ErrNetworkIDIncompatible) {
			t.Fatalf("expected %s, got %s", handshake.ErrNetworkIDIncompatible, err)
		}
	})

	t.Run("Handshake - invalid ack", func(t *testing.T) {
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				ObservedUnderlay: node1maBinary,
			},
			Ack: &pb.Ack{
				Address: &pb.BzzAddress{
					Underlay:  node2maBinary,
					Overlay:   node2BzzAddress.Overlay.Bytes(),
					Signature: node1BzzAddress.Signature,
				},
				NetworkID: networkID,
				FullNode:  true,
			},
		}); err != nil {
			t.Fatal(err)
		}
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, testWelcomeMessage, true, node1AddrInfo.ID, logger)
		if err != nil {
			t.Fatal(err)
		}
		res, err := handshakeService.Handshake(context.Background(), stream1, node2mas)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if !errors.Is(err, handshake.ErrInvalidAck) {
			t.Fatalf("expected %s, got %s", handshake.ErrInvalidAck, err)
		}
	})

	t.Run("Handshake - error advertisable address", func(t *testing.T) {
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		testError := errors.New("test error")
		aaddresser.err = testError
		defer func() {
			aaddresser.err = nil
		}()

		w, _ := protobuf.NewWriterAndReader(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				ObservedUnderlay: node1maBinary,
			},
			Ack: &pb.Ack{
				Address: &pb.BzzAddress{
					Underlay:  node2maBinary,
					Overlay:   node2BzzAddress.Overlay.Bytes(),
					Signature: node2BzzAddress.Signature,
				},
				NetworkID: networkID,
				FullNode:  true,
			},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2mas)
		if !errors.Is(err, testError) {
			t.Fatalf("expected error %v got %v", testError, err)

		}

		if res != nil {
			t.Fatal("expected nil res")
		}

	})

	t.Run("Handle - OK", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, "", true, node1AddrInfo.ID, logger)
		if err != nil {
			t.Fatal(err)
		}
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		if err := w.WriteMsg(&pb.Ack{
			Address: &pb.BzzAddress{
				Underlay:  node2maBinary,
				Overlay:   node2BzzAddress.Overlay.Bytes(),
				Signature: node2BzzAddress.Signature,
			},
			NetworkID: networkID,
			Nonce:     nonce,
			FullNode:  true,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream1, node2mas)
		if err != nil {
			t.Fatal(err)
		}

		testInfo(t, *res, node2Info)

		_, r := protobuf.NewWriterAndReader(stream2)
		var got pb.SynAck
		if err := r.ReadMsg(&got); err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got.Syn.ObservedUnderlay, node2maBinary) {
			t.Fatalf("got bad syn")
		}

		bzzAddress, err := bzz.ParseAddress(got.Ack.Address.Underlay, got.Ack.Address.Overlay, got.Ack.Address.Signature, got.Ack.Nonce, true, got.Ack.NetworkID)
		if err != nil {
			t.Fatal(err)
		}

		testInfo(t, node1Info, handshake.Info{
			BzzAddress: bzzAddress,
			FullNode:   got.Ack.FullNode,
		})
	})

	t.Run("Handle - read error ", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nil, nil, "", true, node1AddrInfo.ID, logger)
		if err != nil {
			t.Fatal(err)
		}
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("read syn message: %w", testErr)
		stream := &mock.Stream{}
		stream.SetReadErr(testErr, 0)
		res, err := handshakeService.Handle(context.Background(), stream, node2mas)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handle returned non-nil res")
		}
	})

	t.Run("Handle - write error ", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nil, nil, "", true, node1AddrInfo.ID, logger)
		if err != nil {
			t.Fatal(err)
		}
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("write synack message: %w", testErr)
		var buffer bytes.Buffer
		stream := mock.NewStream(&buffer, &buffer)
		stream.SetWriteErr(testErr, 1)
		w := protobuf.NewWriter(stream)
		if err := w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream, node2mas)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("Handle - ack read error ", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nil, nil, "", true, node1AddrInfo.ID, logger)
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
		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream1, node2mas)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("Handle - networkID mismatch ", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nil, nil, "", true, node1AddrInfo.ID, logger)
		if err != nil {
			t.Fatal(err)
		}
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		if err := w.WriteMsg(&pb.Ack{
			Address: &pb.BzzAddress{
				Underlay:  node2maBinary,
				Overlay:   node2BzzAddress.Overlay.Bytes(),
				Signature: node2BzzAddress.Signature,
			},
			NetworkID: 5,
			FullNode:  true,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream1, node2mas)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if !errors.Is(err, handshake.ErrNetworkIDIncompatible) {
			t.Fatalf("expected %s, got %s", handshake.ErrNetworkIDIncompatible, err)
		}
	})

	t.Run("Handle - invalid ack", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nil, nil, "", true, node1AddrInfo.ID, logger)
		if err != nil {
			t.Fatal(err)
		}
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		if err := w.WriteMsg(&pb.Ack{
			Address: &pb.BzzAddress{
				Underlay:  node2maBinary,
				Overlay:   node2BzzAddress.Overlay.Bytes(),
				Signature: node1BzzAddress.Signature,
			},
			NetworkID: networkID,
			FullNode:  true,
		}); err != nil {
			t.Fatal(err)
		}

		_, err = handshakeService.Handle(context.Background(), stream1, node2mas)
		if !errors.Is(err, handshake.ErrInvalidAck) {
			t.Fatalf("expected %s, got %v", handshake.ErrInvalidAck, err)
		}
	})

	t.Run("Handle - advertisable error", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nil, nil, "", true, node1AddrInfo.ID, logger)
		if err != nil {
			t.Fatal(err)
		}
		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		testError := errors.New("test error")
		aaddresser.err = testError
		defer func() {
			aaddresser.err = nil
		}()

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.Syn{
			ObservedUnderlay: node1maBinary,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream1, node2mas)
		if !errors.Is(err, testError) {
			t.Fatal("expected error")
		}

		if res != nil {
			t.Fatal("expected nil res")
		}
	})
}

func mockPicker(f func(p2p.Peer) bool) p2p.Picker {
	return &picker{pickerFunc: f}
}

type picker struct {
	pickerFunc func(p2p.Peer) bool
}

func (p *picker) Pick(peer p2p.Peer) bool {
	return p.pickerFunc(peer)
}

// testInfo validates if two Info instances are equal.
func testInfo(t *testing.T, got, want handshake.Info) {
	t.Helper()
	if !got.BzzAddress.Equal(want.BzzAddress) || got.FullNode != want.FullNode {
		t.Fatalf("got info %+v, want %+v", got, want)
	}
}

type AdvertisableAddresserMock struct {
	advertisableAddress ma.Multiaddr
	err                 error
}

func (a *AdvertisableAddresserMock) Resolve(observedAddress ma.Multiaddr) (ma.Multiaddr, error) {
	if a.err != nil {
		return nil, a.err
	}

	if a.advertisableAddress != nil {
		return a.advertisableAddress, nil
	}

	return observedAddress, nil
}

// TestHandshakeWithEmptyUnderlays verifies that the handshake can successfully
// parse a peer address with empty underlays (e.g., inbound-only peers like
// browsers or peers behind strict NAT that cannot be dialed back).
func TestHandshakeWithEmptyUnderlays(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	networkID := uint64(3)

	// Node 1 has normal underlay addresses
	node1ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}

	node1AddrInfo, err := libp2ppeer.AddrInfoFromP2pAddr(node1ma)
	if err != nil {
		t.Fatal(err)
	}

	// Create private keys and signers
	privateKey1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	privateKey2, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	nonce := common.HexToHash("0x1").Bytes()

	signer1 := crypto.NewDefaultSigner(privateKey1)
	signer2 := crypto.NewDefaultSigner(privateKey2)

	// Create overlays
	overlay1, err := crypto.NewOverlayAddress(privateKey1.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	overlay2, err := crypto.NewOverlayAddress(privateKey2.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	// Node 2: Inbound-only peer with EMPTY underlays (simulates browser/WebRTC peer)
	node2BzzAddress, err := bzz.NewAddress(signer2, []ma.Multiaddr{}, overlay2, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	// Verify node2 has empty underlays
	if len(node2BzzAddress.Underlays) != 0 {
		t.Fatalf("expected node2 to have 0 underlays, got %d", len(node2BzzAddress.Underlays))
	}

	// Create handshake service for node1
	handshakeService1, err := handshake.New(
		signer1,
		&AdvertisableAddresserMock{},
		overlay1,
		networkID,
		true,
		nonce,
		nil,
		"node1",
		true,
		node1AddrInfo.ID,
		logger,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create mock stream following the existing test pattern
	var buffer1, buffer2 bytes.Buffer
	stream1 := mock.NewStream(&buffer1, &buffer2)
	stream2 := mock.NewStream(&buffer2, &buffer1)
	defer stream1.Close()
	defer stream2.Close()

	// Serialize empty underlays
	emptyUnderlaysBinary := bzz.SerializeUnderlays([]ma.Multiaddr{})

	// Pre-write the SynAck message with empty underlays (like the existing "Handshake - OK" test)
	w, r := protobuf.NewWriterAndReader(stream2)
	if err := w.WriteMsg(&pb.SynAck{
		Syn: &pb.Syn{
			ObservedUnderlay: bzz.SerializeUnderlays([]ma.Multiaddr{node1ma}),
		},
		Ack: &pb.Ack{
			Address: &pb.BzzAddress{
				Underlay:  emptyUnderlaysBinary, // Node 2 advertises EMPTY underlays
				Overlay:   node2BzzAddress.Overlay.Bytes(),
				Signature: node2BzzAddress.Signature,
			},
			NetworkID:      networkID,
			FullNode:       true,
			Nonce:          nonce,
			WelcomeMessage: "inbound-only-peer",
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Node 1 initiates handshake and receives node2's empty underlays
	info1, err := handshakeService1.Handshake(
		context.Background(),
		stream1,
		[]ma.Multiaddr{}, // Pass empty underlays as we don't know node2's address
	)

	if err != nil {
		t.Fatalf("handshake failed: %v", err)
	}

	// Read the Syn that node1 sent
	var syn pb.Syn
	if err := r.ReadMsg(&syn); err != nil {
		t.Fatal(err)
	}

	// Read the Ack that node1 sent back
	var ack pb.Ack
	if err := r.ReadMsg(&ack); err != nil {
		t.Fatal(err)
	}

	// Verify node1 successfully received node2's address with EMPTY underlays
	if len(info1.BzzAddress.Underlays) != 0 {
		t.Errorf("expected to receive 0 underlays, got %d", len(info1.BzzAddress.Underlays))
	}

	// Verify overlay address matches
	if !info1.BzzAddress.Overlay.Equal(overlay2) {
		t.Errorf("received wrong overlay: got %v, want %v", info1.BzzAddress.Overlay, overlay2)
	}

	// Node 2 is a full node
	if !info1.FullNode {
		t.Error("expected peer to be a full node")
	}
}
