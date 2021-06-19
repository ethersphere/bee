// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bee

var CommitTime string

var (
	version = "1.0.0" // manually set semantic version number
	commit  string    // automatically set git commit hash

	Version = func() string {
		if commit != "" {
			return version + "-" + commit
		}
		return version + "-dev"
	}()
)
