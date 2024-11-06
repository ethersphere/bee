// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mockstorer_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/google/go-cmp/cmp"
)

var now = func() time.Time { return time.Unix(1234567890, 0) }

// TestMain exists to adjust the time.Now function to a fixed value.
func TestMain(m *testing.M) {
	mockstorer.ReplaceTimeNow(now)
	code := m.Run()
	mockstorer.ReplaceTimeNow(time.Now)
	os.Exit(code)
}

func TestMockStorer(t *testing.T) {
	t.Parallel()

	mockStorer := mockstorer.New()

	t.Run("new session", func(t *testing.T) {
		have, err := mockStorer.NewSession()
		if err != nil {
			t.Fatalf("NewSession(): unexpected error: %v", err)
		}

		want := storer.SessionInfo{TagID: 1, StartedAt: now().UnixNano()}

		if diff := cmp.Diff(want, have); diff != "" {
			t.Fatalf("unexpected session info: (-want +have):\n%s", diff)
		}
	})

	t.Run("get session", func(t *testing.T) {
		have, err := mockStorer.Session(1)
		if err != nil {
			t.Fatalf("Session(): unexpected error: %v", err)
		}

		want := storer.SessionInfo{TagID: 1, StartedAt: now().UnixNano()}

		if diff := cmp.Diff(want, have); diff != "" {
			t.Fatalf("unexpected session info: (-want +have):\n%s", diff)
		}
	})

	t.Run("list sessions", func(t *testing.T) {
		have, err := mockStorer.ListSessions(0, 1)
		if err != nil {
			t.Fatalf("ListSessions(): unexpected error: %v", err)
		}

		want := []storer.SessionInfo{
			{TagID: 1, StartedAt: now().UnixNano()},
		}

		if diff := cmp.Diff(want, have); diff != "" {
			t.Fatalf("unexpected session info: (-want +have):\n%s", diff)
		}
	})

	t.Run("delete session", func(t *testing.T) {
		if err := mockStorer.DeleteSession(1); err != nil {
			t.Fatalf("DeleteSession(): unexpected error: %v", err)
		}

		want := storage.ErrNotFound
		_, have := mockStorer.Session(1)
		if !errors.Is(have, want) {
			t.Fatalf("Session(): unexpected error: want %v have %v", want, have)
		}
	})

	verifyChunksAndPin := func(t *testing.T, chunks []swarm.Chunk, root swarm.Address) {
		t.Helper()

		for _, ch := range chunks {
			has, err := mockStorer.ChunkStore().Has(context.Background(), ch.Address())
			if err != nil {
				t.Fatalf("Has(...): unexpected error: %v", err)
			}

			if !has {
				t.Fatalf("expected chunk to be found %s", ch.Address())
			}
		}

		has, err := mockStorer.HasPin(root)
		if err != nil {
			t.Fatalf("HasPin(...): unexpected error: %v", err)
		}

		if !has {
			t.Fatalf("expected to find root pin %s", root)
		}

		pins, err := mockStorer.Pins()
		if err != nil {
			t.Fatalf("Pins(): unexpected error: %v", err)
		}

		found := false
		for _, p := range pins {
			if p.Equal(root) {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("expected to find pin in list %s", root)
		}
	}

	t.Run("upload", func(t *testing.T) {
		session, err := mockStorer.NewSession()
		if err != nil {
			t.Fatalf("NewSession(): unexpected error: %v", err)
		}

		chunks := chunktesting.GenerateTestRandomChunks(5)
		putter, err := mockStorer.Upload(context.Background(), true, session.TagID)
		if err != nil {
			t.Fatalf("Upload(...): unexpected error: %v", err)
		}

		for _, ch := range chunks {
			err = putter.Put(context.Background(), ch)
			if err != nil {
				t.Fatalf("Put(...): unexpected error: %v", err)
			}
		}

		err = putter.Done(chunks[0].Address())
		if err != nil {
			t.Fatalf("Done(...): unexpected error: %v", err)
		}

		have, err := mockStorer.Session(session.TagID)
		if err != nil {
			t.Fatalf("Session(...): unexpected error: %v", err)
		}

		want := storer.SessionInfo{
			TagID:     session.TagID,
			StartedAt: session.StartedAt,
			Address:   chunks[0].Address(),
		}

		if diff := cmp.Diff(want, have); diff != "" {
			t.Fatalf("unexpected session info: (-want +have):\n%s", diff)
		}

		verifyChunksAndPin(t, chunks, chunks[0].Address())
	})

	t.Run("pin", func(t *testing.T) {
		putter, err := mockStorer.NewCollection(context.Background())
		if err != nil {
			t.Fatalf("NewCollection(): unexpected error: %v", err)
		}

		chunks := chunktesting.GenerateTestRandomChunks(5)
		for _, ch := range chunks {
			err = putter.Put(context.Background(), ch)
			if err != nil {
				t.Fatalf("Put(...): unexpected error: %v", err)
			}
		}

		err = putter.Done(chunks[0].Address())
		if err != nil {
			t.Fatalf("Done(...): unexpected error: %v", err)
		}

		verifyChunksAndPin(t, chunks, chunks[0].Address())
	})

	t.Run("direct upload", func(t *testing.T) {
		putter := mockStorer.DirectUpload()

		chunks := chunktesting.GenerateTestRandomChunks(5)
		done := make(chan struct{})
		go func() {
			defer close(done)

			i := 0
			for op := range mockStorer.PusherFeed() {
				if !op.Chunk.Equal(chunks[i]) {
					op.Err <- fmt.Errorf("invalid chunk %s", op.Chunk.Address())
					break
				}
				op.Err <- nil
				i++
				if i == len(chunks) {
					break
				}
			}
		}()

		for _, ch := range chunks {
			err := putter.Put(context.Background(), ch)
			if err != nil {
				t.Fatalf("Put(...): unexpected error: %v", err)
			}
		}

		err := putter.Done(swarm.ZeroAddress)
		if err != nil {
			t.Fatalf("Done(...): unexpected error: %v", err)
		}

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("expected to be done by now")
		}
	})

	t.Run("delete pin", func(t *testing.T) {
		pins, err := mockStorer.Pins()
		if err != nil {
			t.Fatalf("Pins(): unexpected error: %v", err)
		}

		for _, p := range pins {
			err := mockStorer.DeletePin(context.Background(), p)
			if err != nil {
				t.Fatalf("DeletePin(...): unexpected error: %v", err)
			}
		}

		for _, p := range pins {
			has, err := mockStorer.HasPin(p)
			if err != nil {
				t.Fatalf("HasPin(...): unexpected error: %v", err)
			}

			if has {
				t.Fatalf("expected pin to be deleted %s", p)
			}
		}

		pins, err = mockStorer.Pins()
		if err != nil {
			t.Fatalf("Pins(): unexpected error: %v", err)
		}
		if len(pins) != 0 {
			t.Fatalf("expected all pins to be deleted, found %d", len(pins))
		}
	})
}
