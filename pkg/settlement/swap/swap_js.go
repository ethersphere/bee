//go:build js
// +build js

package swap

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/settlement"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/swapprotocol"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Service is the implementation of the swap settlement layer.
type Service struct {
	proto          swapprotocol.Interface
	logger         log.Logger
	store          storage.StateStorer
	accounting     settlement.Accounting
	chequebook     chequebook.Service
	chequeStore    chequebook.ChequeStore
	cashout        chequebook.CashoutService
	addressbook    Addressbook
	networkID      uint64
	cashoutAddress common.Address
}

// New creates a new swap Service.
func New(proto swapprotocol.Interface, logger log.Logger, store storage.StateStorer, chequebook chequebook.Service, chequeStore chequebook.ChequeStore, addressbook Addressbook, networkID uint64, cashout chequebook.CashoutService, accounting settlement.Accounting, cashoutAddress common.Address) *Service {
	return &Service{
		proto:          proto,
		logger:         logger.WithName(loggerName).Register(),
		store:          store,
		chequebook:     chequebook,
		chequeStore:    chequeStore,
		addressbook:    addressbook,
		networkID:      networkID,
		cashout:        cashout,
		accounting:     accounting,
		cashoutAddress: cashoutAddress,
	}
}

// ReceiveCheque is called by the swap protocol if a cheque is received.
func (s *Service) ReceiveCheque(ctx context.Context, peer swarm.Address, cheque *chequebook.SignedCheque, exchangeRate, deduction *big.Int) (err error) {
	// check this is the same chequebook for this peer as previously
	expectedChequebook, known, err := s.addressbook.Chequebook(peer)
	if err != nil {
		return err
	}
	if known && expectedChequebook != cheque.Chequebook {
		return ErrWrongChequebook
	}

	receivedAmount, err := s.chequeStore.ReceiveCheque(ctx, cheque, exchangeRate, deduction)
	if err != nil {
		return fmt.Errorf("rejecting cheque: %w", err)
	}

	if deduction.Cmp(big.NewInt(0)) > 0 {
		err = s.addressbook.AddDeductionFor(peer)
		if err != nil {
			return err
		}
	}

	decreasedAmount := new(big.Int).Sub(receivedAmount, deduction)
	amount := new(big.Int).Div(decreasedAmount, exchangeRate)

	if !known {
		err = s.addressbook.PutChequebook(peer, cheque.Chequebook)
		if err != nil {
			return err
		}
	}

	return s.accounting.NotifyPaymentReceived(peer, amount)
}

// Pay initiates a payment to the given peer
func (s *Service) Pay(ctx context.Context, peer swarm.Address, amount *big.Int) {
	var err error
	defer func() {
		if err != nil {
			s.accounting.NotifyPaymentSent(peer, amount, err)
		}
	}()
	if s.chequebook == nil {
		err = ErrNoChequebook
		return
	}
	beneficiary, known, err := s.addressbook.Beneficiary(peer)
	if err != nil {
		return
	}
	if !known {
		err = ErrUnknownBeneficary
		return
	}

	_, err = s.proto.EmitCheque(ctx, peer, beneficiary, amount, s.chequebook.Issue)

	if err != nil {
		return
	}

	s.accounting.NotifyPaymentSent(peer, amount, nil)
}
