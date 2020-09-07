// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package recovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"testing"
	"time"

	accountingmock "github.com/ethersphere/bee/pkg/accounting/mock"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/netstore"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/pushsync"
	pushsyncmock "github.com/ethersphere/bee/pkg/pushsync/mock"
	"github.com/ethersphere/bee/pkg/recovery"
	"github.com/ethersphere/bee/pkg/retrieval"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	storemock "github.com/ethersphere/bee/pkg/storage/mock"
	chunktesting "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/trojan"
)

// TestRecoveryHook tests that a recovery hook can be created and called.
func TestRecoveryHook(t *testing.T) {
	// test variables needed to be correctly set for any recovery hook to reach the sender func
	chunkAddr := chunktesting.GenerateTestRandomChunk().Address()
	targets := trojan.Targets{[]byte{0xED}}

	//setup the sender
	hookWasCalled := make(chan bool, 1) // channel to check if hook is called
	pssSender := &mockPssSender{
		hookC: hookWasCalled,
	}

	// create recovery hook and call it
	repair := recovery.NewRepair()
	overlayAddress, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	recoveryHook := repair.GetRecoveryHook(pssSender, overlayAddress)
	chunkC := make(chan swarm.Chunk)
	if err := recoveryHook(chunkAddr, targets, chunkC); err != nil {
		t.Fatal(err)
	}
	select {
	case <-hookWasCalled:
		break
	case <-time.After(100 * time.Millisecond):
		t.Fatal("recovery hook was not called")
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
	// generate test chunk, store and publisher
	c := chunktesting.GenerateTestRandomChunk()
	ref := c.Address()
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
			repair := recovery.NewRepair()
			overlayAddress, err := swarm.ParseHexAddress("00112233")
			if err != nil {
				t.Fatal(err)
			}
			recoverFunc := repair.GetRecoveryHook(pssSender, overlayAddress)
			ns, _ := newTestNetStore(t, recoverFunc)

			// fetch test chunk
			_, err = ns.Get(tc.ctx, storage.ModeGetRequest, ref)
			if err != nil && !errors.Is(err, netstore.ErrRecoveryTimeout) && err.Error() != "error decoding prefix string" {
				t.Fatal(err)
			}

			// checks whether the callback is invoked or the test case times out
			select {
			case <-hookWasCalled:
				if !tc.expectsFailure {
					return
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
		c1 := chunktesting.GenerateTestRandomChunk()

		// create a mock storer and put a chunk that will be repaired
		mockStorer := storemock.NewStorer()
		defer mockStorer.Close()
		_, err := mockStorer.Put(context.Background(), storage.ModePutRequest, c1)
		if err != nil {
			t.Fatal(err)
		}

		//setup the sender
		hookWasCalled := make(chan bool, 1) // channel to check if hook is called
		pssSender := &mockPssSender{
			hookC: hookWasCalled,
		}

		// create a mock pushsync service to push the chunk to its destination
		var receipt *pushsync.Receipt
		pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			receipt = &pushsync.Receipt{
				Address: swarm.NewAddress(chunk.Address().Bytes()),
			}
			return receipt, nil
		})

		// create the chunk repair handler
		repair := recovery.NewRepair()
		repairHandler := repair.GetRepairHandler(mockStorer, logger, pushSyncService, pssSender)

		//create a trojan message to trigger the repair of the chunk
		testTopic := trojan.NewTopic("foo")
		var msg trojan.Message
		overlayAddress, err := swarm.ParseHexAddress("00112233")
		if err != nil {
			t.Fatal(err)
		}
		reqMessage := &recovery.RequestRepairMessage{
			DownloaderOverlay:  overlayAddress.String(),
			ReferencesToRepair: []string{c1.Address().String()},
		}
		maxPayload, err := json.Marshal(reqMessage)
		if err != nil {
			t.Fatal(err)
		}
		if msg, err = trojan.NewMessage(testTopic, maxPayload); err != nil {
			t.Fatal(err)
		}

		// invoke the chunk repair handler
		repairHandler(context.Background(), &msg)

		// check if receipt is received
		if receipt == nil {
			t.Fatal("receipt not received")
		}

		if !receipt.Address.Equal(c1.Address()) {
			t.Fatalf("invalid address in receipt: expected %s received %s", c1.Address(), receipt.Address)
		}

		// check if the chunk is sent to the destination through a pss message
		select {
		case <-hookWasCalled:
			break
		case <-time.After(100 * time.Millisecond):
			t.Fatal("repaired chunk not sent to destination")
		}

	})

	t.Run("repair-chunk-not-present", func(t *testing.T) {
		// generate test chunk, store and publisher
		c2 := chunktesting.GenerateTestRandomChunk()

		// create a mock storer
		mockStorer := storemock.NewStorer()
		defer mockStorer.Close()

		// create a mock pushsync service
		pushServiceCalled := false
		pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			pushServiceCalled = true
			return nil, nil
		})

		//setup the sender
		hookWasCalled := make(chan bool, 1) // channel to check if hook is called
		pssSender := &mockPssSender{
			hookC: hookWasCalled,
		}

		// create the chunk repair handler
		repair := recovery.NewRepair()
		repairHandler := repair.GetRepairHandler(mockStorer, logger, pushSyncService, pssSender)

		//create a trojan message to trigger the repair of the chunk
		testTopic := trojan.NewTopic("foo")
		var msg trojan.Message
		overlayAddress, err := swarm.ParseHexAddress("00112233")
		if err != nil {
			t.Fatal(err)
		}
		reqMessage := &recovery.RequestRepairMessage{
			DownloaderOverlay:  overlayAddress.String(),
			ReferencesToRepair: []string{c2.Address().String()},
		}
		maxPayload, err := json.Marshal(reqMessage)
		if err != nil {
			t.Fatal(err)
		}
		msg, err = trojan.NewMessage(testTopic, maxPayload)
		if err != nil {
			t.Fatal(err)
		}

		// invoke the chunk repair handler
		repairHandler(context.Background(), &msg)

		if pushServiceCalled {
			t.Fatal("push service called even if the chunk is not present")
		}
	})

	t.Run("repair-chunk-closest-peer-not-present", func(t *testing.T) {
		// generate test chunk, store and publisher
		c3 := chunktesting.GenerateTestRandomChunk()

		// create a mock storer
		mockStorer := storemock.NewStorer()
		defer mockStorer.Close()
		_, err := mockStorer.Put(context.Background(), storage.ModePutRequest, c3)
		if err != nil {
			t.Fatal(err)
		}

		// create a mock pushsync service
		var receiptError error
		pushSyncService := pushsyncmock.New(func(ctx context.Context, chunk swarm.Chunk) (*pushsync.Receipt, error) {
			receiptError = errors.New("invalid receipt")
			return nil, receiptError
		})

		//setup the sender
		hookWasCalled := make(chan bool, 1) // channel to check if hook is called
		pssSender := &mockPssSender{
			hookC: hookWasCalled,
		}

		// create the chunk repair handler
		repair := recovery.NewRepair()
		repairHandler := repair.GetRepairHandler(mockStorer, logger, pushSyncService, pssSender)

		//create a trojan message to trigger the repair of the chunk
		testTopic := trojan.NewTopic("foo")
		var msg trojan.Message
		overlayAddress, err := swarm.ParseHexAddress("00112233")
		if err != nil {
			t.Fatal(err)
		}
		reqMessage := &recovery.RequestRepairMessage{
			DownloaderOverlay:  overlayAddress.String(),
			ReferencesToRepair: []string{c3.Address().String()},
		}
		maxPayload, err := json.Marshal(reqMessage)
		if err != nil {
			t.Fatal(err)
		}
		msg, err = trojan.NewMessage(testTopic, maxPayload)
		if err != nil {
			t.Fatal(err)
		}

		// invoke the chunk repair handler
		repairHandler(context.Background(), &msg)

		if receiptError == nil {
			t.Fatal("pushsync did not generate a receipt error")
		}
	})
}

