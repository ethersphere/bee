// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap_test

import (
	"context"
	"errors"
	"io/ioutil"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	mockp2p "github.com/ethersphere/bee/pkg/p2p/mock"
	"github.com/ethersphere/bee/pkg/settlement/swap"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	mockchequebook "github.com/ethersphere/bee/pkg/settlement/swap/chequebook/mock"
	mockchequestore "github.com/ethersphere/bee/pkg/settlement/swap/chequestore/mock"
	"github.com/ethersphere/bee/pkg/settlement/swap/swapprotocol"
	mockstore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

type swapProtocolMock struct {
	emitCheque func(context.Context, swarm.Address, common.Address, *big.Int, swapprotocol.IssueFunc) (*big.Int, error)
}

func (m *swapProtocolMock) EmitCheque(ctx context.Context, peer swarm.Address, beneficiary common.Address, value *big.Int, issueFunc swapprotocol.IssueFunc) (*big.Int, error) {
	if m.emitCheque != nil {
		return m.emitCheque(ctx, peer, beneficiary, value, issueFunc)
	}
	return nil, errors.New("not implemented")
}

type testObserver struct {
	receivedCalled chan notifyPaymentReceivedCall
	sentCalled     chan notifyPaymentSentCall
}

type notifyPaymentReceivedCall struct {
	peer   swarm.Address
	amount *big.Int
}

type notifyPaymentSentCall struct {
	peer   swarm.Address
	amount *big.Int
	err    error
}

func newTestObserver() *testObserver {
	return &testObserver{
		receivedCalled: make(chan notifyPaymentReceivedCall, 1),
		sentCalled:     make(chan notifyPaymentSentCall, 1),
	}
}

func (t *testObserver) PeerDebt(peer swarm.Address) (*big.Int, error) {
	return nil, nil
}

func (t *testObserver) NotifyPaymentReceived(peer swarm.Address, amount *big.Int) error {
	t.receivedCalled <- notifyPaymentReceivedCall{
		peer:   peer,
		amount: amount,
	}
	return nil
}

func (t *testObserver) NotifyRefreshmentReceived(peer swarm.Address, amount *big.Int) error {
	return nil
}

func (t *testObserver) NotifyPaymentSent(peer swarm.Address, amount *big.Int, err error) {
	t.sentCalled <- notifyPaymentSentCall{
		peer:   peer,
		amount: amount,
		err:    err,
	}
}

type addressbookMock struct {
	beneficiary      func(peer swarm.Address) (beneficiary common.Address, known bool, err error)
	chequebook       func(peer swarm.Address) (chequebookAddress common.Address, known bool, err error)
	beneficiaryPeer  func(beneficiary common.Address) (peer swarm.Address, known bool, err error)
	chequebookPeer   func(chequebook common.Address) (peer swarm.Address, known bool, err error)
	putBeneficiary   func(peer swarm.Address, beneficiary common.Address) error
	putChequebook    func(peer swarm.Address, chequebook common.Address) error
	addDeductionFor  func(peer swarm.Address) error
	addDeductionBy   func(peer swarm.Address) error
	getDeductionFor  func(peer swarm.Address) (bool, error)
	getDeductionBy   func(peer swarm.Address) (bool, error)
	peerDeductionFor map[string]struct{}
}

func (m *addressbookMock) Beneficiary(peer swarm.Address) (beneficiary common.Address, known bool, err error) {
	return m.beneficiary(peer)
}
func (m *addressbookMock) Chequebook(peer swarm.Address) (chequebookAddress common.Address, known bool, err error) {
	return m.chequebook(peer)
}
func (m *addressbookMock) BeneficiaryPeer(beneficiary common.Address) (peer swarm.Address, known bool, err error) {
	return m.beneficiaryPeer(beneficiary)
}
func (m *addressbookMock) ChequebookPeer(chequebook common.Address) (peer swarm.Address, known bool, err error) {
	return m.chequebookPeer(chequebook)
}
func (m *addressbookMock) PutBeneficiary(peer swarm.Address, beneficiary common.Address) error {
	return m.putBeneficiary(peer, beneficiary)
}
func (m *addressbookMock) PutChequebook(peer swarm.Address, chequebook common.Address) error {
	return m.putChequebook(peer, chequebook)
}
func (m *addressbookMock) AddDeductionFor(peer swarm.Address) error {
	m.peerDeductionFor[peer.String()] = struct{}{}
	return m.addDeductionFor(peer)
}
func (m *addressbookMock) AddDeductionBy(peer swarm.Address) error {
	return m.addDeductionBy(peer)
}
func (m *addressbookMock) GetDeductionFor(peer swarm.Address) (bool, error) {
	return m.getDeductionFor(peer)
}
func (m *addressbookMock) GetDeductionBy(peer swarm.Address) (bool, error) {
	return m.getDeductionBy(peer)
}

type cashoutMock struct {
	cashCheque    func(ctx context.Context, chequebook common.Address, recipient common.Address) (common.Hash, error)
	cashoutStatus func(ctx context.Context, chequebookAddress common.Address) (*chequebook.CashoutStatus, error)
}

func (m *cashoutMock) CashCheque(ctx context.Context, chequebook, recipient common.Address) (common.Hash, error) {
	return m.cashCheque(ctx, chequebook, recipient)
}
func (m *cashoutMock) CashoutStatus(ctx context.Context, chequebookAddress common.Address) (*chequebook.CashoutStatus, error) {
	return m.cashoutStatus(ctx, chequebookAddress)
}

func TestReceiveCheque(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()
	chequebookService := mockchequebook.NewChequebook()
	amount := big.NewInt(50)
	exchange := big.NewInt(10)
	deduction := big.NewInt(10)
	chequebookAddress := common.HexToAddress("0xcd")

	peer := swarm.MustParseHexAddress("abcd")
	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      common.HexToAddress("0xab"),
			CumulativePayout: big.NewInt(10),
			Chequebook:       chequebookAddress,
		},
		Signature: []byte{},
	}

	chequeStore := mockchequestore.NewChequeStore(
		mockchequestore.WithReceiveChequeFunc(func(ctx context.Context, c *chequebook.SignedCheque, e *big.Int, d *big.Int) (*big.Int, error) {
			if !cheque.Equal(c) {
				t.Fatalf("passed wrong cheque to store. wanted %v, got %v", cheque, c)
			}
			if exchange.Cmp(e) != 0 {
				t.Fatalf("passed wrong exchange rate to store. wanted %v, got %v", exchange, e)
			}
			if deduction.Cmp(e) != 0 {
				t.Fatalf("passed wrong deduction to store. wanted %v, got %v", deduction, d)
			}
			return amount, nil
		}),
	)
	networkID := uint64(1)
	addressbook := &addressbookMock{
		peerDeductionFor: make(map[string]struct{}),
		chequebook: func(p swarm.Address) (common.Address, bool, error) {
			if !peer.Equal(p) {
				t.Fatal("querying chequebook for wrong peer")
			}
			return chequebookAddress, true, nil
		},
		putChequebook: func(p swarm.Address, chequebook common.Address) (err error) {
			if !peer.Equal(p) {
				t.Fatal("storing chequebook for wrong peer")
			}
			if chequebook != chequebookAddress {
				t.Fatal("storing wrong chequebook")
			}
			return nil
		},
		addDeductionFor: func(p swarm.Address) error {
			if !peer.Equal(p) {
				t.Fatal("storing deduction for wrong peer")
			}
			return nil
		},
	}

	observer := newTestObserver()

	swap := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		chequebookService,
		chequeStore,
		addressbook,
		networkID,
		&cashoutMock{},
		mockp2p.New(),
		observer,
	)

	err := swap.ReceiveCheque(context.Background(), peer, cheque, exchange, deduction)
	if err != nil {
		t.Fatal(err)
	}

	expectedAmount := big.NewInt(4)

	select {
	case call := <-observer.receivedCalled:
		if call.amount.Cmp(expectedAmount) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, expectedAmount)
		}

		if !call.peer.Equal(peer) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peer)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	if _, ok := addressbook.peerDeductionFor[peer.String()]; !ok {
		t.Fatal("add deduction for peer not called")
	}

}

