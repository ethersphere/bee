// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/crypto/eip712"
)

var (
	// ErrNilCumulativePayout is returned when a cheque has no cumulative payout.
	// It does not wrap ErrNoCheque: callers treat that as "no cheque" and may start a new payout at zero.
	ErrNilCumulativePayout = errors.New("nil cumulative payout")
	// ErrNegativeCumulativePayout is returned when a cheque has a negative cumulative payout.
	ErrNegativeCumulativePayout = errors.New("negative cumulative payout")
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

// chequeJSON is the JSON form of Cheque, as stored in the statestore.
// The field names must not change: they are the names used before Cheque had
// its own JSON methods.
type chequeJSON struct {
	Chequebook       common.Address
	Beneficiary      common.Address
	CumulativePayout *big.Int
}

// signedChequeJSON is the JSON form of SignedCheque, as stored in the
// statestore and sent to peers. It is flat, like the default encoding of the
// embedded Cheque, and its field names must not change.
type signedChequeJSON struct {
	Chequebook       common.Address
	Beneficiary      common.Address
	CumulativePayout *big.Int
	Signature        []byte
}

func validateCumulativePayout(cumulativePayout *big.Int) error {
	if cumulativePayout == nil {
		return ErrNilCumulativePayout
	}
	if cumulativePayout.Sign() < 0 {
		return ErrNegativeCumulativePayout
	}
	return nil
}

// MarshalJSON implements json.Marshaler. It rejects a cheque whose cumulative
// payout is nil or negative, so such a cheque is never stored or sent.
func (cheque Cheque) MarshalJSON() ([]byte, error) {
	if err := validateCumulativePayout(cheque.CumulativePayout); err != nil {
		return nil, fmt.Errorf("marshal cheque: %w", err)
	}
	return json.Marshal(chequeJSON(cheque))
}

// UnmarshalJSON implements json.Unmarshaler. It rejects a cheque whose
// cumulative payout is missing, null or negative.
//
// encoding/json does not call UnmarshalJSON when the whole input is the
// literal null and the target is a pointer, so callers decoding into a
// *Cheque must still check for a nil result.
func (cheque *Cheque) UnmarshalJSON(data []byte) error {
	var v chequeJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if err := validateCumulativePayout(v.CumulativePayout); err != nil {
		return fmt.Errorf("unmarshal cheque: %w", err)
	}
	*cheque = Cheque(v)
	return nil
}

// MarshalJSON implements json.Marshaler. SignedCheque must define its own
// method: otherwise the one promoted from the embedded Cheque would be used
// and the signature would be left out.
func (cheque SignedCheque) MarshalJSON() ([]byte, error) {
	if err := validateCumulativePayout(cheque.CumulativePayout); err != nil {
		return nil, fmt.Errorf("marshal signed cheque: %w", err)
	}
	return json.Marshal(signedChequeJSON{
		Chequebook:       cheque.Chequebook,
		Beneficiary:      cheque.Beneficiary,
		CumulativePayout: cheque.CumulativePayout,
		Signature:        cheque.Signature,
	})
}

// UnmarshalJSON implements json.Unmarshaler. It rejects a cheque whose
// cumulative payout is missing, null or negative. As with Cheque, a literal
// null decoded into a *SignedCheque never reaches this method.
func (cheque *SignedCheque) UnmarshalJSON(data []byte) error {
	var v signedChequeJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if err := validateCumulativePayout(v.CumulativePayout); err != nil {
		return fmt.Errorf("unmarshal signed cheque: %w", err)
	}
	*cheque = SignedCheque{
		Chequebook:       v.Chequebook,
		Beneficiary:      v.Beneficiary,
		CumulativePayout: v.CumulativePayout,
		Signature:        v.Signature,
	}
	return nil
}

// chequebookDomain computes chainId-dependent EIP712 domain
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
