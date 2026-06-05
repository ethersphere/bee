// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake/mock"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// noopAddressbook is a Getter that returns ErrNotFound for every overlay.
type noopAddressbook struct{}

func (noopAddressbook) Get(_ swarm.Address) (*bzz.Address, bool, error) {
	return nil, false, addressbook.ErrNotFound
}

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

	node1maBinary, err := bzz.SerializeUnderlays(node1mas)
	if err != nil {
		t.Fatal(err)
	}
	node2maBinary, err := bzz.SerializeUnderlays(node2mas)
	if err != nil {
		t.Fatal(err)
	}

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
	testTime := time.Unix(1700000000, 0)
	testTimestamp := testTime.Unix()
	node1BzzAddress, err := bzz.NewAddress(signer1, []ma.Multiaddr{node1ma, node1ma2}, addr, networkID, nonce, testTimestamp, common.Address{})
	if err != nil {
		t.Fatal(err)
	}
	addr2, err := crypto.NewOverlayAddress(privateKey2.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	node2BzzAddress, err := bzz.NewAddress(signer2, []ma.Multiaddr{node2ma, node2ma2}, addr2, networkID, nonce, testTimestamp, common.Address{})
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

	handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, testWelcomeMessage, noopAddressbook{}, node1AddrInfo.ID, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	handshakeService.SetTime(func() time.Time { return testTime })

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
					Nonce:     nonce,
					Timestamp: node2BzzAddress.Timestamp,
				},
				NetworkID:      networkID,
				FullNode:       true,
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

		synAddrs, err := bzz.DeserializeUnderlays(syn.ObservedUnderlay)
		if err != nil {
			t.Fatal(err)
		}
		if !bzz.AreUnderlaysEqual(synAddrs, node2mas) {
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
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, "", noopAddressbook{}, node1AddrInfo.ID, nil, logger)
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
				Nonce:     nonce,
				Timestamp: node2BzzAddress.Timestamp,
			},
			NetworkID: networkID,
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
		_, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, LongMessage, noopAddressbook{}, node1AddrInfo.ID, nil, logger)
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
					Nonce:     nonce,
					Timestamp: node2BzzAddress.Timestamp,
				},
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
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, testWelcomeMessage, noopAddressbook{}, node1AddrInfo.ID, nil, logger)
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
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, "", noopAddressbook{}, node1AddrInfo.ID, nil, logger)
		if err != nil {
			t.Fatal(err)
		}
		handshakeService.SetTime(func() time.Time { return testTime })
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
				Nonce:     nonce,
				Timestamp: node2BzzAddress.Timestamp,
			},
			NetworkID: networkID,
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

		gotSynAddrs, err := bzz.DeserializeUnderlays(got.Syn.ObservedUnderlay)
		if err != nil {
			t.Fatal(err)
		}
		if !bzz.AreUnderlaysEqual(gotSynAddrs, node2mas) {
			t.Fatalf("got bad syn")
		}

		bzzAddress, err := bzz.ParseAddress(got.Ack.Address.Underlay, got.Ack.Address.Overlay, got.Ack.Address.Signature, got.Ack.Address.Nonce, got.Ack.Address.Timestamp, got.Ack.NetworkID, got.Ack.Address.ChequebookAddress)
		if err != nil {
			t.Fatal(err)
		}

		testInfo(t, node1Info, handshake.Info{
			BzzAddress: bzzAddress,
			FullNode:   got.Ack.FullNode,
		})
	})

	t.Run("Handle - read error ", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, "", noopAddressbook{}, node1AddrInfo.ID, nil, logger)
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
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, "", noopAddressbook{}, node1AddrInfo.ID, nil, logger)
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
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, "", noopAddressbook{}, node1AddrInfo.ID, nil, logger)
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
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, "", noopAddressbook{}, node1AddrInfo.ID, nil, logger)
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
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, "", noopAddressbook{}, node1AddrInfo.ID, nil, logger)
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
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, "", noopAddressbook{}, node1AddrInfo.ID, nil, logger)
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

	// Regression tests for nil nested protobuf fields on the handshake
	// wire: a malicious peer can send syntactically decodable messages that
	// omit nested pointer fields (SynAck.Syn, SynAck.Ack, Ack.Address), and
	// the parser must reject them as protocol errors rather than panic
	// (unauthenticated DoS). Each subtest recovers from panics so the test
	// binary itself does not crash if a guard is missing.
	t.Run("Handshake - nil SynAck.Syn", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("handshake panicked on nil SynAck.Syn: %v", r)
			}
		}()

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			// Syn is intentionally nil.
			Ack: &pb.Ack{
				Address: &pb.BzzAddress{
					Underlay:          node2maBinary,
					Overlay:           node2BzzAddress.Overlay.Bytes(),
					Signature:         node2BzzAddress.Signature,
					Nonce:             nonce,
					Timestamp:         node2BzzAddress.Timestamp,
					ChequebookAddress: node2BzzAddress.ChequebookAddress.Bytes(),
				},
				NetworkID: networkID,
				FullNode:  true,
			},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2mas)
		if res != nil {
			t.Fatal("res should be nil")
		}
		if !errors.Is(err, handshake.ErrInvalidSyn) {
			t.Fatalf("expected %s, got %v", handshake.ErrInvalidSyn, err)
		}
	})

	t.Run("Handshake - nil SynAck.Ack", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("handshake panicked on nil SynAck.Ack: %v", r)
			}
		}()

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.SynAck{
			Syn: &pb.Syn{
				ObservedUnderlay: node1maBinary,
			},
			// Ack is intentionally nil.
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2mas)
		if res != nil {
			t.Fatal("res should be nil")
		}
		if !errors.Is(err, handshake.ErrInvalidAck) {
			t.Fatalf("expected %s, got %v", handshake.ErrInvalidAck, err)
		}
	})

	t.Run("Handshake - nil SynAck.Ack.Address", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("handshake panicked on nil Ack.Address: %v", r)
			}
		}()

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
				// Address is intentionally nil.
				NetworkID: networkID,
				FullNode:  true,
			},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2mas)
		if res != nil {
			t.Fatal("res should be nil")
		}
		if !errors.Is(err, handshake.ErrInvalidAck) {
			t.Fatalf("expected %s, got %v", handshake.ErrInvalidAck, err)
		}
	})

	t.Run("Handle - nil Ack.Address", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, node1Info.BzzAddress.Overlay, networkID, true, nonce, nil, "", noopAddressbook{}, node1AddrInfo.ID, nil, logger)
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
			// Address is intentionally nil.
			NetworkID: networkID,
			FullNode:  true,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream1, node2mas)
		if res != nil {
			t.Fatal("res should be nil")
		}
		if !errors.Is(err, handshake.ErrInvalidAck) {
			t.Fatalf("expected %s, got %v", handshake.ErrInvalidAck, err)
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

// newTimestampTestService builds a handshake.Service wired to a fixed clock so
// that parseCheckAck can be exercised deterministically.
func newTimestampTestService(t *testing.T, networkID uint64, now time.Time) *handshake.Service {
	t.Helper()

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)
	nonce := common.HexToHash("0x1").Bytes()

	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	m, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}
	infos, err := libp2ppeer.AddrInfosFromP2pAddrs(m)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := handshake.New(signer, resolveIdentity{}, overlay, networkID, true, nonce, nil, "", noopAddressbook{}, infos[0].ID, nil, log.Noop)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetTime(func() time.Time { return now })
	return svc
}

