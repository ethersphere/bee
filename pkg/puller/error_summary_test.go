// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package puller

import (
	"errors"
	"testing"
)

func TestCountErrors(t *testing.T) {
	t.Parallel()

	t.Run("nil error", func(t *testing.T) {
		result := countErrors(nil)
		if result != 0 {
			t.Errorf("expected 0 for nil error, got %d", result)
		}
	})

	t.Run("single error", func(t *testing.T) {
		err := errors.New("test error")
		result := countErrors(err)
		if result != 1 {
			t.Errorf("expected 1, got %d", result)
		}
	})

	t.Run("two different errors", func(t *testing.T) {
		err1 := errors.New("error one")
		err2 := errors.New("error two")
		joined := errors.Join(err1, err2)
		result := countErrors(joined)

		if result != 2 {
			t.Errorf("expected 2, got %d", result)
		}
	})

	t.Run("many identical errors", func(t *testing.T) {
		baseErr := errors.New("storage: not found")
		var joined error
		for range 100 {
			joined = errors.Join(joined, baseErr)
		}

		result := countErrors(joined)

		if result != 100 {
			t.Errorf("expected 100, got %d", result)
		}
	})

	t.Run("mixed repeated errors", func(t *testing.T) {
		err1 := errors.New("storage: not found")
		err2 := errors.New("invalid stamp")
		var joined error
		for range 50 {
			joined = errors.Join(joined, err1)
		}
		for range 30 {
			joined = errors.Join(joined, err2)
		}

		result := countErrors(joined)

		if result != 80 {
			t.Errorf("expected 80, got %d", result)
		}
	})
}
