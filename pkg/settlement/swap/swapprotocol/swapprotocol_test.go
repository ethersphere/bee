// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swapprotocol_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	swapmock "github.com/ethersphere/bee/v2/pkg/settlement/swap/mock"
	priceoraclemock "github.com/ethersphere/bee/v2/pkg/settlement/swap/priceoracle/mock"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/swapprotocol"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/swapprotocol/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestEmitCheques(t *testing.T) {
	t.Parallel()

	// Test negotiating / sending cheques

	logger := log.Noop
	commonAddr := common.HexToAddress("0xab")
	peerID := swarm.MustParseHexAddress("9ee7add7")
	swapReceiver := swapmock.NewSwap()
	swapInitiator := swapmock.NewSwap()

	// mocked exchange rate and deduction
	priceOracle := priceoraclemock.New(big.NewInt(50), big.NewInt(500))

	swappReceiver := swapprotocol.New(nil, logger, commonAddr, priceOracle)
	swappReceiver.SetSwap(swapReceiver)
	recorder := streamtest.New(
		streamtest.WithProtocols(swappReceiver.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)
	commonAddr2 := common.HexToAddress("0xdc")
	swappInitiator := swapprotocol.New(recorder, logger, commonAddr2, priceOracle)
	swappInitiator.SetSwap(swapInitiator)
	peer := p2p.Peer{Address: peerID}

	// amount in accounting credits cheque should cover
	chequeAmount := big.NewInt(1250)

	issueFunc := func(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) (*big.Int, error) {
		cheque := &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary: commonAddr,
				// CumulativePayout only contains value of last cheque
				CumulativePayout: amount,
				Chequebook:       common.Address{},
			},
			Signature: []byte{},
		}
		_ = sendChequeFunc(cheque)
		return big.NewInt(13750), nil
	}

	// in this case we try to send a cheque covering amount * exchange rate + initial deduction

	if _, err := swappInitiator.EmitCheque(context.Background(), peer.Address, commonAddr, chequeAmount, issueFunc); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	records, err := recorder.Records(peerID, "swap", "1.0.0", "swap")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}
	record := records[0]
	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.EmitCheque) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("got %v messages, want %v", len(messages), 1)
	}

	gotCheque := messages[0].(*pb.EmitCheque)

	var gotSignedCheque *chequebook.SignedCheque
	err = json.Unmarshal(gotCheque.Cheque, &gotSignedCheque)
	if err != nil {
		t.Fatal(err)
	}

	// cumulative payout expected to be 50 * 1250 + 500

	if gotSignedCheque.CumulativePayout.Cmp(big.NewInt(63000)) != 0 {
		t.Fatalf("Unexpected cheque amount, expected %v, got %v", 63000, gotSignedCheque.CumulativePayout)
	}

	// attempt to send second cheque covering same amount, this time deduction is not applicable

	if _, err := swappInitiator.EmitCheque(context.Background(), peer.Address, commonAddr, chequeAmount, issueFunc); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	records, err = recorder.Records(peerID, "swap", "1.0.0", "swap")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 2 {
		t.Fatalf("got %v records, want %v", l, 2)
	}
	record = records[1]
	messages, err = protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.EmitCheque) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("got %v messages, want %v", len(messages), 1)
	}

	gotCheque = messages[0].(*pb.EmitCheque)

	err = json.Unmarshal(gotCheque.Cheque, &gotSignedCheque)
	if err != nil {
		t.Fatal(err)
	}

	// cumulative payout only contains last cheque value in this case because of mocks
	// cumulative payout expected to be 50 * 1250 + 0

	if gotSignedCheque.CumulativePayout.Cmp(big.NewInt(62500)) != 0 {
		t.Fatalf("Unexpected cheque amount, expected %v, got %v", 62500, gotSignedCheque.CumulativePayout)
	}
}

