// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package swarm_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
)

func TestValidator(t *testing.T) {
	addr := swarm.MustParseHexAddress("deadbeef")
	data := []byte("deadbeef")
	expChunk := swarm.NewChunk(addr, data)

	c := make(chan int)
	newTestCallback := func(t *testing.T, wantcb bool, i int) func(ctx context.Context, ch swarm.Chunk) {

		if !wantcb {
			return nil
		}
		return func(ctx context.Context, ch swarm.Chunk) {
			if !ch.Equal(expChunk) {
				t.Fatalf("wrong chunk passed to valid func")
			}
			c <- i
		}
	}
	newTestValidFunc := func(t *testing.T, v bool) func(swarm.Chunk) bool {
		return func(ch swarm.Chunk) bool {
			if !ch.Equal(expChunk) {
				t.Fatalf("wrong chunk passed to valid func")
			}
			return v
		}
	}
	newTestValidator := func(t *testing.T, v bool, wantcb bool, i int) swarm.Validator {
		return swarm.Validator{
			Valid:    newTestValidFunc(t, v),
			Callback: newTestCallback(t, wantcb, i),
		}
	}

	checkValidatorIndex := func(expectCallback bool, i int) {
		select {
		case j := <-c:
			if expectCallback {
				if j != i {
					t.Fatalf("incorrect validator callback called; expected %d, got %d", i, j)
				}
			} else {
				t.Fatal("unexpected callback")
			}
		case <-time.After(100 * time.Millisecond):
			if expectCallback {
				t.Fatalf("timed out waiting for callback from validator %d", i)
			}
		}
	}

	cases := []struct {
		name     string
		valid    bool
		callback bool
	}{
		{"valid chunk, callback", true, true},
		{"valid chunk, no callback", true, false},
		{"invalid chunk, callback", false, true},
		{"invalid chunk, no callback", false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			validf := swarm.NewValidator(newTestValidator(t, c.valid, c.callback, 0))
			if valid := validf(context.Background(), expChunk); valid != c.valid {
				t.Fatalf("incorrect validation  response; expected %v, got %v", c.valid, valid)
			}
			checkValidatorIndex(c.valid && c.callback, 0)
		})
	}
	t.Run("multiple validators, valid chunk", func(t *testing.T) {
		validf := swarm.NewValidator(
			newTestValidator(t, false, true, 0),
			newTestValidator(t, false, true, 1),
			newTestValidator(t, true, true, 2),
			newTestValidator(t, true, true, 3),
		)
		if !validf(context.Background(), expChunk) {
			t.Fatal("expected valid chunk")
		}
		checkValidatorIndex(true, 2)
	})
	t.Run("multiple validators, invalid chunk", func(t *testing.T) {
		validf := swarm.NewValidator(
			newTestValidator(t, false, true, 0),
			newTestValidator(t, false, true, 1),
		)
		if validf(context.Background(), expChunk) {
			t.Fatal("expected invalid chunk")
		}
		checkValidatorIndex(false, 0)
	})
}
