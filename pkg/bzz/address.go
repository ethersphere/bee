// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bzz exposes the data structure and operations
// necessary on the bzz.Address type which used in the handshake
// protocol, address-book and hive protocol.
package bzz

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/swarm"

	ma "github.com/multiformats/go-multiaddr"
)

var ErrInvalidAddress = errors.New("invalid address")

// Address represents the bzz address in swarm.
// It consists of a peers underlay (physical) address, overlay (topology) address and signature.
// Signature is used to verify the `Overlay/Underlay` pair, as it is based on `underlay|networkID`, signed with the public key of Overlay address
type Address struct {
	Underlay        ma.Multiaddr
	Overlay         swarm.Address
	Signature       []byte
	Nonce           []byte
	EthereumAddress []byte
}

type addressJSON struct {
	Overlay   string `json:"overlay"`
	Underlay  string `json:"underlay"`
	Signature string `json:"signature"`
	Nonce     string `json:"transaction"`
}

func NewAddress(signer crypto.Signer, underlay ma.Multiaddr, overlay swarm.Address, networkID uint64, nonce []byte) (*Address, error) {
	underlayBinary, err := underlay.MarshalBinary()
	if err != nil {
		return nil, err
	}

	signature, err := signer.Sign(generateSignData(underlayBinary, overlay.Bytes(), networkID))
	if err != nil {
		return nil, err
	}

	return &Address{
		Underlay:  underlay,
		Overlay:   overlay,
		Signature: signature,
		Nonce:     nonce,
	}, nil
}

func ParseAddress(underlay, overlay, signature, nonce []byte, validateOverlay bool, networkID uint64) (*Address, error) {
	recoveredPK, err := crypto.Recover(signature, generateSignData(underlay, overlay, networkID))
	if err != nil {
		return nil, ErrInvalidAddress
	}

	if validateOverlay {
		recoveredOverlay, err := crypto.NewOverlayAddress(*recoveredPK, networkID, nonce)
		if err != nil {
			return nil, ErrInvalidAddress
		}
		if !bytes.Equal(recoveredOverlay.Bytes(), overlay) {
			return nil, ErrInvalidAddress
		}
	}

	multiUnderlay, err := ma.NewMultiaddrBytes(underlay)
	if err != nil {
		return nil, ErrInvalidAddress
	}

	ethAddress, err := crypto.NewEthereumAddress(*recoveredPK)
	if err != nil {
		return nil, fmt.Errorf("extract blockchain address: %w: %w", err, ErrInvalidAddress)
	}

	return &Address{
		Underlay:        multiUnderlay,
		Overlay:         swarm.NewAddress(overlay),
		Signature:       signature,
		Nonce:           nonce,
		EthereumAddress: ethAddress,
	}, nil
}

func generateSignData(underlay, overlay []byte, networkID uint64) []byte {
	networkIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(networkIDBytes, networkID)
	signData := append([]byte("bee-handshake-"), underlay...)
	signData = append(signData, overlay...)
	return append(signData, networkIDBytes...)
}

func (a *Address) Equal(b *Address) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.Overlay.Equal(b.Overlay) && multiaddrEqual(a.Underlay, b.Underlay) && bytes.Equal(a.Signature, b.Signature) && bytes.Equal(a.Nonce, b.Nonce)
}

func multiaddrEqual(a, b ma.Multiaddr) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.Equal(b)
}

func (a *Address) MarshalJSON() ([]byte, error) {
	return json.Marshal(&addressJSON{
		Overlay:   a.Overlay.String(),
		Underlay:  a.Underlay.String(),
		Signature: base64.StdEncoding.EncodeToString(a.Signature),
		Nonce:     common.Bytes2Hex(a.Nonce),
	})
}

func (a *Address) UnmarshalJSON(b []byte) error {
	v := &addressJSON{}
	err := json.Unmarshal(b, v)
	if err != nil {
		return err
	}

	addr, err := swarm.ParseHexAddress(v.Overlay)
	if err != nil {
		return err
	}

	a.Overlay = addr

	m, err := ma.NewMultiaddr(v.Underlay)
	if err != nil {
		return err
	}

	a.Underlay = m
	a.Signature, err = base64.StdEncoding.DecodeString(v.Signature)
	a.Nonce = common.Hex2Bytes(v.Nonce)
	return err
}

func (a *Address) String() string {
	return fmt.Sprintf("[Underlay: %v, Overlay %v, Signature %x, Transaction %x]", a.Underlay, a.Overlay, a.Signature, a.Nonce)
}

// ShortString returns shortened versions of bzz address in a format: [Overlay, Underlay]
// It can be used for logging
func (a *Address) ShortString() string {
	return fmt.Sprintf("[Overlay: %s, Underlay: %s]", a.Overlay.String(), a.Underlay.String())
}
