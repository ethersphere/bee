// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

// Trio is type for group of three elements.
type Trio[T any] struct {
	Element1 T
	Element2 T
	Element3 T
}