func TestCantEmitChequeRateMismatch(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	commonAddr := common.HexToAddress("0xab")
	peerID := swarm.MustParseHexAddress("9ee7add7")
	swapReceiver := swapmock.NewSwap()
	swapInitiator := swapmock.NewSwap()

	// mock different information for the receiver and sender received from oracle

	priceOracle := priceoraclemock.New(big.NewInt(50), big.NewInt(500))
	priceOracle2 := priceoraclemock.New(big.NewInt(52), big.NewInt(560))
	swappReceiver := swapprotocol.New(nil, logger, commonAddr, priceOracle)
	swappReceiver.SetSwap(swapReceiver)
	recorder := streamtest.New(
		streamtest.WithProtocols(swappReceiver.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)
	commonAddr2 := common.HexToAddress("0xdc")
	swappInitiator := swapprotocol.New(recorder, logger, commonAddr2, priceOracle2)
	swappInitiator.SetSwap(swapInitiator)
	peer := p2p.Peer{Address: peerID}

	chequeAmount := big.NewInt(1250)

	issueFunc := func(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) (*big.Int, error) {
		cheque := &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      commonAddr,
				CumulativePayout: amount,
				Chequebook:       common.Address{},
			},
			Signature: []byte{},
		}
		_ = sendChequeFunc(cheque)
		return big.NewInt(13750), nil
	}

	// try to send cheque, this should fail because of the different known exchange rates on the sender and receiver
	// expect rate negotiation error

	if _, err := swappInitiator.EmitCheque(context.Background(), peer.Address, commonAddr, chequeAmount, issueFunc); !errors.Is(err, swapprotocol.ErrNegotiateRate) {
		t.Fatalf("expected error %v, got %v", swapprotocol.ErrNegotiateRate, err)
	}
	records, err := recorder.Records(peerID, "swap", "1.0.0", "swap")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}
	record := records[0]
	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.EmitCheque) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("got %v messages, want %v", len(messages), 0)
	}
}

func TestCantEmitChequeDeductionMismatch(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	commonAddr := common.HexToAddress("0xab")
	peerID := swarm.MustParseHexAddress("9ee7add7")
	swapReceiver := swapmock.NewSwap()
	swapInitiator := swapmock.NewSwap()

	// mock different deduction values for receiver and sender received from the oracle

	priceOracle := priceoraclemock.New(big.NewInt(50), big.NewInt(500))
	priceOracle2 := priceoraclemock.New(big.NewInt(50), big.NewInt(560))
	swappReceiver := swapprotocol.New(nil, logger, commonAddr, priceOracle)
	swappReceiver.SetSwap(swapReceiver)
	recorder := streamtest.New(
		streamtest.WithProtocols(swappReceiver.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)
	commonAddr2 := common.HexToAddress("0xdc")
	swappInitiator := swapprotocol.New(recorder, logger, commonAddr2, priceOracle2)
	swappInitiator.SetSwap(swapInitiator)
	peer := p2p.Peer{Address: peerID}

	chequeAmount := big.NewInt(1250)

	issueFunc := func(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) (*big.Int, error) {
		cheque := &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      commonAddr,
				CumulativePayout: amount,
				Chequebook:       common.Address{},
			},
			Signature: []byte{},
		}
		_ = sendChequeFunc(cheque)
		return big.NewInt(13750), nil
	}

	// attempt negotiating rates, expect deduction negotiation error

	if _, err := swappInitiator.EmitCheque(context.Background(), peer.Address, commonAddr, chequeAmount, issueFunc); !errors.Is(err, swapprotocol.ErrNegotiateDeduction) {
		t.Fatalf("expected error %v, got %v", swapprotocol.ErrNegotiateDeduction, err)
	}

	records, err := recorder.Records(peerID, "swap", "1.0.0", "swap")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}
	record := records[0]
	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.EmitCheque) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("got %v messages, want %v", len(messages), 0)
	}
}

func TestCantEmitChequeIneligibleDeduction(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	commonAddr := common.HexToAddress("0xab")
	peerID := swarm.MustParseHexAddress("9ee7add7")
	swapReceiver := swapmock.NewSwap()
	swapInitiator := swapmock.NewSwap()

	// mock exactly the same rates for exchange and deduction for receiver and sender

	priceOracle := priceoraclemock.New(big.NewInt(50), big.NewInt(500))
	priceOracle2 := priceoraclemock.New(big.NewInt(50), big.NewInt(500))
	swappReceiver := swapprotocol.New(nil, logger, commonAddr, priceOracle)
	swappReceiver.SetSwap(swapReceiver)
	recorder := streamtest.New(
		streamtest.WithProtocols(swappReceiver.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)
	commonAddr2 := common.HexToAddress("0xdc")
	swappInitiator := swapprotocol.New(recorder, logger, commonAddr2, priceOracle2)
	swappInitiator.SetSwap(swapInitiator)
	peer := p2p.Peer{Address: peerID}

	chequeAmount := big.NewInt(1250)

	issueFunc := func(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) (*big.Int, error) {
		cheque := &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      commonAddr,
				CumulativePayout: amount,
				Chequebook:       common.Address{},
			},
			Signature: []byte{},
		}
		_ = sendChequeFunc(cheque)
		return big.NewInt(13750), nil
	}

	// mock that the initiator believes deduction was applied previously, but the accepting peer does not

	err := swapInitiator.AddDeductionByPeer(peerID)
	if err != nil {
		t.Fatal(err)
	}

	// attempt settling anyway, expect it to fail by ineligible deduction error

	if _, err := swappInitiator.EmitCheque(context.Background(), peer.Address, commonAddr, chequeAmount, issueFunc); !errors.Is(err, swapprotocol.ErrHaveDeduction) {
		t.Fatalf("expected error %v, got %v", swapprotocol.ErrHaveDeduction, err)
	}

	records, err := recorder.Records(peerID, "swap", "1.0.0", "swap")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}
	record := records[0]
	messages, err := protobuf.ReadMessages(
		bytes.NewReader(record.In()),
		func() protobuf.Message { return new(pb.EmitCheque) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("got %v messages, want %v", len(messages), 0)
	}
}

