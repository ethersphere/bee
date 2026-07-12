// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package contracts

// LogPrefix is a stable grep target for storage contract gRPC traffic in bee logs.
const LogPrefix = "storage contract grpc"

// LogMarkerUpload is a grep target for file/chunk upload via the gRPC storage client.
const LogMarkerUpload = "storage contract grpc upload"

// LogMarkerDownload is a grep target for file/chunk download via the gRPC storage client.
const LogMarkerDownload = "storage contract grpc download"
