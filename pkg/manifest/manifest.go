// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package manifest

import (
	"errors"
	"net/http"

	"github.com/ethersphere/bee/pkg/swarm"
)

var ErrNotFound = errors.New("manifest: not found")

type Parser interface {
	Parse(bytes []byte) (Interface, error)
}

type Interface interface {
	Add(string, Entry)
	Remove(string)
	FindEntry(string) (Entry, error)
	Serialize() ([]byte, error)
}

type Entry interface {
	GetAddress() swarm.Address
	GetName() string
	GetHeaders() http.Header
}
