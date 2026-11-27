// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package contractversion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/contractversion"
	"github.com/ethersphere/bee/v2/pkg/log"
)

type stubBackend struct {
	block uint64
	err   error
}

func (s *stubBackend) BlockNumber(context.Context) (uint64, error) { return s.block, s.err }

func TestNew(t *testing.T) {
	t.Parallel()

	tests := map[string][]contractversion.Version[string]{
		"empty":       nil,
		"first not 0": {{FromBlock: 1, Impl: "a"}},
		"equal":       {{FromBlock: 0, Impl: "a"}, {FromBlock: 0, Impl: "b"}},
		"decreasing":  {{FromBlock: 0, Impl: "a"}, {FromBlock: 5, Impl: "b"}, {FromBlock: 3, Impl: "c"}},
	}
	for name, v := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := contractversion.New(log.Noop, "c", &stubBackend{}, v)
			if !errors.Is(err, contractversion.ErrInvalidVersions) {
				t.Fatalf("got %v, want ErrInvalidVersions", err)
			}
		})
	}
}

func TestAt(t *testing.T) {
	t.Parallel()

	s, err := contractversion.New(log.Noop, "c", &stubBackend{}, []contractversion.Version[string]{
		{0, "v0"}, {304, "v1"}, {1000, "v2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for block, want := range map[uint64]string{0: "v0", 303: "v0", 304: "v1", 999: "v1", 1000: "v2", 10_000: "v2"} {
		if got := s.At(block); got != want {
			t.Errorf("At(%d)=%s want %s", block, got, want)
		}
	}
}

func TestActive(t *testing.T) {
	t.Parallel()

	b := &stubBackend{}
	s, err := contractversion.New(log.Noop, "c", b, []contractversion.Version[string]{{0, "v0"}, {304, "v1"}})
	if err != nil {
		t.Fatal(err)
	}
	for block, want := range map[uint64]string{303: "v0", 304: "v1", 100: "v0"} {
		b.block = block
		got, err := s.Active(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("block %d: got %s want %s", block, got, want)
		}
	}

	stubErr := errors.New("boom")
	b.err = stubErr
	if _, err := s.Active(context.Background()); !errors.Is(err, stubErr) {
		t.Fatalf("got %v, want stub error", err)
	}
}