func TestReceiveChequeReject(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()
	chequebookService := mockchequebook.NewChequebook()
	chequebookAddress := common.HexToAddress("0xcd")
	exchange := big.NewInt(10)
	deduction := big.NewInt(10)

	peer := swarm.MustParseHexAddress("abcd")
	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      common.HexToAddress("0xab"),
			CumulativePayout: big.NewInt(10),
			Chequebook:       chequebookAddress,
		},
		Signature: []byte{},
	}

	var errReject = errors.New("reject")

	chequeStore := mockchequestore.NewChequeStore(
		mockchequestore.WithReceiveChequeFunc(func(ctx context.Context, c *chequebook.SignedCheque, e *big.Int, d *big.Int) (*big.Int, error) {
			return nil, errReject
		}),
	)
	networkID := uint64(1)
	addressbook := &addressbookMock{
		chequebook: func(p swarm.Address) (common.Address, bool, error) {
			return chequebookAddress, true, nil
		},
	}

	observer := newTestObserver()

	swap := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		chequebookService,
		chequeStore,
		addressbook,
		networkID,
		&cashoutMock{},
		mockp2p.New(),
		observer,
	)

	err := swap.ReceiveCheque(context.Background(), peer, cheque, exchange, deduction)
	if err == nil {
		t.Fatal("accepted invalid cheque")
	}
	if !errors.Is(err, errReject) {
		t.Fatalf("wrong error. wanted %v, got %v", errReject, err)
	}

	select {
	case <-observer.receivedCalled:
		t.Fatalf("observer called by error.")
	default:
	}

}

