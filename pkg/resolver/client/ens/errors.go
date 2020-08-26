// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens

import "fmt"

// ErrNotImplemented denotes that the function has not been implemented.
type ErrNotImplemented struct{}

// Error returns the fomatted not implemented error message.
func (e ErrNotImplemented) Error() string {
	return "function not implemented"
}

// ErrInvalidContentHash denotes that the value of the contenthash record is
// not valid.
type ErrInvalidContentHash string

// Error returns the formatted invalid content hash error.
func (e ErrInvalidContentHash) Error() string {
	return fmt.Sprintf("%q is not a valid swarm ENS content hash value", string(e))
}
