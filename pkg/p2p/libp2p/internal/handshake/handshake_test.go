// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/bzz"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/mock"
	"github.com/ethersphere/bee/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/swarm"

	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func TestHandshake(t *testing.T) {
	const (
		testWelcomeMessage = "HelloWorld"
	)

	logger := logging.New(ioutil.Discard, 0)
	networkID := uint64(3)

	node1ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkA")
	if err != nil {
		t.Fatal(err)
	}
	node2ma, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634/p2p/16Uiu2HAkx8ULY8cTXhdVAcMmLcH9AsTKz6uBQ7DPLKRjMLgBVYkS")
	if err != nil {
		t.Fatal(err)
	}
	node1maBinary, err := node1ma.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	node2maBinary, err := node2ma.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	node2AddrInfo, err := libp2ppeer.AddrInfoFromP2pAddr(node2ma)
	if err != nil {
		t.Fatal(err)
	}

	privateKey1, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	privateKey2, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	trxHash := common.HexToHash("0x1").Bytes()
	blockhash := common.HexToHash("0x2").Bytes()

	signer1 := crypto.NewDefaultSigner(privateKey1)
	signer2 := crypto.NewDefaultSigner(privateKey2)
	addr, err := crypto.NewOverlayAddress(privateKey1.PublicKey, networkID, blockhash)
	if err != nil {
		t.Fatal(err)
	}
	node1BzzAddress, err := bzz.NewAddress(signer1, node1ma, addr, networkID, trxHash)
	if err != nil {
		t.Fatal(err)
	}
	addr2, err := crypto.NewOverlayAddress(privateKey2.PublicKey, networkID, blockhash)
	if err != nil {
		t.Fatal(err)
	}
	node2BzzAddress, err := bzz.NewAddress(signer2, node2ma, addr2, networkID, trxHash)
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

	senderMatcher := &MockSenderMatcher{v: true, blockHash: blockhash}

	handshakeService, err := handshake.New(signer1, aaddresser, senderMatcher, node1Info.BzzAddress.Overlay, networkID, true, trxHash, testWelcomeMessage, logger)
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
				Transaction:    trxHash,
				WelcomeMessage: testWelcomeMessage,
			},
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
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

		if !bytes.Equal(ack.Address.Overlay, node1BzzAddress.Overlay.Bytes()) ||
			!bytes.Equal(ack.Address.Underlay, node1maBinary) ||
			!bytes.Equal(ack.Address.Signature, node1BzzAddress.Signature) ||
			ack.NetworkID != networkID ||
			ack.FullNode != true {
			t.Fatal("bad ack")
		}

		if ack.WelcomeMessage != testWelcomeMessage {
			t.Fatalf("Bad ack welcome message: want %s, got %s", testWelcomeMessage, ack.WelcomeMessage)
		}
	})

	t.Run("Handshake - welcome message too long", func(t *testing.T) {
		const LongMessage = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Morbi consectetur urna ut lorem sollicitudin posuere. Donec sagittis laoreet sapien."

		expectedErr := handshake.ErrWelcomeMessageLength
		_, err := handshake.New(signer1, aaddresser, senderMatcher, node1Info.BzzAddress.Overlay, networkID, true, nil, LongMessage, logger)
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
		res, err := handshakeService.Handshake(context.Background(), stream, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
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
		res, err := handshakeService.Handshake(context.Background(), stream, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
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
				NetworkID: networkID,
				FullNode:  true,
			},
		},
		); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handshake(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
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

		res, err := handshakeService.Handshake(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if err != handshake.ErrNetworkIDIncompatible {
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

		res, err := handshakeService.Handshake(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if err != handshake.ErrInvalidAck {
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

		res, err := handshakeService.Handshake(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err != testError {
			t.Fatalf("expected error %v got %v", testError, err)

		}

		if res != nil {
			t.Fatal("expected nil res")
		}

	})

	t.Run("Handle - OK", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, senderMatcher, node1Info.BzzAddress.Overlay, networkID, true, trxHash, "", logger)
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
			NetworkID:   networkID,
			Transaction: trxHash,
			FullNode:    true,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
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

		bzzAddress, err := bzz.ParseAddress(got.Ack.Address.Underlay, got.Ack.Address.Overlay, got.Ack.Address.Signature, got.Ack.NetworkID, got.Ack.Transaction, blockhash)
		if err != nil {
			t.Fatal(err)
		}

		testInfo(t, node1Info, handshake.Info{
			BzzAddress: bzzAddress,
			FullNode:   got.Ack.FullNode,
		})
	})

	t.Run("Handle - read error ", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, senderMatcher, node1Info.BzzAddress.Overlay, networkID, true, nil, "", logger)
		if err != nil {
			t.Fatal(err)
		}
		testErr := errors.New("test error")
		expectedErr := fmt.Errorf("read syn message: %w", testErr)
		stream := &mock.Stream{}
		stream.SetReadErr(testErr, 0)
		res, err := handshakeService.Handle(context.Background(), stream, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handle returned non-nil res")
		}
	})

	t.Run("Handle - write error ", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, senderMatcher, node1Info.BzzAddress.Overlay, networkID, true, nil, "", logger)
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

		res, err := handshakeService.Handle(context.Background(), stream, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("Handle - ack read error ", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, senderMatcher, node1Info.BzzAddress.Overlay, networkID, true, nil, "", logger)
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

		res, err := handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err == nil || err.Error() != expectedErr.Error() {
			t.Fatal("expected:", expectedErr, "got:", err)
		}

		if res != nil {
			t.Fatal("handshake returned non-nil res")
		}
	})

	t.Run("Handle - networkID mismatch ", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, senderMatcher, node1Info.BzzAddress.Overlay, networkID, true, nil, "", logger)
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

		res, err := handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if res != nil {
			t.Fatal("res should be nil")
		}

		if err != handshake.ErrNetworkIDIncompatible {
			t.Fatalf("expected %s, got %s", handshake.ErrNetworkIDIncompatible, err)
		}
	})

	t.Run("Handle - duplicate handshake", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, senderMatcher, node1Info.BzzAddress.Overlay, networkID, true, trxHash, "", logger)
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
			NetworkID:   networkID,
			Transaction: trxHash,
			FullNode:    true,
		}); err != nil {
			t.Fatal(err)
		}

		res, err := handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
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
		bzzAddress, err := bzz.ParseAddress(got.Ack.Address.Underlay, got.Ack.Address.Overlay, got.Ack.Address.Signature, got.Ack.NetworkID, got.Ack.Transaction, blockhash)
		if err != nil {
			t.Fatal(err)
		}

		testInfo(t, node1Info, handshake.Info{
			BzzAddress: bzzAddress,
			FullNode:   got.Ack.FullNode,
		})

		_, err = handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err != handshake.ErrHandshakeDuplicate {
			t.Fatalf("expected %s, got %s", handshake.ErrHandshakeDuplicate, err)
		}
	})

	t.Run("Handle - invalid ack", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, senderMatcher, node1Info.BzzAddress.Overlay, networkID, true, nil, "", logger)
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

		_, err = handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err != handshake.ErrInvalidAck {
			t.Fatalf("expected %s, got %v", handshake.ErrInvalidAck, err)
		}
	})

	t.Run("Handle - transaction is not on the blockchain", func(t *testing.T) {
		sbMock := &MockSenderMatcher{v: false, blockHash: blockhash}

		handshakeService, err := handshake.New(signer1, aaddresser, sbMock, node1Info.BzzAddress.Overlay, networkID, true, trxHash, "", logger)
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
			NetworkID:   networkID,
			FullNode:    true,
			Transaction: trxHash,
		}); err != nil {
			t.Fatal(err)
		}

		_, err = handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if !errors.Is(err, handshake.ErrAddressNotFound) {
			t.Fatalf("expected error %v, got %v", handshake.ErrAddressNotFound, err)
		}
	})

	t.Run("Handle - advertisable error", func(t *testing.T) {
		handshakeService, err := handshake.New(signer1, aaddresser, senderMatcher, node1Info.BzzAddress.Overlay, networkID, true, nil, "", logger)
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

		res, err := handshakeService.Handle(context.Background(), stream1, node2AddrInfo.Addrs[0], node2AddrInfo.ID)
		if err != testError {
			t.Fatal("expected error")
		}

		if res != nil {
			t.Fatal("expected nil res")
		}
	})
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

func (a *AdvertisableAddresserMock) Resolve(observedAdddress ma.Multiaddr) (ma.Multiaddr, error) {
	if a.err != nil {
		return nil, a.err
	}

	if a.advertisableAddress != nil {
		return a.advertisableAddress, nil
	}

	return observedAdddress, nil
}

type MockSenderMatcher struct {
	v         bool
	blockHash []byte
}

func (m MockSenderMatcher) Matches(context.Context, []byte, uint64, swarm.Address) (bool, []byte, error) {
	return m.v, m.blockHash, nil
}