func TestReceiveChequeWrongChequebook(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()
	chequebookService := mockchequebook.NewChequebook()
	chequebookAddress := common.HexToAddress("0xcd")
	exchange := big.NewInt(10)
	deduction := big.NewInt(10)

	peer := swarm.MustParseHexAddress("abcd")
	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      common.HexToAddress("0xab"),
			CumulativePayout: big.NewInt(10),
			Chequebook:       chequebookAddress,
		},
		Signature: []byte{},
	}

	chequeStore := mockchequestore.NewChequeStore()
	networkID := uint64(1)
	addressbook := &addressbookMock{
		chequebook: func(p swarm.Address) (common.Address, bool, error) {
			return common.HexToAddress("0xcfff"), true, nil
		},
	}

	observer := newTestObserver()
	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		chequebookService,
		chequeStore,
		addressbook,
		networkID,
		&cashoutMock{},
		mockp2p.New(),
		observer,
	)

	err := swapService.ReceiveCheque(context.Background(), peer, cheque, exchange, deduction)
	if err == nil {
		t.Fatal("accepted invalid cheque")
	}
	if !errors.Is(err, swap.ErrWrongChequebook) {
		t.Fatalf("wrong error. wanted %v, got %v", swap.ErrWrongChequebook, err)
	}

	select {
	case <-observer.receivedCalled:
		t.Fatalf("observer called by error.")
	default:
	}

}

func TestPay(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	amount := big.NewInt(50)
	beneficiary := common.HexToAddress("0xcd")
	peer := swarm.MustParseHexAddress("abcd")

	networkID := uint64(1)
	addressbook := &addressbookMock{
		beneficiary: func(p swarm.Address) (common.Address, bool, error) {
			if !peer.Equal(p) {
				t.Fatal("querying beneficiary for wrong peer")
			}
			return beneficiary, true, nil
		},
	}

	observer := newTestObserver()

	var emitCalled bool
	swap := swap.New(
		&swapProtocolMock{
			emitCheque: func(ctx context.Context, p swarm.Address, b common.Address, a *big.Int, issueFunc swapprotocol.IssueFunc) (*big.Int, error) {
				if !peer.Equal(p) {
					t.Fatal("sending to wrong peer")
				}
				if b != beneficiary {
					t.Fatal("issuing for wrong beneficiary")
				}
				if amount.Cmp(a) != 0 {
					t.Fatal("issuing with wrong amount")
				}
				emitCalled = true
				return amount, nil
			},
		},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		addressbook,
		networkID,
		&cashoutMock{},
		mockp2p.New(),
		observer,
	)

	swap.Pay(context.Background(), peer, amount)

	if !emitCalled {
		t.Fatal("swap protocol was not called")
	}
}

