package swap

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/swapprotocol"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	// ErrWrongChequebook is the error if a peer uses a different chequebook from before.
	ErrWrongChequebook = errors.New("wrong chequebook")
	// ErrUnknownBeneficary is the error if a peer has never announced a beneficiary.
	ErrUnknownBeneficary = errors.New("unknown beneficiary for peer")
)

// service is the implementation of the swap settlement layer.
type service struct {
	proto       swapprotocol.Interface
	logger      logging.Logger
	store       storage.StateStorer
	observer    settlement.PaymentObserver
	metrics     metrics
	chequebook  chequebook.Service
	chequeStore chequebook.ChequeStore
	addressbook Addressbook
}

// New creates a new swap service.
func New(proto swapprotocol.Interface, logger logging.Logger, store storage.StateStorer, chequebook chequebook.Service, chequeStore chequebook.ChequeStore, addressbook Addressbook) *service {
	return &service{
		proto:       proto,
		logger:      logger,
		store:       store,
		metrics:     newMetrics(),
		chequebook:  chequebook,
		chequeStore: chequeStore,
		addressbook: addressbook,
	}
}

// ReceiveCheque is called by the swap protocol if a cheque is received.
func (s *service) ReceiveCheque(ctx context.Context, peer swarm.Address, cheque *chequebook.SignedCheque) (err error) {
	// check this is the same chequebook for this peer as previously
	expectedChequebook, known, err := s.addressbook.Chequebook(peer)
	if err != nil {
		return err
	}
	if known {
		if expectedChequebook != cheque.Chequebook {
			return ErrWrongChequebook
		}
	}

	amount, err := s.chequeStore.ReceiveCheque(ctx, cheque)
	if err != nil {
		return err
	}

	if !known {
		err = s.addressbook.PutChequebook(peer, cheque.Chequebook)
		if err != nil {
			return err
		}
	}

	return s.observer.NotifyPayment(peer, amount.Uint64())
}

// Pay initiates a payment to the given peer
func (s *service) Pay(ctx context.Context, peer swarm.Address, amount uint64) error {
	beneficiary, known, err := s.addressbook.Beneficiary(peer)
	if err != nil {
		return err
	}
	if !known {
		return ErrUnknownBeneficary
	}
	return s.chequebook.Issue(beneficiary, big.NewInt(int64(amount)), func(signedCheque *chequebook.SignedCheque) error {
		return s.proto.EmitCheque(ctx, peer, signedCheque)
	})
}

// SetPaymentObserver sets the payment observer which will be notified of incoming payments
func (s *service) SetPaymentObserver(observer settlement.PaymentObserver) {
	s.observer = observer
}

// TotalSent returns the total amount sent to a peer
func (s *service) TotalSent(peer swarm.Address) (totalSent uint64, err error) {
	beneficiary, known, err := s.addressbook.Beneficiary(peer)
	if err != nil {
		return 0, err
	}
	if !known {
		return 0, settlement.ErrPeerNoSettlements
	}
	cheque, err := s.chequebook.LastCheque(beneficiary)
	if err != nil {
		if err == chequebook.ErrNoCheque {
			return 0, settlement.ErrPeerNoSettlements
		}
		return 0, err
	}
	return cheque.CumulativePayout.Uint64(), nil
}

// TotalReceived returns the total amount received from a peer
func (s *service) TotalReceived(peer swarm.Address) (totalReceived uint64, err error) {
	chequebookAddress, known, err := s.addressbook.Chequebook(peer)
	if err != nil {
		return 0, err
	}
	if !known {
		return 0, settlement.ErrPeerNoSettlements
	}

	cheque, err := s.chequeStore.LastCheque(chequebookAddress)
	if err != nil {
		if err == chequebook.ErrNoCheque {
			return 0, settlement.ErrPeerNoSettlements
		}
		return 0, err
	}
	return cheque.CumulativePayout.Uint64(), nil
}

// SettlementsSent returns sent settlements for each individual known peer
func (s *service) SettlementsSent() (map[string]uint64, error) {
	result := make(map[string]uint64)
	cheques, err := s.chequebook.LastCheques()
	if err != nil {
		return nil, err
	}

	for beneficiary, cheque := range cheques {
		peer, known, err := s.addressbook.BeneficiaryPeer(beneficiary)
		if err != nil {
			return nil, err
		}
		if !known {
			continue
		}
		result[peer.String()] = cheque.CumulativePayout.Uint64()
	}

	return result, nil
}

// SettlementsReceived returns received settlements for each individual known peer.
func (s *service) SettlementsReceived() (map[string]uint64, error) {
	result := make(map[string]uint64)
	err := s.store.Iterate(peerPrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := keyPeer(key, peerPrefix)
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}

		if _, ok := result[addr.String()]; !ok {
			totalReceived, err := s.TotalReceived(addr)
			if err != nil && err != settlement.ErrPeerNoSettlements {
				return false, err
			} else if err == settlement.ErrPeerNoSettlements {
				return false, nil
			}

			result[addr.String()] = totalReceived
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Handshake is called by the swap protocol when a handshake is received.
func (s *service) Handshake(peer swarm.Address, beneficiary common.Address) error {
	storedBeneficiary, known, err := s.addressbook.Beneficiary(peer)
	if err != nil {
		return err
	}
	if known {
		if storedBeneficiary != beneficiary {
			return errors.New("beneficiary changed")
		}
		return nil
	}
	s.logger.Tracef("swap handshake peer=%v beneficiary=%x", peer, beneficiary)
	return s.addressbook.PutBeneficiary(peer, beneficiary)
}