func TestRepair_GetRepairResponseHandler(t *testing.T) {
	c := chunktesting.GenerateTestRandomChunk()
	target := "BE"
	targetContext := sctx.SetTargets(context.Background(), target)

	hookWasCalled := make(chan bool, 1) // channel to check if hook is called

	// setup the sender
	pssSender := &mockPssSender{
		hookC: hookWasCalled,
	}
	repair := recovery.NewRepair()
	overlayAddress, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}
	recoverFunc := repair.GetRecoveryHook(pssSender, overlayAddress)

	// send recovery message
	chunkC := make(chan swarm.Chunk, 1)
	targets, err := sctx.GetTargets(targetContext)
	err = recoverFunc(overlayAddress, targets, chunkC)
	if err != nil {
		t.Error(err)
	}

	// simulate the recovery response
	logger := logging.New(ioutil.Discard, 0)
	repairResponseHandler := repair.GetRepairResponseHandler(logger)

	testTopic := trojan.NewTopic("foo")
	msg, err := trojan.NewMessage(testTopic, c.Data())
	if err != nil {
		t.Error(err)
	}

	err = repairResponseHandler(context.Background(), &msg)
	if err != nil {
		t.Error(err)
	}

	ch := <-chunkC
	if !ch.Address().Equal(c.Address()) {
		t.Fatal(err)
	}

	if !bytes.Equal(ch.Data(), c.Data()) {
		t.Fatal(err)
	}

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
	hookC chan bool
}

// Send mocks the pss Send function
func (mp *mockPssSender) Send(ctx context.Context, targets trojan.Targets, topic trojan.Topic, payload []byte) error {
	mp.hookC <- true
	return nil
}
