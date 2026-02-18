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
	"slices"

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
	Underlays       []ma.Multiaddr
	Overlay         swarm.Address
	Signature       []byte
	Nonce           []byte
	EthereumAddress []byte
}

type addressJSON struct {
	Overlay   string   `json:"overlay"`
	Underlay  string   `json:"underlay"` // For backward compatibility
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
		Underlays: underlays,
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

	// Empty underlays are allowed for inbound-only peers (e.g., browsers, WebRTC, strict NAT)
	// that cannot be dialed back. These peers can still use protocols over existing connections
	// but won't participate in Kademlia topology or hive gossip.

	ethAddress, err := crypto.NewEthereumAddress(*recoveredPK)
	if err != nil {
		return nil, fmt.Errorf("extract blockchain address: %w: %w", err, ErrInvalidAddress)
	}

	return &Address{
		Underlays:       multiUnderlays,
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

	return a.Overlay.Equal(b.Overlay) && AreUnderlaysEqual(a.Underlays, b.Underlays) && bytes.Equal(a.Signature, b.Signature) && bytes.Equal(a.Nonce, b.Nonce)
}

func AreUnderlaysEqual(a, b []ma.Multiaddr) bool {
	if len(a) != len(b) {
		return false
	}

	used := make([]bool, len(b))
	for i := range len(a) {
		found := false
		for j := range len(b) {
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
	// Empty underlays are allowed for inbound-only peers that cannot be dialed back
	// select the underlay address for backward compatibility
	var underlay string
	if v := SelectBestAdvertisedAddress(a.Underlays, nil); v != nil {
		underlay = v.String()
	}

	return json.Marshal(&addressJSON{
		Overlay:   a.Overlay.String(),
		Underlay:  underlay,
		Underlays: a.underlaysAsStrings(),
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

	// append the underlay for backward compatibility
	if !slices.Contains(v.Underlays, v.Underlay) {
		v.Underlays = append(v.Underlays, v.Underlay)
	}

	multiaddrs, err := parseMultiaddrs(v.Underlays)
	if err != nil {
		return err
	}

	a.Underlays = multiaddrs
	a.Signature, err = base64.StdEncoding.DecodeString(v.Signature)
	a.Nonce = common.Hex2Bytes(v.Nonce)
	return err
}

func (a *Address) String() string {
	return fmt.Sprintf("[Underlay: %v, Overlay %v, Signature %x, Transaction %x]", a.underlaysAsStrings(), a.Overlay, a.Signature, a.Nonce)
}

// ShortString returns shortened versions of bzz address in a format: [Overlay, Underlay]
// It can be used for logging
func (a *Address) ShortString() string {
	return fmt.Sprintf("[Overlay: %s, Underlays: %v]", a.Overlay.String(), a.underlaysAsStrings())
}

func (a *Address) underlaysAsStrings() []string {
	underlays := make([]string, len(a.Underlays))
	for i, underlay := range a.Underlays {
		underlays[i] = underlay.String()
	}
	return underlays
}

func parseMultiaddrs(addrs []string) ([]ma.Multiaddr, error) {
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
