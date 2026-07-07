// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/multiformats/go-multiaddr"
)

type libp2pServiceOpts struct {
	Logger             log.Logger
	Addressbook        addressbook.Interface
	PrivateKey         *ecdsa.PrivateKey
	MockPeerKey        *ecdsa.PrivateKey
	libp2pOpts         libp2p.Options
	lightNodes         *lightnode.Container
	notifier           p2p.PickyNotifier
	autoTLSCertManager libp2p.AutoTLSCertManager
}

// newService constructs a new libp2p service.
func newService(t *testing.T, networkID uint64, o libp2pServiceOpts) (s *libp2p.Service, overlay swarm.Address) {
	t.Helper()

	swarmKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	nonce := common.HexToHash("0x1").Bytes()

	overlay, err = crypto.NewOverlayAddress(swarmKey.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	addr := ":0"

	if o.Logger == nil {
		o.Logger = log.Noop
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
	t.Cleanup(cancel)

	if o.lightNodes == nil {
		o.lightNodes = lightnode.NewContainer(overlay)
	}
	opts := o.libp2pOpts
	opts.Nonce = nonce
	// Tests connect over loopback / private addresses; allow them by default.
	opts.AllowPrivateCIDRs = true

	if o.autoTLSCertManager != nil {
		libp2p.SetAutoTLSCertManager(&opts, o.autoTLSCertManager)
	}

	s, err = libp2p.New(ctx, crypto.NewDefaultSigner(swarmKey), networkID, overlay, addr, o.Addressbook, statestore, o.lightNodes, o.Logger, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CleanupCloser(t, s)

	if o.notifier != nil {
		s.SetPickyNotifier(o.notifier)
	}

	_ = s.Ready()

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
	err := spinlock.Wait(5*time.Second, func() bool {
		peers = s.Peers()
		return len(peers) == len(addrs)
	})
	if err != nil {
		t.Fatalf("timed out waiting for peers, got  %v, want %v", len(peers), len(addrs))
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

func serviceUnderlayAddress(t *testing.T, s *libp2p.Service) []multiaddr.Multiaddr {
	t.Helper()

	addrs, err := s.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	return addrs
}

// fakeStorer is a libp2p.ChequebookStorer test double that records calls and
// optionally returns a configured error. When err is nil, it runs the
// writeAddressbook callback (mirroring the real registry behaviour on success).
type fakeStorer struct {
	err            error
	puts           int
	cbCallbacks    int
	lastPeer       swarm.Address
	lastChequebook common.Address
}

func (s *fakeStorer) Put(peer swarm.Address, cb common.Address, _ int64, _ bzz.TimestampSource, writeAddressbook func() error) error {
	s.puts++
	s.lastPeer = peer
	s.lastChequebook = cb
	if s.err != nil {
		return s.err
	}
	if writeAddressbook != nil {
		s.cbCallbacks++
		return writeAddressbook()
	}
	return nil
}

func (s *fakeStorer) Remove(_ swarm.Address) {}

// testAddr builds a properly-signed bzz.Address; the addressbook's JSON
// codec rejects entries with no underlays, so we cannot use a bare struct
// literal here.
func testAddr(t *testing.T, cb common.Address) *bzz.Address {
	t.Helper()

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)
	nonce := common.HexToHash("0x1").Bytes()

	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, 1, nonce)
	if err != nil {
		t.Fatal(err)
	}

	underlay, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1634")
	if err != nil {
		t.Fatal(err)
	}

	addr, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{underlay}, overlay, 1, nonce, time.Now().Unix(), cb)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

// TestPutHandshakeAddress_StorerSuccess_VerifiedTrue covers the happy path
// when chequebookStorer is set: the storer is invoked, its writeAddressbook
// callback runs, and the addressbook entry is stored with Verified=true.
func TestPutHandshakeAddress_StorerSuccess_VerifiedTrue(t *testing.T) {
	t.Parallel()

	book := addressbook.New(mock.NewStateStore())
	storer := &fakeStorer{}
	svc := libp2p.NewPutHandshakeAddressTestService(log.Noop, book, storer)

	cb := common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr := testAddr(t, cb)

	if err := svc.PutHandshakeAddress(addr); err != nil {
		t.Fatalf("PutHandshakeAddress: %v", err)
	}

	if storer.puts != 1 {
		t.Fatalf("storer.Put calls: got %d, want 1", storer.puts)
	}
	if storer.cbCallbacks != 1 {
		t.Fatalf("writeAddressbook callbacks: got %d, want 1", storer.cbCallbacks)
	}
	if storer.lastChequebook != cb {
		t.Fatalf("storer cb: got %s, want %s", storer.lastChequebook.Hex(), cb.Hex())
	}

	got, verified, err := book.Get(addr.Overlay)
	if err != nil {
		t.Fatalf("addressbook.Get: %v", err)
	}
	if !verified {
		t.Fatal("addressbook entry must be stored with Verified=true after successful handshake")
	}
	if got.ChequebookAddress != cb {
		t.Fatalf("addressbook cb: got %s, want %s", got.ChequebookAddress.Hex(), cb.Hex())
	}
}

// TestPutHandshakeAddress_StorerTimestampError_Swallowed covers the
// concurrent-gossip race: when the storer rejects the update with a timestamp
// sentinel, the error is swallowed (returns nil), the addressbook is not
// written, and the caller proceeds.
func TestPutHandshakeAddress_StorerTimestampError_Swallowed(t *testing.T) {
	t.Parallel()

	book := addressbook.New(mock.NewStateStore())
	storer := &fakeStorer{err: bzz.ErrTimestampStale}
	svc := libp2p.NewPutHandshakeAddressTestService(log.Noop, book, storer)

	addr := testAddr(t, common.HexToAddress("0x2222222222222222222222222222222222222222"))

	if err := svc.PutHandshakeAddress(addr); err != nil {
		t.Fatalf("PutHandshakeAddress: timestamp error must be swallowed, got %v", err)
	}
	if storer.cbCallbacks != 0 {
		t.Fatal("writeAddressbook must not run when storer returns error")
	}
	if _, _, err := book.Get(addr.Overlay); !errors.Is(err, addressbook.ErrNotFound) {
		t.Fatalf("addressbook must remain empty after swallowed timestamp error, got %v", err)
	}
}

// TestPutHandshakeAddress_StorerOtherError_Propagated covers the failure path:
// non-timestamp storer errors propagate to the caller and the addressbook is
// not written.
func TestPutHandshakeAddress_StorerOtherError_Propagated(t *testing.T) {
	t.Parallel()

	book := addressbook.New(mock.NewStateStore())
	want := errors.New("registry: out of disk")
	storer := &fakeStorer{err: want}
	svc := libp2p.NewPutHandshakeAddressTestService(log.Noop, book, storer)

	addr := testAddr(t, common.HexToAddress("0x3333333333333333333333333333333333333333"))

	if err := svc.PutHandshakeAddress(addr); !errors.Is(err, want) {
		t.Fatalf("PutHandshakeAddress: got %v, want wrapped %v", err, want)
	}
	if _, _, err := book.Get(addr.Overlay); !errors.Is(err, addressbook.ErrNotFound) {
		t.Fatalf("addressbook must remain empty after storer error, got %v", err)
	}
}

// TestPutHandshakeAddress_NoStorer_FallbackVerifiedTrue covers the chequebook
// subsystem disabled path: with chequebookStorer nil, putHandshakeAddress
// writes directly to the addressbook with Verified=true.
func TestPutHandshakeAddress_NoStorer_FallbackVerifiedTrue(t *testing.T) {
	t.Parallel()

	book := addressbook.New(mock.NewStateStore())
	svc := libp2p.NewPutHandshakeAddressTestService(log.Noop, book, nil)

	cb := common.HexToAddress("0x4444444444444444444444444444444444444444")
	addr := testAddr(t, cb)

	if err := svc.PutHandshakeAddress(addr); err != nil {
		t.Fatalf("PutHandshakeAddress: %v", err)
	}

	got, verified, err := book.Get(addr.Overlay)
	if err != nil {
		t.Fatalf("addressbook.Get: %v", err)
	}
	if !verified {
		t.Fatal("addressbook entry must be stored with Verified=true on the no-storer fallback path")
	}
	if got.ChequebookAddress != cb {
		t.Fatalf("addressbook cb: got %s, want %s", got.ChequebookAddress.Hex(), cb.Hex())
	}
}

// countingBook wraps an addressbook and counts writes going through Put.
type countingBook struct {
	addressbook.GetPutter
	puts int
}

func (b *countingBook) Put(overlay swarm.Address, addr bzz.Address, verified bool) error {
	b.puts++
	return b.GetPutter.Put(overlay, addr, verified)
}

// testAddrPair builds two properly-signed bzz.Addresses for the same peer
// identity, the second with a newer timestamp, mirroring a peer whose record
// genuinely changed between two handshakes.
func testAddrPair(t *testing.T, cb common.Address) (*bzz.Address, *bzz.Address) {
	t.Helper()

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(pk)
	nonce := common.HexToHash("0x1").Bytes()

	overlay, err := crypto.NewOverlayAddress(pk.PublicKey, 1, nonce)
	if err != nil {
		t.Fatal(err)
	}

	underlay, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1634")
	if err != nil {
		t.Fatal(err)
	}

	ts := time.Now().Unix()
	first, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{underlay}, overlay, 1, nonce, ts, cb)
	if err != nil {
		t.Fatal(err)
	}
	second, err := bzz.NewAddress(signer, []multiaddr.Multiaddr{underlay}, overlay, 1, nonce, ts+1, cb)
	if err != nil {
		t.Fatal(err)
	}
	return first, second
}

// TestPutHandshakeAddress_UnchangedRecord_SkipsAddressbookWrite covers the
// reconnect path with the storer wired: a byte-identical, already-verified
// record skips the addressbook write, while the storer still runs so the
// chequebook-overlay mapping dropped on disconnect is re-registered.
func TestPutHandshakeAddress_UnchangedRecord_SkipsAddressbookWrite(t *testing.T) {
	t.Parallel()

	book := addressbook.New(mock.NewStateStore())
	storer := &fakeStorer{}
	svc := libp2p.NewPutHandshakeAddressTestService(log.Noop, book, storer)

	addr := testAddr(t, common.HexToAddress("0x5555555555555555555555555555555555555555"))

	if err := svc.PutHandshakeAddress(addr); err != nil {
		t.Fatalf("PutHandshakeAddress: %v", err)
	}
	if err := svc.PutHandshakeAddress(addr); err != nil {
		t.Fatalf("PutHandshakeAddress (reconnect): %v", err)
	}

	if storer.puts != 2 {
		t.Fatalf("storer.Put calls: got %d, want 2 (mapping must re-register on reconnect)", storer.puts)
	}
	if storer.cbCallbacks != 1 {
		t.Fatalf("writeAddressbook callbacks: got %d, want 1 (unchanged record must skip the write)", storer.cbCallbacks)
	}
}

// TestPutHandshakeAddress_NoStorer_UnchangedRecord_SkipsWrite covers the
// reconnect path with the chequebook subsystem disabled: the second put of an
// identical record performs no addressbook write.
func TestPutHandshakeAddress_NoStorer_UnchangedRecord_SkipsWrite(t *testing.T) {
	t.Parallel()

	book := &countingBook{GetPutter: addressbook.New(mock.NewStateStore())}
	svc := libp2p.NewPutHandshakeAddressTestService(log.Noop, book, nil)

	addr := testAddr(t, common.HexToAddress("0x6666666666666666666666666666666666666666"))

	if err := svc.PutHandshakeAddress(addr); err != nil {
		t.Fatalf("PutHandshakeAddress: %v", err)
	}
	if err := svc.PutHandshakeAddress(addr); err != nil {
		t.Fatalf("PutHandshakeAddress (reconnect): %v", err)
	}

	if book.puts != 1 {
		t.Fatalf("addressbook writes: got %d, want 1 (unchanged record must skip the write)", book.puts)
	}
}

// TestPutHandshakeAddress_IdenticalButUnverified_Writes guards the verified
// upgrade: a record identical to the stored one but persisted with
// Verified=false (e.g. by gossip) must still be re-written with Verified=true.
func TestPutHandshakeAddress_IdenticalButUnverified_Writes(t *testing.T) {
	t.Parallel()

	inner := addressbook.New(mock.NewStateStore())
	book := &countingBook{GetPutter: inner}
	svc := libp2p.NewPutHandshakeAddressTestService(log.Noop, book, nil)

	addr := testAddr(t, common.HexToAddress("0x7777777777777777777777777777777777777777"))

	if err := inner.Put(addr.Overlay, *addr, false); err != nil {
		t.Fatalf("seed addressbook: %v", err)
	}

	if err := svc.PutHandshakeAddress(addr); err != nil {
		t.Fatalf("PutHandshakeAddress: %v", err)
	}

	if book.puts != 1 {
		t.Fatalf("addressbook writes: got %d, want 1 (unverified record must be upgraded)", book.puts)
	}
	_, verified, err := inner.Get(addr.Overlay)
	if err != nil {
		t.Fatalf("addressbook.Get: %v", err)
	}
	if !verified {
		t.Fatal("addressbook entry must be upgraded to Verified=true by the handshake")
	}
}

// TestPutHandshakeAddress_ChangedRecord_Writes verifies a genuinely updated
// record for a known overlay is still written.
func TestPutHandshakeAddress_ChangedRecord_Writes(t *testing.T) {
	t.Parallel()

	book := &countingBook{GetPutter: addressbook.New(mock.NewStateStore())}
	svc := libp2p.NewPutHandshakeAddressTestService(log.Noop, book, nil)

	first, second := testAddrPair(t, common.HexToAddress("0x8888888888888888888888888888888888888888"))

	if err := svc.PutHandshakeAddress(first); err != nil {
		t.Fatalf("PutHandshakeAddress: %v", err)
	}
	if err := svc.PutHandshakeAddress(second); err != nil {
		t.Fatalf("PutHandshakeAddress (updated record): %v", err)
	}

	if book.puts != 2 {
		t.Fatalf("addressbook writes: got %d, want 2 (changed record must be written)", book.puts)
	}
	got, _, err := book.Get(first.Overlay)
	if err != nil {
		t.Fatalf("addressbook.Get: %v", err)
	}
	if got.Timestamp != second.Timestamp {
		t.Fatalf("stored timestamp: got %d, want %d", got.Timestamp, second.Timestamp)
	}
}