func TestPayIssueError(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	amount := big.NewInt(50)
	beneficiary := common.HexToAddress("0xcd")

	peer := swarm.MustParseHexAddress("abcd")
	errReject := errors.New("reject")
	networkID := uint64(1)
	addressbook := &addressbookMock{
		beneficiary: func(p swarm.Address) (common.Address, bool, error) {
			if !peer.Equal(p) {
				t.Fatal("querying beneficiary for wrong peer")
			}
			return beneficiary, true, nil
		},
	}

	swap := swap.New(
		&swapProtocolMock{
			emitCheque: func(c context.Context, a1 swarm.Address, a2 common.Address, i *big.Int, issueFunc swapprotocol.IssueFunc) (*big.Int, error) {
				return nil, errReject
			},
		},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		addressbook,
		networkID,
		&cashoutMock{},
		mockp2p.New(),
		nil,
	)

	observer := newTestObserver()
	swap.SetAccounting(observer)

	swap.Pay(context.Background(), peer, amount)
	select {
	case call := <-observer.sentCalled:

		if !call.peer.Equal(peer) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peer)
		}
		if !errors.Is(call.err, errReject) {
			t.Fatalf("wrong error. wanted %v, got %v", errReject, call.err)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

}

func TestPayUnknownBeneficiary(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	amount := big.NewInt(50)
	peer := swarm.MustParseHexAddress("abcd")
	networkID := uint64(1)
	addressbook := &addressbookMock{
		beneficiary: func(p swarm.Address) (common.Address, bool, error) {
			if !peer.Equal(p) {
				t.Fatal("querying beneficiary for wrong peer")
			}
			return common.Address{}, false, nil
		},
	}

	observer := newTestObserver()

	var disconnectCalled bool
	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		addressbook,
		networkID,
		&cashoutMock{},
		mockp2p.New(
			mockp2p.WithDisconnectFunc(func(disconnectPeer swarm.Address) error {
				if !peer.Equal(disconnectPeer) {
					t.Fatalf("disconnecting wrong peer. wanted %v, got %v", peer, disconnectPeer)
				}
				disconnectCalled = true
				return nil
			}),
		),
		observer,
	)

	swapService.Pay(context.Background(), peer, amount)

	select {
	case call := <-observer.sentCalled:

		if !call.peer.Equal(peer) {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peer)
		}
		if !errors.Is(call.err, swap.ErrUnknownBeneficary) {
			t.Fatalf("wrong error. wanted %v, got %v", swap.ErrUnknownBeneficary, call.err)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

	if !disconnectCalled {
		t.Fatal("disconnect was not called")
	}
}

func TestHandshake(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	beneficiary := common.HexToAddress("0xcd")
	networkID := uint64(1)
	peer := crypto.NewOverlayFromEthereumAddress(beneficiary[:], networkID)

	var putCalled bool
	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		&addressbookMock{
			beneficiary: func(p swarm.Address) (common.Address, bool, error) {
				return beneficiary, true, nil
			},
			putBeneficiary: func(p swarm.Address, b common.Address) error {
				putCalled = true
				return nil
			},
		},
		networkID,
		&cashoutMock{},
		mockp2p.New(),
		nil,
	)

	err := swapService.Handshake(peer, beneficiary)
	if err != nil {
		t.Fatal(err)
	}

	if putCalled {
		t.Fatal("beneficiary was saved again")
	}
}

