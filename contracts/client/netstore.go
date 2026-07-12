// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/pusher"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// AsNetStore returns a storer.NetStore facade over gRPC for DirectUpload and Download.
// PusherFeed is delegated to local because it is not part of the storage contract.
func (c *Client) AsNetStore(local storer.NetStore) storer.NetStore {
	return &netStoreAdapter{client: c, local: local}
}

type netStoreAdapter struct {
	client *Client
	local  storer.NetStore
}

func (a *netStoreAdapter) Download(cache bool) storage.Getter {
	return a.client.Download(cache)
}

func (a *netStoreAdapter) DirectUpload() storer.PutterSession {
	sess, err := a.client.DirectUpload()
	if err != nil {
		return &failedPutterSession{err: err}
	}
	return sess
}

func (a *netStoreAdapter) PusherFeed() <-chan *pusher.Op {
	return a.local.PusherFeed()
}

type failedPutterSession struct {
	err error
}

func (f *failedPutterSession) Put(context.Context, swarm.Chunk) error { return f.err }

func (f *failedPutterSession) Done(swarm.Address) error { return f.err }

func (f *failedPutterSession) Cleanup() error { return f.err }
