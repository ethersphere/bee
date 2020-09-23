// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/crypto/eip712"
)

// Cheque represents a cheque for a SimpleSwap chequebook
type Cheque struct {
	Chequebook       common.Address
	Beneficiary      common.Address
	CumulativePayout *big.Int
}

// SignedCheque represents a cheque together with its signature
type SignedCheque struct {
	Cheque
	Signature []byte
}

// chequebookDomain computes chainId-dependant EIP712 domain
func chequebookDomain(chainID int64) eip712.TypedDataDomain {
	return eip712.TypedDataDomain{
		Name:    "Chequebook",
		Version: "1.0",
		ChainId: math.NewHexOrDecimal256(chainID),
	}
}

// ChequeTypes are the needed type descriptions for cheque signing
var ChequeTypes = eip712.Types{
	"EIP712Domain": eip712.EIP712DomainType,
	"Cheque": []eip712.Type{
		{
			Name: "chequebook",
			Type: "address",
		},
		{
			Name: "beneficiary",
			Type: "address",
		},
		{
			Name: "cumulativePayout",
			Type: "uint256",
		},
	},
}

// ChequeSigner signs cheque
type ChequeSigner interface {
	// Sign signs a cheque
	Sign(cheque *Cheque) ([]byte, error)
}

type chequeSigner struct {
	signer  crypto.Signer // the underlying signer used
	chainID int64         // the chainID used for EIP712
}

// NewChequeSigner creates a new cheque signer for the given chainID.
func NewChequeSigner(signer crypto.Signer, chainID int64) ChequeSigner {
	return &chequeSigner{
		signer:  signer,
		chainID: chainID,
	}
}

// eip712DataForCheque converts a cheque into the correct TypedData structure.
func eip712DataForCheque(cheque *Cheque, chainID int64) *eip712.TypedData {
	return &eip712.TypedData{
		Domain: chequebookDomain(chainID),
		Types:  ChequeTypes,
		Message: eip712.TypedDataMessage{
			"chequebook":       cheque.Chequebook.Hex(),
			"beneficiary":      cheque.Beneficiary.Hex(),
			"cumulativePayout": cheque.CumulativePayout.String(),
		},
		PrimaryType: "Cheque",
	}
}

// Sign signs a cheque.
func (s *chequeSigner) Sign(cheque *Cheque) ([]byte, error) {
	return s.signer.SignTypedData(eip712DataForCheque(cheque, s.chainID))
}

func (cheque *Cheque) String() string {
	return fmt.Sprintf("Contract: %x Beneficiary: %x CumulativePayout: %v", cheque.Chequebook, cheque.Beneficiary, cheque.CumulativePayout)
}

func (cheque *Cheque) Equal(other *Cheque) bool {
	if cheque.Beneficiary != other.Beneficiary {
		return false
	}
	if cheque.CumulativePayout.Cmp(other.CumulativePayout) != 0 {
		return false
	}
	return cheque.Chequebook == other.Chequebook
}

func (cheque *SignedCheque) Equal(other *SignedCheque) bool {
	if !bytes.Equal(cheque.Signature, other.Signature) {
		return false
	}
	return cheque.Cheque.Equal(&other.Cheque)
}
