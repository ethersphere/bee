// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package recovery_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io/ioutil"
	"testing"
	"time"

	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/content"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/netstore"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/pushsync"
	pushsyncmock "github.com/ethersphere/bee/pkg/pushsync/mock"
	"github.com/ethersphere/bee/pkg/recovery"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	storemock "github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
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

	// create recovery hook and call it
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
	overlayAddres := swarm.NewAddress(overlayBytes)
	recoveryHook := recovery.NewRecoveryHook(pssSender, overlayAddres, signer)
	chunkC := make(chan swarm.Chunk)
	socAddress, err := recoveryHook(c1.Address(), targets, chunkC)
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
	if overlayAddres.Bytes()[0] != socAddress.Bytes()[0] {
		t.Fatal("invalid address mined")
	}

	if !bytes.Equal(pssSender.payload[:swarm.SectionSize], socAddress.Bytes()) {
		t.Fatalf("invalid soc address in payload")
	}

	// extract the payload and check if that is equal to the chunk address requested
	cursor := swarm.SectionSize + soc.IdSize + soc.SignatureSize
	span := binary.BigEndian.Uint64(pssSender.payload[cursor : cursor+swarm.SpanSize])
	cursor += swarm.SpanSize
	data := pssSender.payload[cursor:]
	if !bytes.Equal(c1.Address().Bytes(), data[:span]) {
		t.Fatal("invalid data (requested chunk address) in payload")
	}
}

func Test_Miner(t *testing.T) {
	payload := make([]byte, swarm.ChunkSize)
	_, err := rand.Read(payload)
	if err != nil {
		t.Fatal(err)
	}
	c1, err := content.NewChunk(payload)
	if err != nil {
		t.Fatal(err)
	}

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
	overlayAddres := swarm.NewAddress(overlayBytes)

	// with target len 1
	envelope1, err := recovery.CreateSelfAddressedEnvelope(overlayAddres, 1, c1.Address(), signer)
	if err != nil {
		t.Fatal(err)
	}
	if envelope1.Address().Bytes()[0] != overlayAddres.Bytes()[0] {
		t.Fatal(" invalid prefix mined")
	}

	// with target len 2
	envelope2, err := recovery.CreateSelfAddressedEnvelope(overlayAddres, 2, c1.Address(), signer)
	if err != nil {
		t.Fatal(err)
	}
	if envelope2.Address().Bytes()[0] != overlayAddres.Bytes()[0] {
		t.Fatal(" invalid prefix mined")
	}
}

// RecoveryHookTestCase is a struct used as test cases for the TestRecoveryHookCalls func.
type recoveryHookTestCase struct {
	name           string
	ctx            context.Context
	expectsFailure bool
}

