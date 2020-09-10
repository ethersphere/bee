// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multierror

import (
	"errors"
	"fmt"
	"strings"
)

// Ensure multierror implements Error interface.
var _ error = (*Error)(nil)

// Error is an error type to track multiple errors. This can be used to
// accumulate errors and return them as a single "error" type.
type Error struct {
	Errors []error
}

// New will return a new multierror.
func New(errs ...error) *Error {
	e := Error{}
	e.Append(errs...)

	return &e
}

func (e *Error) Error() string {
	return format(e.Errors)
}

// Append will append errors to the multierror.
func (e *Error) Append(errs ...error) {
	e.Errors = append(e.Errors, errs...)
}

// ErrorOrNil returns an error interface if the multierror represents a list of
// errors or nil if the list is empty.
func (e *Error) ErrorOrNil() error {
	if e == nil || len(e.Errors) == 0 {
		return nil
	}

	return e
}

// WrapErrorOrNil will wrap the given error if the multierror contains errors.
func (e *Error) WrapErrorOrNil(toWrap error) error {
	if err := e.ErrorOrNil(); err != nil {
		return fmt.Errorf("%v: %w", err, toWrap)
	}
	return nil
}

// Unwrap returns an error from Error (or nil if there are no errors).
// The error returned supports Unwrap, so that the entire chain of errors can
// be unwrapped. The order will match the order of Errors at the time of
// calling.
//
// This will perform a shallow copy of the errors slice. Any errors appended
// to this error after calling Unwrap will not be available until a new
// Unwrap is called on the multierror.Error.
func (e *Error) Unwrap() error {
	if e == nil || len(e.Errors) == 0 {
		return nil
	}

	if len(e.Errors) == 0 {
		return e.Errors[0]
	}

	// Shallow copy the error slice.
	errs := make([]error, len(e.Errors))
	copy(errs, e.Errors)
	return chain(errs)
}

type chain []error

// Error implements the error interface.
func (ec chain) Error() string {
	return ec[0].Error()
}

// Unwrap implements errors.Unwrap by returning the next error in the chain or
// nil if there are no more errors.
func (ec chain) Unwrap() error {
	if len(ec) == 1 {
		return nil
	}

	// Return the rest of the chain.
	return ec[1:]
}

// As implements errors.As by attempting to map the current value.
func (ec chain) As(target interface{}) bool {
	return errors.As(ec[0], target)
}

// Is implements errors.Is by comparing the current value directly.
func (ec chain) Is(target error) bool {
	return errors.Is(ec[0], target)
}

func format(errs []error) string {
	if len(errs) == 1 {
		return fmt.Sprintf("1 error occurred: %s", errs[0])
	}

	msgs := make([]string, len(errs))
	for i, err := range errs {
		msgs[i] = fmt.Sprintf("%s", err)
	}

	return fmt.Sprintf("%d errors occurred: %s",
		len(errs), strings.Join(msgs, ", "))

}