func TestHandshakeNewPeer(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	beneficiary := common.HexToAddress("0xcd")
	networkID := uint64(1)
	peer := crypto.NewOverlayFromEthereumAddress(beneficiary[:], networkID)

	var putCalled bool
	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		&addressbookMock{
			beneficiary: func(p swarm.Address) (common.Address, bool, error) {
				return beneficiary, false, nil
			},
			putBeneficiary: func(p swarm.Address, b common.Address) error {
				putCalled = true
				return nil
			},
		},
		networkID,
		&cashoutMock{},
		mockp2p.New(),
		nil,
	)

	err := swapService.Handshake(peer, beneficiary)
	if err != nil {
		t.Fatal(err)
	}

	if !putCalled {
		t.Fatal("beneficiary was not saved")
	}
}

func TestHandshakeWrongBeneficiary(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	beneficiary := common.HexToAddress("0xcd")
	peer := swarm.MustParseHexAddress("abcd")
	networkID := uint64(1)

	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		&addressbookMock{},
		networkID,
		&cashoutMock{},
		mockp2p.New(),
		nil,
	)

	err := swapService.Handshake(peer, beneficiary)
	if !errors.Is(err, swap.ErrWrongBeneficiary) {
		t.Fatalf("wrong error. wanted %v, got %v", swap.ErrWrongBeneficiary, err)
	}
}

func TestCashout(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	theirChequebookAddress := common.HexToAddress("ffff")
	ourChequebookAddress := common.HexToAddress("fffa")
	peer := swarm.MustParseHexAddress("abcd")
	txHash := common.HexToHash("eeee")
	addressbook := &addressbookMock{
		chequebook: func(p swarm.Address) (common.Address, bool, error) {
			if !peer.Equal(p) {
				t.Fatal("querying chequebook for wrong peer")
			}
			return theirChequebookAddress, true, nil
		},
	}

	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		mockchequebook.NewChequebook(
			mockchequebook.WithChequebookAddressFunc(func() common.Address {
				return ourChequebookAddress
			}),
		),
		mockchequestore.NewChequeStore(),
		addressbook,
		uint64(1),
		&cashoutMock{
			cashCheque: func(ctx context.Context, c common.Address, r common.Address) (common.Hash, error) {
				if c != theirChequebookAddress {
					t.Fatalf("not cashing with the right chequebook. wanted %v, got %v", theirChequebookAddress, c)
				}
				if r != ourChequebookAddress {
					t.Fatalf("not cashing with the right recipient. wanted %v, got %v", ourChequebookAddress, r)
				}
				return txHash, nil
			},
		},
		mockp2p.New(),
		nil,
	)

	returnedHash, err := swapService.CashCheque(context.Background(), peer)
	if err != nil {
		t.Fatal(err)
	}

	if returnedHash != txHash {
		t.Fatalf("go wrong tx hash. wanted %v, got %v", txHash, returnedHash)
	}
}

func TestCashoutStatus(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	theirChequebookAddress := common.HexToAddress("ffff")
	peer := swarm.MustParseHexAddress("abcd")
	addressbook := &addressbookMock{
		chequebook: func(p swarm.Address) (common.Address, bool, error) {
			if !peer.Equal(p) {
				t.Fatal("querying chequebook for wrong peer")
			}
			return theirChequebookAddress, true, nil
		},
	}

	expectedStatus := &chequebook.CashoutStatus{}

	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		addressbook,
		uint64(1),
		&cashoutMock{
			cashoutStatus: func(ctx context.Context, c common.Address) (*chequebook.CashoutStatus, error) {
				if c != theirChequebookAddress {
					t.Fatalf("getting status for wrong chequebook. wanted %v, got %v", theirChequebookAddress, c)
				}
				return expectedStatus, nil
			},
		},
		mockp2p.New(),
		nil,
	)

	returnedStatus, err := swapService.CashoutStatus(context.Background(), peer)
	if err != nil {
		t.Fatal(err)
	}

	if expectedStatus != returnedStatus {
		t.Fatalf("go wrong status. wanted %v, got %v", expectedStatus, returnedStatus)
	}
}
