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

const NonceLength = 32

// Address represents the bzz address in swarm.
// It consists of a peers underlay (physical) address, overlay (topology) address and signature.
// Signature is used to verify the `Overlay/Underlay` pair, as it is based on
// `underlay|networkID|nonce|timestamp|chequebook`, signed with the public key
// of Overlay address.
type Address struct {
	Underlays         []ma.Multiaddr
	Overlay           swarm.Address
	Signature         []byte
	Nonce             []byte
	Timestamp         int64
	EthereumAddress   []byte
	ChequebookAddress common.Address
}

type addressJSON struct {
	Overlay           string   `json:"overlay"`
	Underlays         []string `json:"underlays"`
	Signature         string   `json:"signature"`
	Nonce             string   `json:"nonce"`
	Timestamp         int64    `json:"timestamp"`
	ChequebookAddress string   `json:"chequebook,omitempty"`
}

func NewAddress(signer crypto.Signer, underlays []ma.Multiaddr, overlay swarm.Address, networkID uint64, nonce []byte, timestamp int64, chequebookAddress common.Address) (*Address, error) {
	if len(nonce) != NonceLength {
		return nil, ErrInvalidAddress
	}

	if timestamp <= 0 {
		return nil, ErrTimestampInvalid
	}

	underlaysBinary, err := SerializeUnderlays(underlays)
	if err != nil {
		return nil, fmt.Errorf("serialize underlays: %w", err)
	}

	signature, err := signer.Sign(generateSignData(underlaysBinary, overlay.Bytes(), networkID, nonce, timestamp, chequebookAddress.Bytes()))
	if err != nil {
		return nil, err
	}

	return &Address{
		Underlays:         underlays,
		Overlay:           overlay,
		Signature:         signature,
		Nonce:             nonce,
		Timestamp:         timestamp,
		ChequebookAddress: chequebookAddress,
	}, nil
}

// ParseAddress validates a wire-encoded BzzAddress. The chequebook bytes
// must be either empty or exactly common.AddressLength — other lengths
// would be silently truncated or left-padded by common.BytesToAddress.
func ParseAddress(underlay, overlay, signature, nonce []byte, timestamp int64, networkID uint64, chequebookAddress []byte) (*Address, error) {
	if len(nonce) != NonceLength {
		return nil, ErrInvalidAddress
	}

	if len(chequebookAddress) != 0 && len(chequebookAddress) != common.AddressLength {
		return nil, ErrInvalidAddress
	}

	if timestamp <= 0 {
		return nil, ErrTimestampInvalid
	}

	cb := common.BytesToAddress(chequebookAddress)

	recoveredPK, err := crypto.Recover(signature, generateSignData(underlay, overlay, networkID, nonce, timestamp, cb.Bytes()))
	if err != nil {
		return nil, ErrInvalidAddress
	}

	recoveredOverlay, err := crypto.NewOverlayAddress(*recoveredPK, networkID, nonce)
	if err != nil {
		return nil, ErrInvalidAddress
	}
	if !bytes.Equal(recoveredOverlay.Bytes(), overlay) {
		return nil, ErrInvalidAddress
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
		Underlays:         multiUnderlays,
		Overlay:           swarm.NewAddress(overlay),
		Signature:         signature,
		Nonce:             nonce,
		Timestamp:         timestamp,
		EthereumAddress:   ethAddress,
		ChequebookAddress: cb,
	}, nil
}

func generateSignData(underlay, overlay []byte, networkID uint64, nonce []byte, timestamp int64, chequebookAddress []byte) []byte {
	const prefix = "bee-handshake-"
	networkIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(networkIDBytes, networkID)
	timestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes, uint64(timestamp))
	signData := make([]byte, 0,
		len(prefix)+
			len(underlay)+
			len(overlay)+
			len(networkIDBytes)+
			len(nonce)+
			len(timestampBytes)+
			len(chequebookAddress),
	)
	signData = append(signData, prefix...)
	signData = append(signData, underlay...)
	signData = append(signData, overlay...)
	signData = append(signData, networkIDBytes...)
	signData = append(signData, nonce...)
	signData = append(signData, timestampBytes...)
	return append(signData, chequebookAddress...)
}

func (a *Address) Equal(b *Address) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.Overlay.Equal(b.Overlay) &&
		AreUnderlaysEqual(a.Underlays, b.Underlays) &&
		bytes.Equal(a.Signature, b.Signature) &&
		bytes.Equal(a.Nonce, b.Nonce) &&
		a.Timestamp == b.Timestamp &&
		a.ChequebookAddress == b.ChequebookAddress
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
	if len(a.Underlays) == 0 {
		return nil, fmt.Errorf("no underlays for %s", a.Overlay)
	}

	chequebook := ""
	if a.ChequebookAddress != (common.Address{}) {
		chequebook = common.Bytes2Hex(a.ChequebookAddress.Bytes())
	}

	return json.Marshal(&addressJSON{
		Overlay:           a.Overlay.String(),
		Underlays:         a.underlaysAsStrings(),
		Signature:         base64.StdEncoding.EncodeToString(a.Signature),
		Nonce:             common.Bytes2Hex(a.Nonce),
		Timestamp:         a.Timestamp,
		ChequebookAddress: chequebook,
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

	multiaddrs, err := parseMultiaddrs(v.Underlays)
	if err != nil {
		return err
	}

	a.Underlays = multiaddrs
	a.Signature, err = base64.StdEncoding.DecodeString(v.Signature)
	if err != nil {
		return err
	}

	a.Nonce = common.Hex2Bytes(v.Nonce)
	a.Timestamp = v.Timestamp
	if v.ChequebookAddress != "" {
		a.ChequebookAddress = common.HexToAddress(v.ChequebookAddress)
	}

	return nil
}

func (a *Address) String() string {
	return fmt.Sprintf("[Underlay: %v, Overlay %v, Signature %x, Nonce %x, Timestamp %d, Chequebook %x]", a.underlaysAsStrings(), a.Overlay, a.Signature, a.Nonce, a.Timestamp, a.ChequebookAddress)
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