type resolveIdentity struct{}

func (resolveIdentity) Resolve(observed ma.Multiaddr) (ma.Multiaddr, error) {
	return observed, nil
}

// signProtoAck builds a signed pb.Ack for a fresh peer identity with the given
// timestamp, bypassing bzz.NewAddress's positivity guard so rejection paths
// can be exercised.
func signProtoAck(t *testing.T, networkID uint64, ts int64) *pb.Ack {
	t.Helper()

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)
	nonce := common.HexToHash("0x2").Bytes()

	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	u, err := ma.NewMultiaddr("/ip4/10.0.0.5/tcp/7070")
	if err != nil {
		t.Fatal(err)
	}
	underlaysBytes, err := bzz.SerializeUnderlays([]ma.Multiaddr{u})
	if err != nil {
		t.Fatal(err)
	}

	sig, err := signer.Sign(handshakeSigningBytes(underlaysBytes, overlay.Bytes(), networkID, nonce, ts))
	if err != nil {
		t.Fatal(err)
	}

	return &pb.Ack{
		Address: &pb.BzzAddress{
			Underlay:  underlaysBytes,
			Overlay:   overlay.Bytes(),
			Signature: sig,
			Nonce:     nonce,
			Timestamp: ts,
		},
		NetworkID: networkID,
		FullNode:  true,
	}
}

// handshakeSigningBytes mirrors bzz.generateSignData so this test can produce
// wire records with timestamps that NewAddress would otherwise reject.
func handshakeSigningBytes(underlay, overlay []byte, networkID uint64, nonce []byte, timestamp int64) []byte {
	networkIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(networkIDBytes, networkID)
	tsBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(tsBytes, uint64(timestamp))
	signData := append([]byte("bee-handshake-"), underlay...)
	signData = append(signData, overlay...)
	signData = append(signData, networkIDBytes...)
	signData = append(signData, nonce...)
	signData = append(signData, tsBytes...)
	return append(signData, (common.Address{}).Bytes()...)
}

