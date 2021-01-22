package feeds_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestFinder(t *testing.T) {
	storer := mock.NewStorer()
	topic := "testtopic"
	pk, _ := crypto.GenerateSecp256k1Key()
	signer := crypto.NewDefaultSigner(pk)

	updater, err := feeds.NewUpdater(storer, signer, topic)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	finder := feeds.NewFinder(storer, updater.Feed)
	t.Run("no update", func(t *testing.T) {
		ch, err := finder.Latest(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		if ch != nil {
			t.Fatalf("expected no update, got addr %v", ch.Address())
		}
	})
	t.Run("first update root epoch", func(t *testing.T) {
		payload := []byte("payload")
		at := time.Now().Unix()
		err = updater.Update(ctx, at, payload)
		if err != nil {
			t.Fatal(err)
		}
		ch, err := finder.Latest(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		if ch == nil {
			t.Fatalf("expected to find update, got none")
		}
		exp := payload
		payload, ts, err := feeds.FromChunk(ch)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(payload, exp) {
			t.Fatalf("result mismatch. want %8x... got %8x...", exp, payload)
		}
		if ts != uint64(at) {
			t.Fatalf("timestamp mismatch: expected %v, got %v", at, ts)
		}
	})
}

func TestFinderEverySecond(t *testing.T) {
	for _, tc := range []struct {
		count  int64
		step   int64
		offset int64
	}{
		{50, 1, 0},
		{50, 1, 100000},
		{50, 1000, 0},
		{50, 1000, 1000000},
	} {
		t.Run(fmt.Sprintf("count=%d,step=%d,offset=%d", tc.count, tc.step, tc.offset), func(t *testing.T) {
			storer := mock.NewStorer()
			topic := "testtopic"
			pk, _ := crypto.GenerateSecp256k1Key()
			signer := crypto.NewDefaultSigner(pk)

			updater, err := feeds.NewUpdater(storer, signer, topic)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()

			payload := []byte("payload")
			for at := tc.offset; at < tc.offset+tc.count*tc.step; at += tc.step {
				err = updater.Update(ctx, at, payload)
				if err != nil {
					t.Fatal(err)
				}
			}
			finder := feeds.NewFinder(storer, updater.Feed)
			for at := tc.offset; at < tc.offset+tc.count*tc.step; at += tc.step {
				for after := tc.offset; after < at; after += tc.step {
					for now := at + 4; now < at+tc.step; now += tc.step / 4 {

						ch, err := finder.At(ctx, now, after)
						if err != nil {
							t.Fatal(err)
						}
						if ch == nil {
							t.Fatalf("expected to find update, got none")
						}
						exp := payload
						payload, ts, err := feeds.FromChunk(ch)
						if err != nil {
							t.Fatal(err)
						}
						if !bytes.Equal(payload, exp) {
							t.Fatalf("payload mismatch: expected %x, got %x", exp, payload)
						}
						if ts != uint64(at) {
							t.Fatalf("timestamp mismatch: expected %v, got %v", at, ts)
						}
					}
				}
			}
		})
	}
}

func TestFinderRandomIntervals(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("random intervals %d", i), func(t *testing.T) {
			storer := mock.NewStorer()
			topic := "testtopic"
			pk, _ := crypto.GenerateSecp256k1Key()
			signer := crypto.NewDefaultSigner(pk)

			updater, err := feeds.NewUpdater(storer, signer, topic)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()

			payload := []byte("payload")
			var at int64
			ats := make([]int64, 100)
			for j := 0; j < 50; j++ {
				at += int64(rand.Intn(1 << 10))
				ats[j] = at
				err = updater.Update(ctx, at, payload)
				if err != nil {
					t.Fatal(err)
				}
			}
			finder := feeds.NewFinder(storer, updater.Feed)
			for j := 0; j < 49; j++ {
				diff := ats[j+1] - ats[j]
				for at := ats[j]; at < ats[j+1]; at += int64(rand.Intn(int(diff))) {
					for after := int64(0); after < at; after += int64(rand.Intn(int(at))) {
						ch, err := finder.At(ctx, at, after)
						if err != nil {
							t.Fatal(err)
						}
						if ch == nil {
							t.Fatalf("expected to find update, got none")
						}
						exp := payload
						payload, ts, err := feeds.FromChunk(ch)
						if err != nil {
							t.Fatal(err)
						}
						if !bytes.Equal(payload, exp) {
							t.Fatalf("payload mismatch: expected %x, got %x", exp, payload)
						}
						if ts != uint64(ats[j]) {
							t.Fatalf("timestamp mismatch: expected %v, got %v", ats[j], ts)
						}
					}
				}
			}
		})
	}
}

var span = []byte{0, 0, 0, 0, 0, 0, 0, 8}

func isUpdateAt(t *testing.T, ch swarm.Chunk, at int64) {
	t.Helper()
	s, err := soc.FromChunk(ch)
	if err != nil {
		t.Fatal(err)
	}
	cac := s.Chunk
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(at))
	content := append(ts, ts...)
	exp := append(append([]byte{}, span...), content...)
	if !bytes.Equal(cac.Data(), exp) {
		t.Fatalf("feed update mismatch. want %24x... got %24x...", exp, cac.Data())
	}
}
