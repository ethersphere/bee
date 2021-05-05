// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"io/ioutil"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/addressbook"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	"github.com/multiformats/go-multiaddr"
)

type libp2pServiceOpts struct {
	Logger      logging.Logger
	Addressbook addressbook.Interface
	PrivateKey  *ecdsa.PrivateKey
	MockPeerKey *ecdsa.PrivateKey
	libp2pOpts  libp2p.Options
}

// mockSwapBackend is used in spoofed address validation
var mockSwapBackend = &SwapBackend{
	addrs: make(map[common.Hash]common.Address),
}

// newService constructs a new libp2p service.
func newService(t *testing.T, networkID uint64, o libp2pServiceOpts) (s *libp2p.Service, overlay swarm.Address) {
	t.Helper()

	swarmKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	overlay, err = crypto.NewOverlayAddress(swarmKey.PublicKey, networkID)
	if err != nil {
		t.Fatal(err)
	}

	addr := ":0"

	if o.Logger == nil {
		o.Logger = logging.New(ioutil.Discard, 0)
	}

	statestore := mock.NewStateStore()
	if o.Addressbook == nil {
		o.Addressbook = addressbook.New(statestore)
	}

	if o.PrivateKey == nil {
		libp2pKey, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}

		o.PrivateKey = libp2pKey
	}

	ctx, cancel := context.WithCancel(context.Background())

	lightnodes := lightnode.NewContainer()

	opts := o.libp2pOpts
	opts.Transaction = hexutil.EncodeUint64(o.PrivateKey.Y.Uint64())

	signer := crypto.NewDefaultSigner(swarmKey)
	ethAddr, _ := signer.EthereumAddress()
	tx := common.HexToHash(opts.Transaction)
	mockSwapBackend.add(tx, ethAddr)

	s, err = libp2p.New(ctx, signer, networkID, overlay, addr, o.Addressbook, statestore, lightnodes, mockSwapBackend, o.Logger, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	s.Ready()

	t.Cleanup(func() {
		cancel()
		s.Close()
		delete(mockSwapBackend.addrs, tx)
	})
	return s, overlay
}

// expectPeers validates that peers with addresses are connected.
func expectPeers(t *testing.T, s *libp2p.Service, addrs ...swarm.Address) {
	t.Helper()

	peers := s.Peers()

	if len(peers) != len(addrs) {
		t.Fatalf("got peers %v, want %v", len(peers), len(addrs))
	}

	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i].Bytes(), addrs[j].Bytes()) == -1
	})
	sort.Slice(peers, func(i, j int) bool {
		return bytes.Compare(peers[i].Address.Bytes(), peers[j].Address.Bytes()) == -1
	})

	for i, got := range peers {
		want := addrs[i]
		if !got.Address.Equal(want) {
			t.Errorf("got %v peer %s, want %s", i, got.Address, want)
		}
	}
}

// expectPeersEventually validates that peers with addresses are connected with
// retries. It is supposed to be used to validate asynchronous connecting on the
// peer that is connected to.
func expectPeersEventually(t *testing.T, s *libp2p.Service, addrs ...swarm.Address) {
	t.Helper()

	var peers []p2p.Peer
	for i := 0; i < 100; i++ {
		peers = s.Peers()
		if len(peers) == len(addrs) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(peers) != len(addrs) {
		t.Fatalf("got peers %v, want %v", len(peers), len(addrs))
	}

	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(addrs[i].Bytes(), addrs[j].Bytes()) == -1
	})
	sort.Slice(peers, func(i, j int) bool {
		return bytes.Compare(peers[i].Address.Bytes(), peers[j].Address.Bytes()) == -1
	})

	for i, got := range peers {
		want := addrs[i]
		if !got.Address.Equal(want) {
			t.Errorf("got %v peer %s, want %s", i, got.Address, want)
		}
	}
}

func serviceUnderlayAddress(t *testing.T, s *libp2p.Service) multiaddr.Multiaddr {
	t.Helper()

	addrs, err := s.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	return addrs[0]
}

// SwapBackend maps overlay to eth address
type SwapBackend struct {
	addrs map[common.Hash]common.Address
	m     sync.RWMutex
}

func (sb *SwapBackend) add(tx common.Hash, ethAddr common.Address) {
	sb.m.Lock()
	defer sb.m.Unlock()

	mockSwapBackend.addrs[tx] = ethAddr
}

func (sb *SwapBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	mockSwapBackend.m.RLock()
	defer mockSwapBackend.m.RUnlock()

	return &types.Receipt{
		ContractAddress: mockSwapBackend.addrs[txHash],
	}, nil
}
