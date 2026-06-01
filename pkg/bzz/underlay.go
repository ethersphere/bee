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

var (
	// ErrUnderlayByteSizeExceeded is returned when the serialized underlay data
	// exceeds the maximum allowed byte size.
	ErrUnderlayByteSizeExceeded = errors.New("underlay byte size exceeded")

	// ErrUnderlayCountExceeded is returned when the number of underlay addresses
	// exceeds the maximum allowed per peer.
	ErrUnderlayCountExceeded = errors.New("underlay count exceeded")
)

// underlayListPrefix is a magic byte designated for identifying a serialized list of multiaddrs.
// A value of 0x99 (153) was chosen as it is not a defined multiaddr protocol code.
// This ensures that a failure is triggered by the original multiaddr.NewMultiaddrBytes function,
// which expects a valid protocol code at the start of the data.
const underlayListPrefix byte = 0x99

// maxUnderlaysPerPeer is the maximum number of underlay addresses allowed per peer.
// This prevents abuse where a malicious peer sends a huge number of multiaddrs.
const maxUnderlaysPerPeer = 20

// maxUnderlayBytes is the maximum byte size allowed for a serialized underlay field.
// With maxUnderlaysPerPeer addresses at ~150 bytes each (typical WSS multiaddr),
// expected size is under 1 KB; 2 KB provides ample headroom.
const maxUnderlayBytes = 2048

// SerializeUnderlays serializes a slice of multiaddrs into a single byte slice.
func SerializeUnderlays(addrs []multiaddr.Multiaddr) ([]byte, error) {
	if len(addrs) > maxUnderlaysPerPeer {
		return nil, fmt.Errorf("underlay count %d exceeds maximum of %d: %w", len(addrs), maxUnderlaysPerPeer, ErrUnderlayCountExceeded)
	}

	// The format is: [varint_len_1][addr_1_bytes]...
	var buf bytes.Buffer

	for _, addr := range addrs {
		addrBytes := addr.Bytes()
		buf.Write(varint.ToUvarint(uint64(len(addrBytes))))
		buf.Write(addrBytes)
	}

	result := buf.Bytes()
	if len(result) > maxUnderlayBytes {
		return nil, fmt.Errorf("serialized underlay size %d exceeds maximum of %d bytes: %w", len(result), maxUnderlayBytes, ErrUnderlayByteSizeExceeded)
	}
	return result, nil
}

// DeserializeUnderlays deserializes a byte slice into a slice of multiaddrs.
func DeserializeUnderlays(data []byte) ([]multiaddr.Multiaddr, error) {
	if len(data) == 0 {
		return nil, nil
	}

	if len(data) > maxUnderlayBytes {
		return nil, fmt.Errorf("underlay data size %d exceeds maximum of %d bytes: %w", len(data), maxUnderlayBytes, ErrUnderlayByteSizeExceeded)
	}

	return deserializeList(data)
}

// deserializeList handles the parsing of the underlays list format.
func deserializeList(data []byte) ([]multiaddr.Multiaddr, error) {
	var addrs []multiaddr.Multiaddr
	r := bytes.NewReader(data)

	for r.Len() > 0 {
		if len(addrs) >= maxUnderlaysPerPeer {
			return nil, fmt.Errorf("underlay list exceeds maximum of %d entries: %w", maxUnderlaysPerPeer, ErrUnderlayCountExceeded)
		}

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
