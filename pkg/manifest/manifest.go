// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package manifest

import (
	"errors"

	"github.com/ethersphere/bee/pkg/swarm"
)

var ErrNotFound = errors.New("manifest: not found")

type Parser interface {
	Parse(bytes []byte) (Interface, error)
}

type Interface interface {
	FindEntry(path string) (*Entry, error)
	Serialize() []byte
}

type Entry struct {
	Address  swarm.Address `json:"address"`
	Filename string        `json:"filename"`
	MimeType string        `json:"mimetype"`
}
