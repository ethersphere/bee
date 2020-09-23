package swap

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/crypto"
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
	// ErrWrongBeneficiary is the error if a peer uses a different beneficiary than expected.
	ErrWrongBeneficiary = errors.New("wrong beneficiary")
	// ErrUnknownBeneficary is the error if a peer has never announced a beneficiary.
	ErrUnknownBeneficary = errors.New("unknown beneficiary for peer")
)

// Service is the implementation of the swap settlement layer.
type Service struct {
	proto       swapprotocol.Interface
	logger      logging.Logger
	store       storage.StateStorer
	observer    settlement.PaymentObserver
	metrics     metrics
	chequebook  chequebook.Service
	chequeStore chequebook.ChequeStore
	addressbook Addressbook
	networkID   uint64
}

// New creates a new swap Service.
func New(proto swapprotocol.Interface, logger logging.Logger, store storage.StateStorer, chequebook chequebook.Service, chequeStore chequebook.ChequeStore, addressbook Addressbook, networkID uint64) *Service {
	return &Service{
		proto:       proto,
		logger:      logger,
		store:       store,
		metrics:     newMetrics(),
		chequebook:  chequebook,
		chequeStore: chequeStore,
		addressbook: addressbook,
		networkID:   networkID,
	}
}

// ReceiveCheque is called by the swap protocol if a cheque is received.
func (s *Service) ReceiveCheque(ctx context.Context, peer swarm.Address, cheque *chequebook.SignedCheque) (err error) {
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

	s.metrics.TotalReceived.Add(float64(amount.Uint64()))

	return s.observer.NotifyPayment(peer, amount.Uint64())
}

// Pay initiates a payment to the given peer
func (s *Service) Pay(ctx context.Context, peer swarm.Address, amount uint64) error {
	beneficiary, known, err := s.addressbook.Beneficiary(peer)
	if err != nil {
		return err
	}
	if !known {
		return ErrUnknownBeneficary
	}
	err = s.chequebook.Issue(beneficiary, big.NewInt(int64(amount)), func(signedCheque *chequebook.SignedCheque) error {
		return s.proto.EmitCheque(ctx, peer, signedCheque)
	})
	if err != nil {
		return err
	}
	s.metrics.TotalSent.Add(float64(amount))
	return nil
}

// SetPaymentObserver sets the payment observer which will be notified of incoming payments
func (s *Service) SetPaymentObserver(observer settlement.PaymentObserver) {
	s.observer = observer
}

// TotalSent returns the total amount sent to a peer
func (s *Service) TotalSent(peer swarm.Address) (totalSent uint64, err error) {
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
func (s *Service) TotalReceived(peer swarm.Address) (totalReceived uint64, err error) {
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
func (s *Service) SettlementsSent() (map[string]uint64, error) {
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
func (s *Service) SettlementsReceived() (map[string]uint64, error) {
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
func (s *Service) Handshake(peer swarm.Address, beneficiary common.Address) error {
	// check that the overlay address was derived from the beneficiary (implying they have the same private key)
	// while this is not strictly necessary for correct functionality we need to ensure no two peers use the same beneficiary
	// as long as we enforce this we might not need the handshake message if the p2p layer exposed the overlay public key
	expectedOverlay := crypto.NewOverlayFromEthereumAddress(beneficiary[:], s.networkID)
	if !expectedOverlay.Equal(peer) {
		return ErrWrongBeneficiary
	}

	storedBeneficiary, known, err := s.addressbook.Beneficiary(peer)
	if err != nil {
		return err
	}
	if !known {
		s.logger.Tracef("initial swap handshake peer: %v beneficiary: %x", peer, beneficiary)
		return s.addressbook.PutBeneficiary(peer, beneficiary)
	}
	if storedBeneficiary != beneficiary {
		return ErrWrongBeneficiary
	}
	return nil
}
