// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens

import (
	"errors"
)

var (
	// ErrNotImplemented denotes that the function has not been implemented.
	ErrNotImplemented = errors.New("function not implemented")
	// ErrInvalidContentHash denotes that the value of the contenthash record is
	// not valid.
	ErrInvalidContentHash = errors.New("invalid swarm content hash")
)
