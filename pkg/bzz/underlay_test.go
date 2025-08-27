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
	ip4TCPAddr := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/80/p2p/QmWqeeHEqG2db37JsuKUxyJ2JF8LtVJMGohKVT8h3aeCVH")
	p2pAddr := mustNewMultiaddr(t, "/ip4/65.108.66.216/tcp/16341/p2p/QmVuCJ3M96c7vwv4MQBv7WY1HWQacyCEHvM99R8MUDj95d")
	wssAddr := mustNewMultiaddr(t, "/ip4/127.0.0.1/tcp/443/wss/p2p/QmWqeeHEqG2db37JsuKUxyJ2JF8LtVJMGohKVT8h3aeCVH")
	dnsSwarmAddr := mustNewMultiaddr(t, "/dnsaddr/mainnet.ethswarm.org")

	t.Run("multiple addresses list", func(t *testing.T) {
		addrs := []multiaddr.Multiaddr{ip4TCPAddr, p2pAddr, wssAddr, dnsSwarmAddr}
		serialized := bzz.SerializeUnderlays(addrs)

		if serialized[0] != bzz.UnderlayListPrefix {
			t.Errorf("expected prefix %x for multiple addresses, got %x", bzz.UnderlayListPrefix, serialized[0])
		}
	})

	t.Run("single address list", func(t *testing.T) {
		addrs := []multiaddr.Multiaddr{dnsSwarmAddr}
		serialized := bzz.SerializeUnderlays(addrs)
		expected := dnsSwarmAddr.Bytes() // Should be legacy format without prefix

		if !bytes.Equal(serialized, expected) {
			t.Errorf("expected single address to serialize to legacy format %x, got %x", expected, serialized)
		}
		if serialized[0] == bzz.UnderlayListPrefix {
			t.Error("single address serialization should not have the list prefix")
		}
	})

	t.Run("empty list", func(t *testing.T) {
		addrs := []multiaddr.Multiaddr{}
		serialized := bzz.SerializeUnderlays(addrs)
		expected := []byte{bzz.UnderlayListPrefix}
		if !bytes.Equal(serialized, expected) {
			t.Errorf("expected %x for empty list, got %x", expected, serialized)
		}
	})

	t.Run("nil list", func(t *testing.T) {
		var addrs []multiaddr.Multiaddr = nil
		serialized := bzz.SerializeUnderlays(addrs)
		expected := []byte{bzz.UnderlayListPrefix}
		if !bytes.Equal(serialized, expected) {
			t.Errorf("expected %x for nil list, got %x", expected, serialized)
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

	t.Run("single legacy multiaddr", func(t *testing.T) {
		singleBytes := wssAddr.Bytes()
		deserialized, err := bzz.DeserializeUnderlays(singleBytes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(deserialized) != 1 || !deserialized[0].Equal(wssAddr) {
			t.Errorf("expected [%v], got %v", wssAddr, deserialized)
		}
	})

	t.Run("empty byte slice", func(t *testing.T) {
		_, err := bzz.DeserializeUnderlays([]byte{})
		if err == nil {
			t.Error("expected an error for empty slice, but got nil")
		}
	})

	t.Run("list with only prefix", func(t *testing.T) {
		deserialized, err := bzz.DeserializeUnderlays([]byte{bzz.UnderlayListPrefix})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(deserialized) != 0 {
			t.Errorf("expected empty slice, got %v", deserialized)
		}
	})

	t.Run("corrupted list - length too long", func(t *testing.T) {
		maBytes := ip4TCPAddr.Bytes()
		var buf bytes.Buffer
		buf.WriteByte(bzz.UnderlayListPrefix)
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
		buf.WriteByte(bzz.UnderlayListPrefix)
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(deserialized) != 0 {
			t.Errorf("expected empty slice from round trip, got %v", deserialized)
		}
	})
}

func TestLegacyCompatibility(t *testing.T) {
	ip4TCPAddr := mustNewMultiaddr(t, "/ip4/1.2.3.4/tcp/5678/p2p/QmWqeeHEqG2db37JsuKUxyJ2JF8LtVJMGohKVT8h3aeCVH")
	p2pAddr := mustNewMultiaddr(t, "/ip4/65.108.66.216/tcp/16341/p2p/QmVuCJ3M96c7vwv4MQBv7WY1HWQacyCEHvM99R8MUDj95d")
	dnsSwarmAddr := mustNewMultiaddr(t, "/dnsaddr/mainnet.ethswarm.org")

	t.Run("legacy parser fails on new list format", func(t *testing.T) {
		addrs := []multiaddr.Multiaddr{ip4TCPAddr, p2pAddr, dnsSwarmAddr}
		listBytes := bzz.SerializeUnderlays(addrs) // This will have the prefix
		_, err := multiaddr.NewMultiaddrBytes(listBytes)
		if err == nil {
			t.Error("expected legacy NewMultiaddrBytes to fail on list format, but it succeeded")
		}
	})

	t.Run("legacy parser succeeds on new single-addr format", func(t *testing.T) {
		addrs := []multiaddr.Multiaddr{dnsSwarmAddr}
		singleBytes := bzz.SerializeUnderlays(addrs) // This will NOT have the prefix
		_, err := multiaddr.NewMultiaddrBytes(singleBytes)
		if err != nil {
			t.Errorf("expected legacy NewMultiaddrBytes to succeed on single-addr format, but it failed: %v", err)
		}
	})

	t.Run("new parser succeeds on legacy format", func(t *testing.T) {
		singleBytes := p2pAddr.Bytes()
		deserialized, err := bzz.DeserializeUnderlays(singleBytes)
		if err != nil {
			t.Fatalf("Deserialize failed on legacy bytes: %v", err)
		}
		expected := []multiaddr.Multiaddr{p2pAddr}
		if !reflect.DeepEqual(expected, deserialized) {
			t.Errorf("expected %v, got %v", expected, deserialized)
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
