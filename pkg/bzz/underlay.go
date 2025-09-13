// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-varint"
)

// underlayListPrefix is a magic byte designated for identifying a serialized list of multiaddrs.
// A value of 0x99 (153) was chosen as it is not a defined multiaddr protocol code.
// This ensures that a failure is triggered by the original multiaddr.NewMultiaddrBytes function,
// which expects a valid protocol code at the start of the data.
const underlayListPrefix byte = 0x99

// SerializeUnderlays serializes a slice of multiaddrs into a single byte slice.
// If the slice contains exactly one address, the standard, backward-compatible
// multiaddr format is used. For zero or more than one address, a custom list format
// prefixed with a magic byte is utilized.
func SerializeUnderlays(addrs []multiaddr.Multiaddr) []byte {
	// Backward compatibility if exactly one address is present.
	if len(addrs) == 1 {
		return addrs[0].Bytes()
	}

	// For 0 or 2+ addresses, the custom list format with the prefix is used.
	// The format is: [prefix_byte][varint_len_1][addr_1_bytes]...
	var buf bytes.Buffer
	buf.WriteByte(underlayListPrefix)

	for _, addr := range addrs {
		addrBytes := addr.Bytes()
		buf.Write(varint.ToUvarint(uint64(len(addrBytes))))
		buf.Write(addrBytes)
	}
	return buf.Bytes()
}

// DeserializeUnderlays deserializes a byte slice into a slice of multiaddrs.
// The data format is automatically detected as either a single legacy multiaddr
// or a list of multiaddrs (identified by underlayListPrefix), and is parsed accordingly.
func DeserializeUnderlays(data []byte) ([]multiaddr.Multiaddr, error) {
	if len(data) == 0 {
		return nil, errors.New("cannot deserialize empty byte slice")
	}

	// If the data begins with the magic prefix, it is handled as a list.
	if data[0] == underlayListPrefix {
		return deserializeList(data[1:])
	}

	// Otherwise, the data is handled as a single, backward-compatible multiaddr.
	addr, err := multiaddr.NewMultiaddrBytes(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse as single multiaddr: %w", err)
	}
	// The result is returned as a single-element slice for a consistent return type.
	return []multiaddr.Multiaddr{addr}, nil
}

// deserializeList handles the parsing of the custom list format.
// The provided data is expected to have already been stripped of the underlayListPrefix.
func deserializeList(data []byte) ([]multiaddr.Multiaddr, error) {
	var addrs []multiaddr.Multiaddr
	r := bytes.NewReader(data)

	for r.Len() > 0 {
		// The varint-encoded length of the next address is read.
		addrLen, err := varint.ReadUvarint(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read address length from list: %w", err)
		}

		// A sanity check is performed to ensure enough bytes remain for the declared length.
		if uint64(r.Len()) < addrLen {
			return nil, fmt.Errorf("inconsistent data: expected %d bytes for address, but only %d remain", addrLen, r.Len())
		}

		// The individual address bytes are read and parsed.
		addrBytes := make([]byte, addrLen)
		if _, err := r.Read(addrBytes); err != nil {
			return nil, fmt.Errorf("failed to read address bytes: %w", err)
		}

		addr, err := multiaddr.NewMultiaddrBytes(addrBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse multiaddr from list: %w", err)
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}