// TestHandlerRejectsNilCheque is a regression test for NIL-06: a peer-supplied
// cheque whose body is the JSON literal null unmarshals to a nil
// *SignedCheque with a nil error. The handler must reject it before it reaches
// ReceiveCheque, whose chequestore path dereferences cheque.Beneficiary
// unconditionally and would otherwise crash the process.
func TestHandlerRejectsNilCheque(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	commonAddr := common.HexToAddress("0xab")
	peerID := swarm.MustParseHexAddress("9ee7add7")

	var received atomic.Bool
	swapReceiver := swapmock.NewSwap(
		swapmock.WithReceiveChequeFunc(func(_ context.Context, _ swarm.Address, cheque *chequebook.SignedCheque, _, _ *big.Int) error {
			received.Store(true)
			// Touch a field the way the real chequestore does; if the nil guard
			// regresses this dereference is what crashes the process.
			_ = cheque.Beneficiary
			return nil
		}),
	)

	priceOracle := priceoraclemock.New(big.NewInt(50), big.NewInt(500))
	swapp := swapprotocol.New(nil, logger, commonAddr, priceOracle)
	swapp.SetSwap(swapReceiver)

	recorder := streamtest.New(
		streamtest.WithProtocols(swapp.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	stream, err := recorder.NewStream(context.Background(), peerID, nil, "swap", "1.0.0", "swap")
	if err != nil {
		t.Fatal(err)
	}

	w := protobuf.NewWriter(stream)
	if err := w.WriteMsgWithContext(context.Background(), &pb.EmitCheque{Cheque: []byte("null")}); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(peerID, "swap", "1.0.0", "swap")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}

	if err := records[0].Err(); !errors.Is(err, swapprotocol.ErrNilCheque) {
		t.Fatalf("expected error %v, got %v", swapprotocol.ErrNilCheque, err)
	}

	if received.Load() {
		t.Fatal("nil cheque reached ReceiveCheque; guard did not hold")
	}
}

// TestHandlerRejectsNilCumulativePayout covers a peer-supplied cheque that
// omits cumulativePayout: it unmarshals into a non-nil cheque whose
// CumulativePayout is a nil *big.Int, which the chequestore passes to
// big.Int.Sub. The handler must reject it before it gets there.
func TestHandlerRejectsNilCumulativePayout(t *testing.T) {
	t.Parallel()

	logger := log.Noop
	commonAddr := common.HexToAddress("0xab")
	peerID := swarm.MustParseHexAddress("9ee7add7")

	var received atomic.Bool
	swapReceiver := swapmock.NewSwap(
		swapmock.WithReceiveChequeFunc(func(_ context.Context, _ swarm.Address, cheque *chequebook.SignedCheque, _, _ *big.Int) error {
			received.Store(true)
			// The way the real chequestore uses it; if the guard regresses this
			// nil dereference is what crashes the process.
			_ = big.NewInt(0).Sub(cheque.CumulativePayout, big.NewInt(0))
			return nil
		}),
	)

	priceOracle := priceoraclemock.New(big.NewInt(50), big.NewInt(500))
	swapp := swapprotocol.New(nil, logger, commonAddr, priceOracle)
	swapp.SetSwap(swapReceiver)

	recorder := streamtest.New(
		streamtest.WithProtocols(swapp.Protocol()),
		streamtest.WithBaseAddr(peerID),
	)

	stream, err := recorder.NewStream(context.Background(), peerID, nil, "swap", "1.0.0", "swap")
	if err != nil {
		t.Fatal(err)
	}

	cheque := []byte(`{"beneficiary":"0x00000000000000000000000000000000000000be","chequebook":"0x00000000000000000000000000000000000000cb","signature":"AAA="}`)

	w := protobuf.NewWriter(stream)
	if err := w.WriteMsgWithContext(context.Background(), &pb.EmitCheque{Cheque: cheque}); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(peerID, "swap", "1.0.0", "swap")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want %v", l, 1)
	}

	if err := records[0].Err(); !errors.Is(err, swapprotocol.ErrNilCumulativePayout) {
		t.Fatalf("expected error %v, got %v", swapprotocol.ErrNilCumulativePayout, err)
	}

	if received.Load() {
		t.Fatal("cheque with nil cumulative payout reached ReceiveCheque; guard did not hold")
	}
}
