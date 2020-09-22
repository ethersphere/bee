package chequebook

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/storage"
)

var (
	// ErrNoCheque is the error returned if there is no prior cheque for a chequebook or beneficiary.
	ErrNoCheque = errors.New("no cheque")
	// ErrChequeNotIncreasing is the error returned if the cheque amount is the same or lower.
	ErrChequeNotIncreasing = errors.New("cheque cumulativePayout is not increasing")
	// ErrChequeInvalid is the error returned if the cheque itself is invalid.
	ErrChequeInvalid = errors.New("invalid cheque")
	// ErrWrongBeneficiary is the error returned if the cheque has the wrong beneficiary.
	ErrWrongBeneficiary = errors.New("wrong beneficiary")
	// ErrBouncingCheque is the error returned if the chequebook is demonstrably illiquid.
	ErrBouncingCheque = errors.New("bouncing cheque")
)

// ChequeStore handles the verification and storage of received cheques
type ChequeStore interface {
	// ReceiveCheque verifies and stores a cheque. It returns the totam amount earned.
	ReceiveCheque(ctx context.Context, cheque *SignedCheque) (*big.Int, error)
	// LastCheque returns the last cheque we received from a specific chequebook.
	LastCheque(chequebook common.Address) (*SignedCheque, error)
}

type chequeStore struct {
	store                 storage.StateStorer
	factory               Factory
	chaindID              int64
	simpleSwapBindingFunc SimpleSwapBindingFunc
	backend               Backend
	beneficiary           common.Address // the beneficiary we expect in cheques sent to us
}

// NewChequeStore creates new ChequeStore
func NewChequeStore(store storage.StateStorer, backend Backend, factory Factory, chainID int64, beneficiary common.Address, simpleSwapBindingFunc SimpleSwapBindingFunc) ChequeStore {
	return &chequeStore{
		store:                 store,
		factory:               factory,
		backend:               backend,
		chaindID:              chainID,
		simpleSwapBindingFunc: simpleSwapBindingFunc,
		beneficiary:           beneficiary,
	}
}

// lastReceivedChequeKey computes the key where to store the last cheque received from a chequebook.
func lastReceivedChequeKey(chequebook common.Address) string {
	return fmt.Sprintf("chequebook_last_received_cheque_%x", chequebook)
}

// LastCheque returns the last cheque we received from a specific chequebook.
func (s *chequeStore) LastCheque(chequebook common.Address) (*SignedCheque, error) {
	var cheque *SignedCheque
	err := s.store.Get(lastReceivedChequeKey(chequebook), &cheque)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}
		return nil, ErrNoCheque
	}

	return cheque, nil
}

// ReceiveCheque verifies and stores a cheque. It returns the totam amount earned.
func (s *chequeStore) ReceiveCheque(ctx context.Context, cheque *SignedCheque) (*big.Int, error) {
	// verify we are the beneficiary
	if cheque.Beneficiary != s.beneficiary {
		return nil, ErrWrongBeneficiary
	}

	// load the lastCumulativePayout for the cheques chequebook
	var lastCumulativePayout *big.Int
	var lastReceivedCheque *SignedCheque
	err := s.store.Get(lastReceivedChequeKey(cheque.Chequebook), &lastReceivedCheque)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}

		// if this is the first cheque from this chequebook, verify with the factory.
		err = s.factory.VerifyChequebook(ctx, cheque.Chequebook)
		if err != nil {
			return nil, err
		}

		lastCumulativePayout = big.NewInt(0)
	} else {
		lastCumulativePayout = lastReceivedCheque.CumulativePayout
	}

	// check this cheque is actually increasing in value
	amount := big.NewInt(0).Sub(cheque.CumulativePayout, lastCumulativePayout)

	if amount.Cmp(big.NewInt(0)) <= 0 {
		return nil, ErrChequeNotIncreasing
	}

	// blockchain calls below

	binding, err := s.simpleSwapBindingFunc(cheque.Chequebook, s.backend)
	if err != nil {
		return nil, err
	}

	// this does not change for the same chequebook
	expectedIssuer, err := binding.Issuer(&bind.CallOpts{
		Context: ctx,
	})
	if err != nil {
		return nil, err
	}

	// verify the cheque signature
	err = s.verifyChequeSignature(ctx, cheque, expectedIssuer)
	if err != nil {
		return nil, err
	}

	// basic liquidity check
	// could be omitted as it is not particularly useful
	balance, err := binding.Balance(&bind.CallOpts{
		Context: ctx,
	})
	if err != nil {
		return nil, err
	}

	if balance.Cmp(cheque.CumulativePayout) < 0 {
		return nil, ErrBouncingCheque
	}

	// store the accepted cheque
	err = s.store.Put(lastReceivedChequeKey(cheque.Chequebook), cheque)
	if err != nil {
		return nil, err
	}

	return amount, nil
}

// verifyChequeSignature recovers the signer and checks it against the onchain issuer
func (s *chequeStore) verifyChequeSignature(ctx context.Context, cheque *SignedCheque, expectedIssuer common.Address) error {
	eip712Data := eip712DataForCheque(&cheque.Cheque, s.chaindID)

	pubkey, err := crypto.RecoverEIP712(cheque.Signature, eip712Data)
	if err != nil {
		return err
	}

	ethAddr, err := crypto.NewEthereumAddress(*pubkey)
	if err != nil {
		return err
	}

	var issuer common.Address
	copy(issuer[:], ethAddr)

	if issuer != expectedIssuer {
		return ErrChequeInvalid
	}

	return nil
}
