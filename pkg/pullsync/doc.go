// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package pullsync contains protocol that is used to ensure that there is correct
chunk replication in the neighborhood of the node.

The protocol is used to exchange information about what chunks are stored on
other nodes and if necessary pull any chunks it needs. It also takes care that
chunks are not synced twice.

The pullsync protocol uses Protobuf messages for encoding. It then exposes
several functions which use those messages to start the exchange of other node
cursors using `GetCursors` function, after which node can schedule syncing
of chunks using `SyncInterval` function, and in the case of any errors or
timed-out operations cancel syncing using `CancelRuid`.
*/
package pullsync
