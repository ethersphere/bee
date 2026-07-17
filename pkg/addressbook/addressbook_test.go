// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package addressbook_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	ma "github.com/multiformats/go-multiaddr"
)

func newTestAddr(t *testing.T, overlay swarm.Address) bzz.Address {
	t.Helper()

	multiaddr, err := ma.NewMultiaddr("/ip4/1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	bzzAddr, err := bzz.NewAddress(crypto.NewDefaultSigner(pk), []ma.Multiaddr{multiaddr}, overlay, 1, common.HexToHash("0x1").Bytes(), 1, common.Address{})
	if err != nil {
		t.Fatal(err)
	}
	return *bzzAddr
}

type bookFunc func() (book addressbook.Interface)

func TestInMem(t *testing.T) {
	t.Parallel()

	run(t, func() addressbook.Interface {
		store := mock.NewStateStore()
		book := addressbook.New(store)
		return book
	})
}

func run(t *testing.T, f bookFunc) {
	t.Helper()

	store := f()
	addr1 := swarm.NewAddress([]byte{0, 1, 2, 3})
	addr2 := swarm.NewAddress([]byte{0, 1, 2, 4})
	trxHash := common.HexToHash("0x1").Bytes()
	multiaddr, err := ma.NewMultiaddr("/ip4/1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}

	pk, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	bzzAddr, err := bzz.NewAddress(crypto.NewDefaultSigner(pk), []ma.Multiaddr{multiaddr}, addr1, 1, trxHash, 1, common.Address{})
	if err != nil {
		t.Fatal(err)
	}

	err = store.Put(addr1, *bzzAddr, true)
	if err != nil {
		t.Fatal(err)
	}

	v, verified, err := store.Get(addr1)
	if err != nil {
		t.Fatal(err)
	}

	if !verified {
		t.Fatal("expected verified flag to be true")
	}

	if !bzzAddr.Equal(v) {
		t.Fatalf("expectted: %s, want %s", v, multiaddr)
	}

	notFound, _, err := store.Get(addr2)
	if !errors.Is(err, addressbook.ErrNotFound) {
		t.Fatal(err)
	}

	if notFound != nil {
		t.Fatalf("expected nil got %s", v)
	}

	overlays, err := store.Overlays()
	if err != nil {
		t.Fatal(err)
	}

	if len(overlays) != 1 {
		t.Fatalf("expected overlay len %v, got %v", 1, len(overlays))
	}

	addresses, err := store.Addresses()
	if err != nil {
		t.Fatal(err)
	}

	if len(addresses) != 1 {
		t.Fatalf("expected addresses len %v, got %v", 1, len(addresses))
	}
}

// TestSeen covers a sighting of a peer we already hold: the last-seen time
// moves, and the entry then survives a prune that would otherwise catch it. An
// overlay we do not know is skipped rather than created.
func TestSeen(t *testing.T) {
	t.Parallel()

	base := time.Unix(1_000_000, 0)
	now := base
	state := mock.NewStateStore()
	book := addressbook.NewWithClock(state, func() time.Time { return now })

	overlay := swarm.NewAddress([]byte{0, 1, 2, 3})

	// an unknown overlay is skipped, not created.
	if err := book.Seen(overlay); err != nil {
		t.Fatal(err)
	}
	if _, _, err := book.Get(overlay); !errors.Is(err, addressbook.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if err := book.Put(overlay, newTestAddr(t, overlay), true); err != nil {
		t.Fatal(err)
	}

	// a much later sighting moves last-seen forward, so a prune whose cutoff
	// predates it keeps the entry.
	now = base.Add(90 * 24 * time.Hour)
	if err := book.Seen(overlay); err != nil {
		t.Fatal(err)
	}
	if got := lastSeenOf(t, state, overlay); got != now.Unix() {
		t.Fatalf("seen did not move last seen: got %d, want %d", got, now.Unix())
	}

	// reopening the book prunes whatever has gone unseen for PruneAfter; the
	// sighting above is what keeps this entry.
	reopened := addressbook.NewWithClock(state, func() time.Time { return now })
	if _, verified, err := reopened.Get(overlay); err != nil || !verified {
		t.Fatalf("entry pruned despite a recent sighting: verified=%v err=%v", verified, err)
	}
}

// TestSeenVariadic marks several overlays in one call, which is how kademlia
// refreshes everything it is connected to.
func TestSeenVariadic(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_000_000, 0)
	state := mock.NewStateStore()
	book := addressbook.NewWithClock(state, func() time.Time { return now })

	overlays := []swarm.Address{
		swarm.NewAddress([]byte{0, 1, 2, 3}),
		swarm.NewAddress([]byte{0, 1, 2, 4}),
	}
	for _, overlay := range overlays {
		if err := book.Put(overlay, newTestAddr(t, overlay), true); err != nil {
			t.Fatal(err)
		}
	}

	now = now.Add(time.Hour)
	if err := book.Seen(overlays...); err != nil {
		t.Fatal(err)
	}

	for _, overlay := range overlays {
		if got := lastSeenOf(t, state, overlay); got != now.Unix() {
			t.Fatalf("overlay %s not marked seen: got %d, want %d", overlay, got, now.Unix())
		}
	}
}

// TestSeenKeepsConcurrentPut pins Seen's read-modify-write against a Put that
// lands between its read and its write. Seen rewrites the whole record, so
// without serialization the Put's verified flag is rolled back, which would
// also desync the addressbook from hive's chequebook registry.
func TestSeenKeepsConcurrentPut(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_000_000, 0)
	hooked := &hookStore{StateStorer: mock.NewStateStore()}
	book := addressbook.NewWithClock(hooked, func() time.Time { return now })

	overlay := swarm.NewAddress([]byte{0, 1, 2, 3})
	addr := newTestAddr(t, overlay)

	// a known, not yet verified peer.
	if err := book.Put(overlay, addr, false); err != nil {
		t.Fatal(err)
	}

	// While Seen holds the entry it has just read, hive verifies the same peer
	// and stores it with Verified=true.
	started, finished := make(chan struct{}), make(chan struct{})
	hooked.onGet = func() {
		go func() {
			defer close(finished)
			close(started)
			if err := book.Put(overlay, addr, true); err != nil {
				t.Error(err)
			}
		}()
		<-started
		// Give the writer time to land. Serialized, it blocks on the
		// addressbook lock until Seen returns; unsynchronized, its write
		// completes here and is then overwritten below.
		time.Sleep(100 * time.Millisecond)
	}

	if err := book.Seen(overlay); err != nil {
		t.Fatal(err)
	}
	<-finished

	if _, verified, err := book.Get(overlay); err != nil || !verified {
		t.Fatalf("concurrent Put(verified=true) was rolled back: verified=%v err=%v", verified, err)
	}
}

// hookStore fires onGet once, immediately after a Get returns, to interleave a
// concurrent writer inside Seen's read-modify-write.
type hookStore struct {
	storage.StateStorer
	onGet func()
}

func (h *hookStore) Get(key string, i any) error {
	err := h.StateStorer.Get(key, i)
	if h.onGet != nil {
		f := h.onGet
		h.onGet = nil
		f()
	}
	return err
}

func lastSeenOf(t *testing.T, state storage.StateStorer, overlay swarm.Address) int64 {
	t.Helper()

	v := &addressbook.VerifiedAddress{}
	if err := state.Get("addressbook_entry_"+overlay.String(), v); err != nil {
		t.Fatalf("get entry: %v", err)
	}
	return v.LastSeen
}

// TestPrune drops overlays last seen before the cutoff and keeps the rest.
func TestPrune(t *testing.T) {
	t.Parallel()

	base := time.Unix(1_000_000_000, 0)
	now := base
	state := mock.NewStateStore()
	book := addressbook.NewWithClock(state, func() time.Time { return now })

	stale := swarm.NewAddress([]byte{0, 1, 2, 3})
	if err := book.Put(stale, newTestAddr(t, stale), true); err != nil {
		t.Fatal(err)
	}

	now = now.Add(48 * time.Hour)
	fresh := swarm.NewAddress([]byte{0, 1, 2, 4})
	if err := book.Put(fresh, newTestAddr(t, fresh), true); err != nil {
		t.Fatal(err)
	}

	// Reopen with a clock that puts the cutoff between the two puts: the first
	// overlay has gone unseen for longer than PruneAfter, the second has not.
	reopenAt := base.Add(addressbook.PruneAfter + time.Hour)
	reopened := addressbook.NewWithClock(state, func() time.Time { return reopenAt })

	if _, _, err := reopened.Get(stale); !errors.Is(err, addressbook.ErrNotFound) {
		t.Fatalf("stale entry should have been pruned, got err=%v", err)
	}
	if _, _, err := reopened.Get(fresh); err != nil {
		t.Fatalf("fresh entry should survive: %v", err)
	}
}

// TestPruneKeepsEntriesWithoutLastSeen covers records that predate pruning and
// have not been stamped by the migration. They are kept, and stamped on their
// next sighting.
func TestPruneKeepsEntriesWithoutLastSeen(t *testing.T) {
	t.Parallel()

	state := mock.NewStateStore()
	overlay := swarm.NewAddress([]byte{0, 1, 2, 3})

	if err := state.Put("addressbook_entry_"+overlay.String(), &addressbook.VerifiedAddress{
		Address:  addrPtr(newTestAddr(t, overlay)),
		Verified: true,
	}); err != nil {
		t.Fatal(err)
	}

	// open the book far past any plausible cutoff: the entry still survives,
	// because it carries no last-seen time to judge it by.
	book := addressbook.NewWithClock(state, func() time.Time { return time.Unix(5_000_000_000, 0) })
	if _, _, err := book.Get(overlay); err != nil {
		t.Fatalf("entry without a last-seen time must not be pruned: %v", err)
	}
}

func addrPtr(a bzz.Address) *bzz.Address { return &a }
