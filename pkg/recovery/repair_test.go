// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package recovery_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"io/ioutil"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/content"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pushsync"
	pushsyncmock "github.com/ethersphere/bee/pkg/pushsync/mock"
	"github.com/ethersphere/bee/pkg/recovery"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	storemock "github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
)

// TestRecoveryHook tests that a recovery hook can be created and called.
func TestRecoveryHook(t *testing.T) {
	/// generate test chunk, store and publisher
	payload := make([]byte, swarm.ChunkSize)
	_, err := rand.Read(payload)
	if err != nil {
		t.Fatal(err)
	}
	c1, err := content.NewChunk(payload)
	if err != nil {
		t.Fatal(err)
	}

	targets := trojan.Targets{[]byte{0xED}}

	//setup the sender
	hookWasCalled := make(chan bool, 1) // channel to check if hook is called
	pssSender := &mockPssSender{
		hookC: hookWasCalled,
	}

	// create a new signer
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)
	publicKey, err := signer.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	overlayBytes, err := crypto.NewEthereumAddress(*publicKey)
	if err != nil {
		t.Fatal(err)
	}
	overlayAddress := swarm.NewAddress(overlayBytes)
	recoveryHook := recovery.NewRecoveryHook(pssSender, overlayAddress, signer)
	chunkC := make(chan swarm.Chunk)
	socAddress, err := recoveryHook(context.Background(), c1.Address(), targets, chunkC)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-hookWasCalled:
		break
	case <-time.After(100 * time.Millisecond):
		t.Fatal("recovery hook was not called")
	}

	// check if the mining process is done properly
	targetLength := len(targets[0])
	if !bytes.Equal(overlayAddress.Bytes()[:targetLength], socAddress.Bytes()[:targetLength]) {
		t.Fatal("invalid address mined")
	}

	if !bytes.Equal(pssSender.payload[:swarm.HashSize], socAddress.Bytes()) {
		t.Fatalf("invalid soc address in payload")
	}

	// extract the payload and check if that is equal to the chunk address requested
	cursor := swarm.HashSize + soc.IdSize + soc.SignatureSize
	span := binary.BigEndian.Uint64(pssSender.payload[cursor : cursor+swarm.SpanSize])
	cursor += swarm.SpanSize
	data := pssSender.payload[cursor:]
	if !bytes.Equal(c1.Address().Bytes(), data[:span]) {
		t.Fatal("invalid data (requested chunk address) in payload")
	}
}

// TestNewRepairHandler tests the function of repairing a chunk when a request for chunk repair is received.
func TestNewRepairHandler(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	t.Run("repair-chunk", func(t *testing.T) {
		// generate test chunk, store and publisher
		payload := make([]byte, swarm.ChunkSize)
		_, err := rand.Read(payload)
		if err != nil {
			t.Fatal(err)
		}
		c1, err := content.NewChunk(payload)
		if err != nil {
			t.Fatal(err)
		}

		// create a mock storer and put a chunk that will be repaired
		mockStorer := storemock.NewStorer()
		defer mockStorer.Close()
		_, err = mockStorer.Put(context.Background(), storage.ModePutRequest, c1)
		if err != nil {
			t.Fatal(err)
		}

		// create a mock pushsync service to push the chunk to its destination
		var receipt *pushsync.Receipt
		var recoveryResponse swarm.Chunk
		mockPushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			if receipt == nil {
				receipt = &pushsync.Receipt{
					Address: swarm.NewAddress(chunk.Address().Bytes()),
				}
				return receipt, nil
			}
			recoveryResponse = chunk
			return receipt, nil

		})

		//create a trojan message to trigger the repair of the chunk
		testTopic := trojan.NewTopic("foo")
		var msg trojan.Message
		privKey, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		signer := crypto.NewDefaultSigner(privKey)
		id := make([]byte, 32)
		data := make([]byte, swarm.SpanSize+swarm.HashSize)
		binary.BigEndian.PutUint64(data[:swarm.SpanSize], swarm.HashSize)
		copy(data[swarm.SpanSize:swarm.SpanSize+swarm.HashSize], c1.Address().Bytes())
		c := swarm.NewChunk(c1.Address(), data)
		sch, err := soc.NewChunk(id, c, signer)
		if err != nil {
			t.Fatal(err)
		}
		repairMessagePayload := append(sch.Address().Bytes(), sch.Data()...)
		if msg, err = trojan.NewMessage(testTopic, repairMessagePayload); err != nil {
			t.Fatal(err)
		}

		// create the chunk repair handler
		repairHandler := recovery.NewRepairHandler(mockStorer, logger, mockPushSyncService)

		// invoke the chunk repair handler
		repairHandler(context.Background(), &msg)

		// check if receipt is received
		if receipt == nil {
			t.Fatal("receipt not received")
		}

		if !receipt.Address.Equal(c1.Address()) {
			t.Fatalf("invalid address in receipt: expected %s received %s", c1.Address(), receipt.Address)
		}

		data = recoveryResponse.Data()
		cursor := uint64(soc.IdSize + soc.SignatureSize)
		span := binary.LittleEndian.Uint64(data[cursor : cursor+swarm.SpanSize])
		cursor += swarm.SpanSize
		if !bytes.Equal(c1.Data()[swarm.SpanSize:], recoveryResponse.Data()[cursor:cursor+span]) {
			t.Fatal("invalid data received")
		}
	})

	t.Run("repair-chunk-not-present", func(t *testing.T) {
		// generate test chunk, store and publisher
		payload := make([]byte, swarm.ChunkSize)
		_, err := rand.Read(payload)
		if err != nil {
			t.Fatal(err)
		}
		c2, err := content.NewChunk(payload)
		if err != nil {
			t.Fatal(err)
		}

		// create a mock storer
		mockStorer := storemock.NewStorer()
		defer mockStorer.Close()

		// create a mock pushsync service
		pushServiceCalled := false
		pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			pushServiceCalled = true
			return nil, nil
		})

		//create a trojan message to trigger the repair of the chunk
		testTopic := trojan.NewTopic("foo")
		var msg trojan.Message
		privKey, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		signer := crypto.NewDefaultSigner(privKey)
		id := make([]byte, 32)
		data := make([]byte, swarm.SpanSize+swarm.HashSize)
		binary.BigEndian.PutUint64(data[:swarm.SpanSize], swarm.HashSize)
		copy(data[swarm.SpanSize:swarm.SpanSize+swarm.HashSize], c2.Address().Bytes())
		c := swarm.NewChunk(c2.Address(), data)
		sch, err := soc.NewChunk(id, c, signer)
		if err != nil {
			t.Fatal(err)
		}
		trojanPayload := append(sch.Address().Bytes(), sch.Data()...)
		if msg, err = trojan.NewMessage(testTopic, trojanPayload); err != nil {
			t.Fatal(err)
		}
		msg, err = trojan.NewMessage(testTopic, trojanPayload)
		if err != nil {
			t.Fatal(err)
		}

		// create the chunk repair handler
		repairHandler := recovery.NewRepairHandler(mockStorer, logger, pushSyncService)

		// invoke the chunk repair handler
		repairHandler(context.Background(), &msg)

		if pushServiceCalled {
			t.Fatal("push service called even if the chunk is not present")
		}
	})
}

type mockPssSender struct {
	hookC   chan bool
	payload []byte
}

// Send mocks the pss Send function
func (mp *mockPssSender) Send(ctx context.Context, targets trojan.Targets, topic trojan.Topic, payload []byte) error {
	mp.payload = payload
	mp.hookC <- true
	return nil
}