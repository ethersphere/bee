// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"context"
	"io"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

type PutterCloser interface {
	io.Closer
	storage.Putter
}

type PinStore interface {
	NewCollection(context.Context, swarm.Address) (PutterCloser, error)
	Pins(context.Context) ([]swarm.Address, error)
	HasPin(context.Context) (bool, error)
	DeletePin(context.Context, swarm.Address) error
}
