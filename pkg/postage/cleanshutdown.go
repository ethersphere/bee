// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import "github.com/ethersphere/bee/v2/pkg/storage"

// cleanShutdownItem is a storage.Item implementation for marking graceful shutdown.
type cleanShutdownItem struct{}

// ID implements the storage.Item interface.
func (c *cleanShutdownItem) ID() string {
	return "clean_shutdown"
}

// Namespace implements the storage.Item interface.
func (c *cleanShutdownItem) Namespace() string {
	return "postage"
}

// Marshal implements the storage.Item interface.
func (c *cleanShutdownItem) Marshal() ([]byte, error) {
	return []byte{0}, nil
}

// Unmarshal implements the storage.Item interface.
func (c *cleanShutdownItem) Unmarshal(data []byte) error {
	return nil
}

// Clone implements the storage.Item interface.
func (c *cleanShutdownItem) Clone() storage.Item {
	if c == nil {
		return nil
	}
	return &cleanShutdownItem{}
}

// String implements the fmt.Stringer interface.
func (c cleanShutdownItem) String() string {
	return "postage/clean_shutdown"
}