// TestRecoveryHookCalls verifies that recovery hooks are being called as expected when net store attempts to get a chunk.
func TestRecoveryHookCalls(t *testing.T) {
	//// generate test chunk, store and publisher
	payload := make([]byte, swarm.ChunkSize)
	_, err := rand.Read(payload)
	if err != nil {
		t.Fatal(err)
	}
	c1, err := content.NewChunk(payload)
	if err != nil {
		t.Fatal(err)
	}
	target := "BE"

	// test cases variables
	dummyContext := context.Background() // has no publisher
	targetContext := sctx.SetTargets(context.Background(), target)

	for _, tc := range []recoveryHookTestCase{
		{
			name:           "no targets in context",
			ctx:            dummyContext,
			expectsFailure: true,
		},
		{
			name:           "targets set in context",
			ctx:            targetContext,
			expectsFailure: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hookWasCalled := make(chan bool, 1) // channel to check if hook is called

			// setup the sender
			pssSender := &mockPssSender{
				hookC: hookWasCalled,
			}
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
			overlayAddres := swarm.NewAddress(overlayBytes)
			recoverFunc := recovery.NewRecoveryHook(pssSender, overlayAddres, signer)
			ns, _ := newTestNetStore(t, recoverFunc)

			// fetch test chunk
			_, err = ns.Get(tc.ctx, storage.ModeGetRequest, c1.Address())
			if err != nil && !errors.Is(err, netstore.ErrRecoveryTimeout) && err.Error() != "error decoding prefix string" {
				t.Fatal(err)
			}

			// checks whether the callback is invoked or the test case times out
			select {
			case <-hookWasCalled:
				if !tc.expectsFailure {
					break
				}
				t.Fatal("recovery hook was unexpectedly called")
			case <-time.After(1000 * time.Millisecond):
				if tc.expectsFailure {
					return
				}
				t.Fatal("recovery hook was not called when expected")
			}
		})
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
		var socChunk swarm.Chunk
		pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			if receipt == nil {
				receipt = &pushsync.Receipt{
					Address: swarm.NewAddress(chunk.Address().Bytes()),
				}
				return receipt, nil
			}
			socChunk = chunk
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
		data := make([]byte, swarm.SpanSize+swarm.SectionSize)
		binary.BigEndian.PutUint64(data[:swarm.SpanSize], swarm.SectionSize)
		copy(data[swarm.SpanSize:swarm.SpanSize+swarm.SectionSize], c1.Address().Bytes())
		c := swarm.NewChunk(c1.Address(), data)
		sch, err := soc.NewChunk(id, c, signer)
		if err != nil {
			t.Fatal(err)
		}
		maxPayload := append(sch.Address().Bytes(), sch.Data()...)
		if msg, err = trojan.NewMessage(testTopic, maxPayload); err != nil {
			t.Fatal(err)
		}

		// create the chunk repair handler
		repairHandler := recovery.NewRepairHandler(mockStorer, logger, pushSyncService)

		// invoke the chunk repair handler
		repairHandler(context.Background(), &msg)

		// check if receipt is received
		if receipt == nil {
			t.Fatal("receipt not received")
		}

		if !receipt.Address.Equal(c1.Address()) {
			t.Fatalf("invalid address in receipt: expected %s received %s", c1.Address(), receipt.Address)
		}

		data = socChunk.Data()
		cursor := uint64(soc.IdSize + soc.SignatureSize)
		span := binary.LittleEndian.Uint64(data[cursor : cursor+swarm.SpanSize])
		cursor += swarm.SpanSize
		if !bytes.Equal(c1.Data()[swarm.SpanSize:], socChunk.Data()[cursor:cursor+span]) {
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
		data := make([]byte, swarm.SpanSize+swarm.SectionSize)
		binary.BigEndian.PutUint64(data[:swarm.SpanSize], swarm.SectionSize)
		copy(data[swarm.SpanSize:swarm.SpanSize+swarm.SectionSize], c2.Address().Bytes())
		c := swarm.NewChunk(c2.Address(), data)
		sch, err := soc.NewChunk(id, c, signer)
		if err != nil {
			t.Fatal(err)
		}
		maxPayload := append(sch.Address().Bytes(), sch.Data()...)
		if msg, err = trojan.NewMessage(testTopic, maxPayload); err != nil {
			t.Fatal(err)
		}
		msg, err = trojan.NewMessage(testTopic, maxPayload)
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

	t.Run("repair-chunk-closest-peer-not-present", func(t *testing.T) {
		// generate test chunk, store and publisher
		payload := make([]byte, swarm.ChunkSize)
		_, err := rand.Read(payload)
		if err != nil {
			t.Fatal(err)
		}
		c3, err := content.NewChunk(payload)
		if err != nil {
			t.Fatal(err)
		}

		// create a mock storer
		mockStorer := storemock.NewStorer()
		defer mockStorer.Close()
		_, err = mockStorer.Put(context.Background(), storage.ModePutRequest, c3)
		if err != nil {
			t.Fatal(err)
		}

		// create a mock pushsync service
		var receiptError error
		pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			receiptError = errors.New("invalid receipt")
			return nil, receiptError
		})

		//create a trojan message to trigger the repair of the chunk
		testTopic := trojan.NewTopic("foo")
		privKey, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		signer := crypto.NewDefaultSigner(privKey)
		id := make([]byte, 32)
		data := make([]byte, swarm.SpanSize+swarm.SectionSize)
		binary.BigEndian.PutUint64(data[:swarm.SpanSize], swarm.SectionSize)
		copy(data[swarm.SpanSize:swarm.SpanSize+swarm.SectionSize], c3.Address().Bytes())
		c := swarm.NewChunk(c3.Address(), data)
		sch, err := soc.NewChunk(id, c, signer)
		if err != nil {
			t.Fatal(err)
		}
		maxPayload := append(sch.Address().Bytes(), sch.Data()...)
		msg, err := trojan.NewMessage(testTopic, maxPayload)
		if err != nil {
			t.Fatal(err)
		}

		// create the chunk repair handler
		repairHandler := recovery.NewRepairHandler(mockStorer, logger, pushSyncService)

		// invoke the chunk repair handler
		repairHandler(context.Background(), &msg)

		if receiptError == nil {
			t.Fatal("pushsync did not generate a receipt error")
		}
	})
}

// newTestNetStore creates a test store with a set RemoteGet func.
func newTestNetStore(t *testing.T, recoveryFunc recovery.RecoveryHook) (storage.Storer, *storemock.MockStorer) {
	t.Helper()
	storer := mock.NewStorer()
	logger := logging.New(ioutil.Discard, 5)

	mockStorer := storemock.NewStorer()
	serverMockAccounting := accountingmock.NewAccounting()
	price := uint64(12345)
	pricerMock := accountingmock.NewPricer(price, price)
	peerID := swarm.MustParseHexAddress("deadbeef")
	ps := mockPeerSuggester{eachPeerRevFunc: func(f topology.EachPeerFunc) error {
		_, _, _ = f(peerID, 0)
		return nil
	}}
	server := retrieval.New(swarm.ZeroAddress, nil, nil, logger, serverMockAccounting, nil, nil, nil)
	server.SetStorer(mockStorer)
	recorder := streamtest.New(
		streamtest.WithProtocols(server.Protocol()),
	)
	retrieve := retrieval.New(swarm.ZeroAddress, recorder, ps, logger, serverMockAccounting, pricerMock, nil, nil)
	retrieve.SetStorer(mockStorer)
	ns := netstore.New(storer, recoveryFunc, retrieve, logger, nil)
	netstore.SetTimeout(250 * time.Millisecond)
	return ns, mockStorer
}

type mockPeerSuggester struct {
	eachPeerRevFunc func(f topology.EachPeerFunc) error
}

func (s mockPeerSuggester) EachPeer(topology.EachPeerFunc) error {
	return errors.New("not implemented")
}
func (s mockPeerSuggester) EachPeerRev(f topology.EachPeerFunc) error {
	return s.eachPeerRevFunc(f)
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
