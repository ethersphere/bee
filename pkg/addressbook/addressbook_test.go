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

func TestUpdateLastSeen(t *testing.T) {
	t.Parallel()

	now := time.Unix(1000, 0)
	store := addressbook.NewWithClock(mock.NewStateStore(), func() time.Time { return now })

	overlay := swarm.NewAddress([]byte{0, 1, 2, 3})

	// UpdateLastSeen on a missing overlay is a no-op and must not create an entry.
	if err := store.UpdateLastSeen(overlay); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(overlay); !errors.Is(err, addressbook.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if err := store.Put(overlay, newTestAddr(t, overlay), true); err != nil {
		t.Fatal(err)
	}

	// advance the clock past the update interval and bump last-seen; the entry
	// must survive a prune at the original time.
	seenAt := now.Add(2 * 24 * time.Hour)
	now = seenAt
	if err := store.UpdateLastSeen(overlay); err != nil {
		t.Fatal(err)
	}

	if err := store.Prune(seenAt.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(overlay); err != nil {
		t.Fatalf("entry pruned despite recent last-seen: %v", err)
	}
}

// TestUpdateLastSeenThrottled asserts that a bump within lastSeenUpdateInterval
// does not write. Hive calls UpdateLastSeen on every gossip sighting, so this
// is what keeps the write rate bounded to roughly one per peer per day.
func TestUpdateLastSeenThrottled(t *testing.T) {
	t.Parallel()

	base := time.Unix(1_000_000, 0)
	now := base
	mockStore := mock.NewStateStore()
	store := addressbook.NewWithClock(mockStore, func() time.Time { return now })

	overlay := swarm.NewAddress([]byte{0, 1, 2, 3})
	if err := store.Put(overlay, newTestAddr(t, overlay), true); err != nil {
		t.Fatal(err)
	}

	// well inside the interval: must not touch the record.
	now = base.Add(time.Hour)
	if err := store.UpdateLastSeen(overlay); err != nil {
		t.Fatal(err)
	}
	if got := lastSeenOf(t, mockStore, overlay); got != base.Unix() {
		t.Fatalf("throttled update wrote: last_seen = %d, want %d", got, base.Unix())
	}

	// past the interval: must write.
	now = base.Add(addressbook.LastSeenUpdateInterval + time.Second)
	if err := store.UpdateLastSeen(overlay); err != nil {
		t.Fatal(err)
	}
	if got := lastSeenOf(t, mockStore, overlay); got != now.Unix() {
		t.Fatalf("update past interval did not write: last_seen = %d, want %d", got, now.Unix())
	}
}

// TestUpdateLastSeenKeepsConcurrentPut pins the read-modify-write in
// UpdateLastSeen against a Put that lands between its read and its write.
// Without serialization the Put's Verified flag is rolled back, which would
// also desync the addressbook from hive's chequebook registry.
func TestUpdateLastSeenKeepsConcurrentPut(t *testing.T) {
	t.Parallel()

	base := time.Unix(1_000_000, 0)
	now := base
	hooked := &hookStore{StateStorer: mock.NewStateStore()}
	book := addressbook.NewWithClock(hooked, func() time.Time { return now })

	overlay := swarm.NewAddress([]byte{0, 1, 2, 3})
	addr := newTestAddr(t, overlay)

	// a known, not-yet-verified peer.
	if err := book.Put(overlay, addr, false); err != nil {
		t.Fatal(err)
	}
	// move past the throttle so UpdateLastSeen really writes.
	now = base.Add(addressbook.LastSeenUpdateInterval + time.Second)

	// While UpdateLastSeen holds the entry it has just read, hive verifies the
	// same peer and stores it with Verified=true.
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
		// addressbook lock until UpdateLastSeen returns; unsynchronized, its
		// write completes here and is then overwritten below.
		time.Sleep(100 * time.Millisecond)
	}

	if err := book.UpdateLastSeen(overlay); err != nil {
		t.Fatal(err)
	}
	<-finished

	if _, verified, err := book.Get(overlay); err != nil || !verified {
		t.Fatalf("concurrent Put(verified=true) was rolled back: verified=%v err=%v", verified, err)
	}
}

// hookStore fires onGet once, immediately after a Get returns, to interleave a
// concurrent writer inside UpdateLastSeen's read-modify-write.
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

func lastSeenOf(t *testing.T, store storage.StateStorer, overlay swarm.Address) int64 {
	t.Helper()

	v := &addressbook.VerifiedAddress{}
	if err := store.Get("addressbook_entry_"+overlay.String(), v); err != nil {
		t.Fatalf("get entry: %v", err)
	}
	return v.LastSeen
}

func TestPrune(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	store := addressbook.NewWithClock(mock.NewStateStore(), func() time.Time { return now })

	stale := swarm.NewAddress([]byte{0, 1, 2, 3})
	fresh := swarm.NewAddress([]byte{0, 1, 2, 4})

	now = time.Unix(1000, 0)
	if err := store.Put(stale, newTestAddr(t, stale), true); err != nil {
		t.Fatal(err)
	}

	now = time.Unix(9000, 0)
	if err := store.Put(fresh, newTestAddr(t, fresh), true); err != nil {
		t.Fatal(err)
	}

	if err := store.Prune(time.Unix(5000, 0)); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.Get(stale); !errors.Is(err, addressbook.ErrNotFound) {
		t.Fatalf("stale entry should have been pruned, got err=%v", err)
	}
	if _, _, err := store.Get(fresh); err != nil {
		t.Fatalf("fresh entry should survive: %v", err)
	}
}

func TestPruneKeepsEntriesWithoutLastSeen(t *testing.T) {
	t.Parallel()

	mockStore := mock.NewStateStore()
	overlay := swarm.NewAddress([]byte{0, 1, 2, 3})

	// Seed an entry without a last_seen field, mirroring records that predate
	// pruning before the stamping migration runs.
	if err := mockStore.Put("addressbook_entry_"+overlay.String(), &addressbook.VerifiedAddress{
		Address:  addrPtr(newTestAddr(t, overlay)),
		Verified: true,
	}); err != nil {
		t.Fatal(err)
	}

	store := addressbook.New(mockStore)
	if err := store.Prune(time.Unix(5000, 0)); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.Get(overlay); err != nil {
		t.Fatalf("entry without last_seen must not be pruned: %v", err)
	}
}

func addrPtr(a bzz.Address) *bzz.Address { return &a }
