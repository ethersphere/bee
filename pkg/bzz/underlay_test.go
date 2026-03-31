// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-varint"
)

func TestSerializeUnderlays(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		addrs := []multiaddr.Multiaddr{}
		serialized := bzz.SerializeUnderlays(addrs)
		if len(serialized) != 0 {
			t.Errorf("expected empty serialized addresses")
		}
	})

	t.Run("nil list", func(t *testing.T) {
		var addrs []multiaddr.Multiaddr = nil
		serialized := bzz.SerializeUnderlays(addrs)
		if len(serialized) != 0 {
			t.Errorf("expected empty serialized addresses")
		}
	})
}

func TestDeserializeUnderlays(t *testing.T) {
	ip4TCPAddr := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/80/p2p/QmWqeeHEqG2db37JsuKUxyJ2JF8LtVJMGohKVT8h3aeCVH")
	p2pAddr := mustNewMultiaddr(t, "/ip4/65.108.66.216/tcp/16341/p2p/QmVuCJ3M96c7vwv4MQBv7WY1HWQacyCEHvM99R8MUDj95d")
	wssAddr := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/443/wss/p2p/QmWqeeHEqG2db37JsuKUxyJ2JF8LtVJMGohKVT8h3aeCVH")

	t.Run("valid list of multiple", func(t *testing.T) {
		addrs := []multiaddr.Multiaddr{ip4TCPAddr, p2pAddr, wssAddr}
		serialized := bzz.SerializeUnderlays(addrs)
		deserialized, err := bzz.DeserializeUnderlays(serialized)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(addrs, deserialized) {
			t.Errorf("expected %v, got %v", addrs, deserialized)
		}
	})

	t.Run("empty byte slice", func(t *testing.T) {
		_, err := bzz.DeserializeUnderlays([]byte{})
		if err == nil {
			t.Error("expected an error for empty slice, but got nil")
		}
	})

	t.Run("serialize deserialize empty list", func(t *testing.T) {
		addrs := []multiaddr.Multiaddr{}
		serialized := bzz.SerializeUnderlays(addrs)
		deserialized, err := bzz.DeserializeUnderlays(serialized)
		if err == nil {
			t.Fatalf("expected error")
		}
		if len(deserialized) != 0 {
			t.Errorf("expected empty slice, got %v", deserialized)
		}
	})

	t.Run("serialize deserialize nil list", func(t *testing.T) {
		serialized := bzz.SerializeUnderlays(nil)
		deserialized, err := bzz.DeserializeUnderlays(serialized)
		if err == nil {
			t.Fatalf("expected error")
		}
		if len(deserialized) != 0 {
			t.Errorf("expected empty slice, got %v", deserialized)
		}
	})

	t.Run("corrupted list - length too long", func(t *testing.T) {
		maBytes := ip4TCPAddr.Bytes()
		var buf bytes.Buffer
		buf.Write(varint.ToUvarint(uint64(len(maBytes) + 5))) // Write a length that is too long
		buf.Write(maBytes)

		_, err := bzz.DeserializeUnderlays(buf.Bytes())
		if err == nil {
			t.Error("expected an error for corrupted data, but got nil")
		}
	})

	t.Run("corrupted list - invalid multiaddr bytes", func(t *testing.T) {
		invalidAddrBytes := []byte{0xde, 0xad, 0xbe, 0xef}
		var buf bytes.Buffer
		buf.Write(varint.ToUvarint(uint64(len(invalidAddrBytes))))
		buf.Write(invalidAddrBytes)

		_, err := bzz.DeserializeUnderlays(buf.Bytes())
		if err == nil {
			t.Error("expected an error for invalid multiaddr bytes, but got nil")
		}
	})
}

func TestSerializeUnderlaysDeserializeUnderlays(t *testing.T) {
	ip4TCPAddr := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/80/p2p/QmWqeeHEqG2db37JsuKUxyJ2JF8LtVJMGohKVT8h3aeCVH")
	dnsSwarmAddr := mustNewMultiaddr(t, "/dnsaddr/mainnet.ethswarm.org")
	p2pAddr := mustNewMultiaddr(t, "/ip4/65.108.66.216/tcp/16341/p2p/QmVuCJ3M96c7vwv4MQBv7WY1HWQacyCEHvM99R8MUDj95d")
	wssAddr := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/443/wss/p2p/QmWqeeHEqG2db37JsuKUxyJ2JF8LtVJMGohKVT8h3aeCVH")

	t.Run("multiple addresses list", func(t *testing.T) {
		addrs := []multiaddr.Multiaddr{ip4TCPAddr, dnsSwarmAddr, p2pAddr, wssAddr}
		serialized := bzz.SerializeUnderlays(addrs)
		deserialized, err := bzz.DeserializeUnderlays(serialized)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(addrs, deserialized) {
			t.Errorf("round trip failed. expected %v, got %v", addrs, deserialized)
		}
	})

	t.Run("single address list", func(t *testing.T) {
		addrs := []multiaddr.Multiaddr{dnsSwarmAddr}
		serialized := bzz.SerializeUnderlays(addrs)
		deserialized, err := bzz.DeserializeUnderlays(serialized)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(addrs, deserialized) {
			t.Errorf("round trip failed. expected %v, got %v", addrs, deserialized)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		addrs := []multiaddr.Multiaddr{}
		serialized := bzz.SerializeUnderlays(addrs)
		deserialized, err := bzz.DeserializeUnderlays(serialized)
		if err == nil {
			t.Fatalf("expected error")
		}
		if len(deserialized) != 0 {
			t.Errorf("expected empty slice from round trip, got %v", deserialized)
		}
	})
}

func mustNewMultiaddr(tb testing.TB, s string) multiaddr.Multiaddr {
	tb.Helper()

	a, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		tb.Fatal(err)
	}
	return a
}