// TestParseCheckAck_TimestampRejections exercises the rejection branches of
// parseCheckAck — future beyond skew, ts=0, negative ts — and confirms a
// future-at-skew-boundary is accepted.
func TestParseCheckAck_TimestampRejections(t *testing.T) {
	t.Parallel()

	networkID := uint64(7)
	now := time.Unix(1700000000, 0)
	svc := newTimestampTestService(t, networkID, now)

	t.Run("future beyond skew rejected", func(t *testing.T) {
		ack := signProtoAck(t, networkID, now.Add(bzz.MaxClockSkew+2*time.Second).Unix())
		if _, err := svc.ParseCheckAck(context.Background(), ack); !errors.Is(err, bzz.ErrTimestampInFuture) {
			t.Fatalf("expected ErrTimestampInFuture for future-beyond-skew, got %v", err)
		}
	})

	t.Run("future at skew boundary accepted", func(t *testing.T) {
		ack := signProtoAck(t, networkID, now.Add(bzz.MaxClockSkew).Unix())
		if _, err := svc.ParseCheckAck(context.Background(), ack); err != nil {
			t.Fatalf("expected acceptance at skew boundary, got %v", err)
		}
	})

	t.Run("zero timestamp rejected", func(t *testing.T) {
		ack := signProtoAck(t, networkID, 0)
		if _, err := svc.ParseCheckAck(context.Background(), ack); !errors.Is(err, bzz.ErrTimestampInvalid) {
			t.Fatalf("expected ErrTimestampInvalid for ts=0, got %v", err)
		}
	})

	t.Run("negative timestamp rejected", func(t *testing.T) {
		ack := signProtoAck(t, networkID, -1)
		if _, err := svc.ParseCheckAck(context.Background(), ack); !errors.Is(err, bzz.ErrTimestampInvalid) {
			t.Fatalf("expected ErrTimestampInvalid for negative ts, got %v", err)
		}
	})
}

// testUnderlays returns a single-multiaddr underlay set on the given port so
// tests can produce distinct canonical underlay sets cheaply.
func testUnderlays(t *testing.T, port int) []ma.Multiaddr {
	t.Helper()

	m, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA", port))
	if err != nil {
		t.Fatal(err)
	}
	return []ma.Multiaddr{m}
}

// TestSignedAddress_StableAcrossCalls verifies that the signed address is
// minted once and reused verbatim on subsequent calls, even when the clock
// advances.
func TestSignedAddress_StableAcrossCalls(t *testing.T) {
	t.Parallel()

	networkID := uint64(3)
	now := time.Unix(1700000000, 0)
	svc := newTimestampTestService(t, networkID, now)

	underlays := testUnderlays(t, 1634)

	first, err := svc.SignedAddress(underlays)
	if err != nil {
		t.Fatal(err)
	}
	if first.Timestamp != now.Unix() {
		t.Fatalf("first timestamp: got %d, want %d", first.Timestamp, now.Unix())
	}

	svc.SetTime(func() time.Time { return now.Add(time.Hour) })

	second, err := svc.SignedAddress(underlays)
	if err != nil {
		t.Fatal(err)
	}

	if second.Timestamp != first.Timestamp {
		t.Fatalf("timestamp changed across calls: got %d, want %d", second.Timestamp, first.Timestamp)
	}
	if !bytes.Equal(second.Signature, first.Signature) {
		t.Fatal("signature changed across calls for the same underlay set")
	}
}

// TestSignedAddress_ReSignsOnChange verifies that changing the underlay set or
// the local chequebook mints a new record with a strictly newer timestamp,
// even within the same clock second.
func TestSignedAddress_ReSignsOnChange(t *testing.T) {
	t.Parallel()

	networkID := uint64(3)
	now := time.Unix(1700000000, 0)
	svc := newTimestampTestService(t, networkID, now)

	first, err := svc.SignedAddress(testUnderlays(t, 1634))
	if err != nil {
		t.Fatal(err)
	}

	second, err := svc.SignedAddress(testUnderlays(t, 1635))
	if err != nil {
		t.Fatal(err)
	}
	if second.Timestamp != first.Timestamp+1 {
		t.Fatalf("expected monotonic bump on underlay change: got %d, want %d", second.Timestamp, first.Timestamp+1)
	}

	chequebook := common.HexToAddress("0xab4f3b3c1d2e000000000000000000000000cafe")
	svc.SetChequebookAddress(chequebook)

	third, err := svc.SignedAddress(testUnderlays(t, 1634))
	if err != nil {
		t.Fatal(err)
	}
	if third.Timestamp <= second.Timestamp {
		t.Fatalf("expected newer timestamp after chequebook change: got %d, existing %d", third.Timestamp, second.Timestamp)
	}
	if third.ChequebookAddress != chequebook {
		t.Fatalf("chequebook: got %s, want %s", third.ChequebookAddress, chequebook)
	}
}

