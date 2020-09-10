// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package multierror_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/resolver/multiresolver/multierror"
)

// nestedError implements error and is used for tests.
type nestedError struct{}

func (*nestedError) Error() string { return "" }

func TestErrorError(t *testing.T) {
	t.Run("one error", func(t *testing.T) {
		want := "1 error occurred: foo"

		multi := multierror.New(errors.New("foo"))
		if multi.Error() != want {
			t.Fatalf("got: %q, want %q", multi.Error(), want)
		}
	})
	t.Run("multiple errors", func(t *testing.T) {
		want := "2 errors occurred: foo, bar"

		multi := multierror.New(
			errors.New("foo"),
			errors.New("bar"),
		)
		if multi.Error() != want {
			t.Fatalf("got: %q, want %q", multi.Error(), want)
		}
	})
}

func TestErrorErrorOrNil(t *testing.T) {
	err := multierror.New()
	if err.ErrorOrNil() != nil {
		t.Fatalf("bad: %#v", err.ErrorOrNil())
	}

	err.Errors = []error{errors.New("foo")}
	if got := err.ErrorOrNil(); got == nil {
		t.Fatal("should not be nil")
	} else if got != err {
		t.Fatalf("bad: %#v", got)
	}
}

func TestErrorWrapErrorOrNil(t *testing.T) {
	wrapErr := errors.New("wrapper error")

	err := multierror.New()
	if err.WrapErrorOrNil(wrapErr) != nil {
		t.Fatalf("bad: %#v", err.ErrorOrNil())
	}

	multi := multierror.New(
		errors.New("foo"),
		errors.New("bar"),
	)
	if got := multi.WrapErrorOrNil(wrapErr); got == nil {
		t.Fatal("should not be nil")
	} else if !errors.Is(got, wrapErr) {
		t.Fatalf("bad: %#v", got)
	}
}

func TestErrorUnwrap(t *testing.T) {
	t.Run("with errors", func(t *testing.T) {
		err := multierror.New(
			errors.New("foo"),
			errors.New("bar"),
			errors.New("baz"),
		)

		var current error = err
		for i := 0; i < len(err.Errors); i++ {
			current = errors.Unwrap(current)
			if !errors.Is(current, err.Errors[i]) {
				t.Fatal("should be next value")
			}
		}

		if errors.Unwrap(current) != nil {
			t.Fatal("should be nil at the end")
		}
	})

	t.Run("with no errors", func(t *testing.T) {
		err := multierror.New()
		if errors.Unwrap(err) != nil {
			t.Fatal("should be nil")
		}
	})

	t.Run("with nil multierror", func(t *testing.T) {
		var err *multierror.Error
		if errors.Unwrap(err) != nil {
			t.Fatal("should be nil")
		}
	})
}

func TestErrorIs(t *testing.T) {
	errBar := errors.New("bar")

	t.Run("with errBar", func(t *testing.T) {
		err := multierror.New(
			errors.New("foo"),
			errBar,
			errors.New("baz"),
		)

		if !errors.Is(err, errBar) {
			t.Fatal("should be true")
		}
	})

	t.Run("with errBar wrapped by fmt.Errorf", func(t *testing.T) {
		err := multierror.New(
			errors.New("foo"),
			fmt.Errorf("errorf: %w", errBar),
			errors.New("baz"),
		)

		if !errors.Is(err, errBar) {
			t.Fatal("should be true")
		}
	})

	t.Run("without errBar", func(t *testing.T) {
		err := multierror.New(
			errors.New("foo"),
			errors.New("baz"),
		)

		if errors.Is(err, errBar) {
			t.Fatal("should be false")
		}
	})
}

func TestErrorAs(t *testing.T) {
	match := &nestedError{}

	t.Run("with the value", func(t *testing.T) {
		err := multierror.New(
			errors.New("foo"),
			match,
			errors.New("baz"),
		)

		var target *nestedError
		if !errors.As(err, &target) {
			t.Fatal("should be true")
		}
		if target == nil {
			t.Fatal("target should not be nil")
		}
	})

	t.Run("with the value wrapped by fmt.Errorf", func(t *testing.T) {
		err := multierror.New(
			errors.New("foo"),
			fmt.Errorf("errorf: %w", match),
			errors.New("baz"),
		)

		var target *nestedError
		if !errors.As(err, &target) {
			t.Fatal("should be true")
		}
		if target == nil {
			t.Fatal("target should not be nil")
		}
	})

	t.Run("without the value", func(t *testing.T) {
		err := multierror.New(
			errors.New("foo"),
			errors.New("baz"),
		)

		var target *nestedError
		if errors.As(err, &target) {
			t.Fatal("should be false")
		}
		if target != nil {
			t.Fatal("target should be nil")
		}
	})
}
