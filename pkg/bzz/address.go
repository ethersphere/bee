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
	Underlay        []ma.Multiaddr
	Overlay         swarm.Address
	Signature       []byte
	Nonce           []byte
	EthereumAddress []byte
}

type addressJSON struct {
	Overlay   string   `json:"overlay"`
	Underlays []string `json:"underlays"`
	Signature string   `json:"signature"`
	Nonce     string   `json:"transaction"`
}

func NewAddress(signer crypto.Signer, underlays []ma.Multiaddr, overlay swarm.Address, networkID uint64, nonce []byte) (*Address, error) {
	underlaysBinary := SerializeUnderlays(underlays)

	signature, err := signer.Sign(generateSignData(underlaysBinary, overlay.Bytes(), networkID))
	if err != nil {
		return nil, err
	}

	return &Address{
		Underlay:  underlays,
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

	multiUnderlays, err := DeserializeUnderlays(underlay)
	if err != nil {
		return nil, fmt.Errorf("deserialize underlays: %w: %w", ErrInvalidAddress, err)
	}

	if len(multiUnderlays) == 0 {
		// no underlays sent
		return nil, ErrInvalidAddress
	}

	ethAddress, err := crypto.NewEthereumAddress(*recoveredPK)
	if err != nil {
		return nil, fmt.Errorf("extract blockchain address: %w: %w", err, ErrInvalidAddress)
	}

	return &Address{
		Underlay:        multiUnderlays,
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

	return a.Overlay.Equal(b.Overlay) && IsUnderlayEqual(a.Underlay, b.Underlay) && bytes.Equal(a.Signature, b.Signature) && bytes.Equal(a.Nonce, b.Nonce)
}

func IsAtLeastOneUnderlayEqual(a, b []ma.Multiaddr) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for i := 0; i < len(a); i++ {
		for j := 0; j < len(b); j++ {
			if a[i] != nil && b[j] != nil && a[i].Equal(b[j]) {
				return true
			}
		}
	}
	return false
}

func IsUnderlayEqual(a, b []ma.Multiaddr) bool {
	if len(a) != len(b) {
		return false
	}

	used := make([]bool, len(b))
	for i := 0; i < len(a); i++ {
		found := false
		for j := 0; j < len(b); j++ {
			if used[j] {
				continue
			}
			if a[i].Equal(b[j]) {
				used[j] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (a *Address) MarshalJSON() ([]byte, error) {
	return json.Marshal(&addressJSON{
		Overlay:   a.Overlay.String(),
		Underlays: a.underlaysToString(),
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

	multiaddrs, err := stringsToMultiAddr(v.Underlays)
	if err != nil {
		return err
	}

	a.Underlay = multiaddrs
	a.Signature, err = base64.StdEncoding.DecodeString(v.Signature)
	a.Nonce = common.Hex2Bytes(v.Nonce)
	return err
}

func (a *Address) String() string {
	return fmt.Sprintf("[Underlay: %v, Overlay %v, Signature %x, Transaction %x]", a.underlaysToString(), a.Overlay, a.Signature, a.Nonce)
}

// ShortString returns shortened versions of bzz address in a format: [Overlay, Underlay]
// It can be used for logging
func (a *Address) ShortString() string {
	return fmt.Sprintf("[Overlay: %s, Underlays: %v]", a.Overlay.String(), a.underlaysToString())
}

func (a *Address) underlaysToString() []string {
	underlays := make([]string, len(a.Underlay))
	for i, underlay := range a.Underlay {
		underlays[i] = underlay.String()
	}
	return underlays
}

func stringsToMultiAddr(addrs []string) ([]ma.Multiaddr, error) {
	multiAddrs := make([]ma.Multiaddr, len(addrs))
	for i, addr := range addrs {
		multiAddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			return nil, err
		}
		multiAddrs[i] = multiAddr
	}
	return multiAddrs, nil
}