// TestSignedAddress_ClockRegression verifies a record minted after the wall
// clock steps back still carries a strictly newer timestamp.
func TestSignedAddress_ClockRegression(t *testing.T) {
	t.Parallel()

	networkID := uint64(3)
	now := time.Unix(1700000000, 0)
	svc := newTimestampTestService(t, networkID, now)

	first, err := svc.SignedAddress(testUnderlays(t, 1634))
	if err != nil {
		t.Fatal(err)
	}

	svc.SetTime(func() time.Time { return now.Add(-time.Hour) })

	second, err := svc.SignedAddress(testUnderlays(t, 1635))
	if err != nil {
		t.Fatal(err)
	}
	if second.Timestamp != first.Timestamp+1 {
		t.Fatalf("timestamp after clock regression: got %d, want %d", second.Timestamp, first.Timestamp+1)
	}
}

// TestSignedAddress_LatestEntryOnly verifies the cache keeps only the most
// recently minted address: returning to a previously used underlay set
// re-mints with a strictly newer timestamp instead of reviving the old record.
func TestSignedAddress_LatestEntryOnly(t *testing.T) {
	t.Parallel()

	networkID := uint64(3)
	now := time.Unix(1700000000, 0)
	svc := newTimestampTestService(t, networkID, now)

	first, err := svc.SignedAddress(testUnderlays(t, 2000))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.SignedAddress(testUnderlays(t, 2001)); err != nil {
		t.Fatal(err)
	}

	reminted, err := svc.SignedAddress(testUnderlays(t, 2000))
	if err != nil {
		t.Fatal(err)
	}
	if reminted.Timestamp != first.Timestamp+2 {
		t.Fatalf("expected re-mint with newer timestamp: got %d, want %d", reminted.Timestamp, first.Timestamp+2)
	}

	cached, err := svc.SignedAddress(testUnderlays(t, 2000))
	if err != nil {
		t.Fatal(err)
	}
	if cached.Timestamp != reminted.Timestamp || !bytes.Equal(cached.Signature, reminted.Signature) {
		t.Fatal("latest entry not reused verbatim")
	}
}

// TestHandle_SignedAddressStableAcrossHandshakes verifies end to end that two
// inbound handshakes advertise a byte-identical BzzAddress record (timestamp
// and signature) even when the clock advances between them.
func TestHandle_SignedAddressStableAcrossHandshakes(t *testing.T) {
	t.Parallel()

	networkID := uint64(3)
	now := time.Unix(1700000000, 0)
	svc := newTimestampTestService(t, networkID, now)

	observed := testUnderlays(t, 1634)
	observedBinary, err := bzz.SerializeUnderlays(observed)
	if err != nil {
		t.Fatal(err)
	}

	run := func(clock time.Time) *pb.SynAck {
		t.Helper()

		svc.SetTime(func() time.Time { return clock })

		var buffer1 bytes.Buffer
		var buffer2 bytes.Buffer
		stream1 := mock.NewStream(&buffer1, &buffer2)
		stream2 := mock.NewStream(&buffer2, &buffer1)

		w := protobuf.NewWriter(stream2)
		if err := w.WriteMsg(&pb.Syn{ObservedUnderlay: observedBinary}); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteMsg(signProtoAck(t, networkID, clock.Unix())); err != nil {
			t.Fatal(err)
		}

		if _, err := svc.Handle(context.Background(), stream1, observed); err != nil {
			t.Fatal(err)
		}

		_, r := protobuf.NewWriterAndReader(stream2)
		var got pb.SynAck
		if err := r.ReadMsg(&got); err != nil {
			t.Fatal(err)
		}
		return &got
	}

	first := run(now)
	second := run(now.Add(time.Hour))

	if second.Ack.Address.Timestamp != first.Ack.Address.Timestamp {
		t.Fatalf("advertised timestamp changed across handshakes: got %d, want %d", second.Ack.Address.Timestamp, first.Ack.Address.Timestamp)
	}
	if !bytes.Equal(second.Ack.Address.Signature, first.Ack.Address.Signature) {
		t.Fatal("advertised signature changed across handshakes")
	}
	if !bytes.Equal(second.Ack.Address.Underlay, first.Ack.Address.Underlay) {
		t.Fatal("advertised underlays changed across handshakes")
	}
}
