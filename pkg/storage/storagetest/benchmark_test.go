// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

const (
	cr = 0.5
	vs = 100
)

var (
	format = "100000000000000%d"
)

func TestCompressibleBytes(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(time.Now().Unix()))

	bts := compressibleBytes(rng, cr, vs)
	if !bytes.Equal(bts[:50], bts[50:]) {
		t.Errorf("expected \n%s to equal \n%s", string(bts[:50]), string(bts[50:]))
	}

	bts = compressibleBytes(rng, 0.25, vs)
	if !bytes.Equal(bts[:25], bts[25:50]) || !bytes.Equal(bts[50:75], bts[75:]) {
		t.Errorf("expected \n%s to equal \n%s", string(bts[:50]), string(bts[50:]))
	}
}

func TestRandomValueGenerator(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(time.Now().Unix()))

	t.Run("generates random values", func(t *testing.T) {
		gen := makeRandomValueGenerator(rng, cr, vs)
		if bytes.Equal(gen.Value(1), gen.Value(2)) {
			t.Fatal("expected values to be random, got same value")
		}
	})

	t.Run("respects value size", func(t *testing.T) {
		gen := makeRandomValueGenerator(rng, cr, 10)
		if len(gen.Value(1)) != 10 {
			t.Fatal("expected values size to be 10, got", len(gen.Value(1)))
		}
	})
}

func TestFullRandomEntryGenerator(t *testing.T) {
	t.Parallel()

	t.Run("startAt is respected", func(t *testing.T) {
		startAt, size := 10, 100
		gen := newFullRandomEntryGenerator(startAt, size)
		minVal := []byte("1000000000000010")
		for i := 0; i < gen.NKey(); i++ {
			if bytes.Compare(minVal, gen.Key(i)) > 0 {
				t.Fatalf("%s should not be lower than %s", gen.Key(i), minVal)
			}
		}
	})
}

func TestSequentialEntryGenerator(t *testing.T) {
	t.Parallel()

	t.Run("generated values are consecutive ascending", func(t *testing.T) {
		gen := newSequentialEntryGenerator(10)
		for i := 0; i < gen.NKey(); i++ {
			expected := fmt.Sprintf(format, i)
			if expected != string(gen.Key(i)) {
				t.Fatalf("%s expected to equal %s", expected, string(gen.Key(i)))
			}
		}
	})
}

func TestReverseGenerator(t *testing.T) {
	t.Parallel()

	t.Run("generated values are consecutive descending", func(t *testing.T) {
		gen := newReversedKeyGenerator(newSequentialKeyGenerator(10))
		for i := 0; i < gen.NKey(); i++ {
			expected := fmt.Sprintf(format, 9-i)
			if expected != string(gen.Key(i)) {
				t.Fatalf("%s expected to equal %s", expected, string(gen.Key(i)))
			}
		}
	})
}

func TestStartAtEntryGenerator(t *testing.T) {
	t.Parallel()

	t.Run("generated values are consecutive ascending", func(t *testing.T) {
		startAt := 5
		gen := newStartAtEntryGenerator(startAt, newSequentialEntryGenerator(10))
		for i := 0; i < gen.NKey(); i++ {
			expected := fmt.Sprintf(format, i+startAt)
			if expected != string(gen.Key(i)) {
				t.Fatalf("%s expected to equal %s", expected, string(gen.Key(i)))
			}
		}
	})
}

func TestRoundKeyGenerator(t *testing.T) {
	t.Parallel()

	t.Run("repeating values are generated", func(t *testing.T) {
		gen := newRoundKeyGenerator(newRandomKeyGenerator(100))
		v := string(gen.Key(50))
		var found bool
		for i := 50; i < gen.NKey()+50; i++ {
			if v == string(gen.Key(i)) {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("repeating values not found")
		}
	})
}
