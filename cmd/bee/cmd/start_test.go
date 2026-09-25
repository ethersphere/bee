// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/cmd/bee/cmd"
)

func TestCheckPostageSnapshotOptions(t *testing.T) {
	t.Parallel()

	const wantErr = "postage-snapshot-file and skip-postage-snapshot cannot be used together"

	for _, tc := range []struct {
		name    string
		skip    bool
		file    string
		wantErr bool
	}{
		{name: "neither set"},
		{name: "only skip", skip: true},
		{name: "only file", file: "/data/snapshot.ndjson.gz"},
		{name: "both set", skip: true, file: "/data/snapshot.ndjson.gz", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := cmd.CheckPostageSnapshotOptions(tc.skip, tc.file)
			switch {
			case tc.wantErr && (err == nil || err.Error() != wantErr):
				t.Fatalf("got error %v, want %q", err, wantErr)
			case !tc.wantErr && err != nil:
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
