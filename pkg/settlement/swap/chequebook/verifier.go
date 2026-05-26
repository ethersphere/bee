// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

var (
	ErrChequebookIssuerMismatch      = errors.New("chequebook issuer does not match peer ethereum address")
	ErrChequebookBytecodeMismatch    = errors.New("chequebook deployed bytecode hash not in accepted set")
	ErrChequebookInsufficientBalance = errors.New("chequebook token balance below minimum threshold")
	ErrChequebookAlreadyAssociated   = errors.New("chequebook already associated with a different peer")
	ErrChequebookAddressMissing      = errors.New("chequebook address missing from address record")
)

// VerifyErrorLabel returns the metric label for a non-nil Verifier.Verify
// result. Unknown errors map to "verify_error".
func VerifyErrorLabel(err error) string {
	switch {
	case errors.Is(err, ErrChequebookIssuerMismatch):
		return "issuer_mismatch"
	case errors.Is(err, ErrChequebookBytecodeMismatch):
		return "bytecode_mismatch"
	case errors.Is(err, ErrChequebookInsufficientBalance):
		return "insufficient_balance"
	case errors.Is(err, ErrChequebookAlreadyAssociated):
		return "already_associated"
	}
	return "verify_error"
}

// Verifier runs the chequebook checks from the spec: issuer match, deployed
// bytecode hash, minimum balance, and uniqueness. When the addressbook already
// holds an entry for (overlay, chequebook), the issuer and bytecode RPCs are
// skipped — only uniqueness and balance are re-checked. Returns the typed error
// of the first failing check.
type Verifier interface {
	Verify(ctx context.Context, chequebookAddress, peerEthereumAddress common.Address, overlay swarm.Address, verified bool) error
}

// PeerChequebookRegistryLookup is the subset of swap.Addressbook used by the
// uniqueness check.
type PeerChequebookRegistryLookup interface {
	Peer(chequebook common.Address) (peer swarm.Address, known bool)
}

// CodeReader returns the deployed bytecode at a contract address.
// Satisfied by transaction.Backend and ethclient.Client.
type CodeReader interface {
	CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error)
}

type VerifierConfig struct {
	// AcceptedBytecodeHashes is a list so older chequebook versions can
	// remain valid after a master-copy upgrade. Empty disables the check.
	AcceptedBytecodeHashes [][32]byte

	// MinBalance disables the balance check when nil.
	MinBalance *big.Int
}

func NewVerifier(transactionService transaction.Service, codeReader CodeReader, addressbook PeerChequebookRegistryLookup, cfg VerifierConfig) (Verifier, error) {
	if transactionService == nil {
		return nil, errors.New("chequebook verifier: nil transaction service")
	}
	if len(cfg.AcceptedBytecodeHashes) > 0 && codeReader == nil {
		return nil, errors.New("chequebook verifier: nil code reader with accepted bytecode hashes set")
	}
	if cfg.MinBalance != nil && cfg.MinBalance.Sign() < 0 {
		return nil, errors.New("chequebook verifier: negative min balance")
	}
	return &verifier{
		transactionService:     transactionService,
		codeReader:             codeReader,
		peerChequebookRegistry: addressbook,
		acceptedHashes:         cfg.AcceptedBytecodeHashes,
		minBalance:             cfg.MinBalance,
	}, nil
}

type verifier struct {
	transactionService     transaction.Service
	codeReader             CodeReader
	peerChequebookRegistry PeerChequebookRegistryLookup
	acceptedHashes         [][32]byte
	minBalance             *big.Int
}

// Verify checks the peer's claimed chequebook. When verified is true the
// (overlay, chequebook) pair was previously accepted, so issuer and bytecode
// checks are skipped; uniqueness and balance are still re-checked.
func (v *verifier) Verify(ctx context.Context, chequebookAddress, peerEthereumAddress common.Address, overlay swarm.Address, verified bool) error {
	if (chequebookAddress == common.Address{}) {
		return ErrChequebookAddressMissing
	}

	if v.peerChequebookRegistry != nil {
		if existing, known := v.peerChequebookRegistry.Peer(chequebookAddress); known && !existing.Equal(overlay) {
			return ErrChequebookAlreadyAssociated
		}
	}

	if !verified {
		contract := newChequebookContract(chequebookAddress, v.transactionService)

		issuer, err := contract.Issuer(ctx)
		if err != nil {
			return fmt.Errorf("read chequebook issuer: %w", err)
		}
		if issuer.Cmp(peerEthereumAddress) != 0 {
			return ErrChequebookIssuerMismatch
		}

		if len(v.acceptedHashes) > 0 {
			code, err := v.codeReader.CodeAt(ctx, chequebookAddress, nil)
			if err != nil {
				return fmt.Errorf("read chequebook code: %w", err)
			}
			if len(code) == 0 {
				return ErrChequebookBytecodeMismatch
			}

			got := crypto.Keccak256Hash(code)
			match := false
			for _, want := range v.acceptedHashes {
				if bytes.Equal(got[:], want[:]) {
					match = true
					break
				}
			}
			if !match {
				return ErrChequebookBytecodeMismatch
			}
		}
	}

	if v.minBalance != nil {
		contract := newChequebookContract(chequebookAddress, v.transactionService)
		balance, err := contract.Balance(ctx)
		if err != nil {
			return fmt.Errorf("read chequebook balance: %w", err)
		}
		if balance.Cmp(v.minBalance) < 0 {
			return ErrChequebookInsufficientBalance
		}
	}

	return nil
}
