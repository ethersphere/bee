// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bee

import (
	"strconv"
	"time"
)

var (
	version    = "1.1.0" // manually set semantic version number
	commitHash string    // automatically set git commit hash
	commitTime string    // automatically set git commit time

	Version = func() string {
		if commitHash != "" {
			return version + "-" + commitHash
		}
		return version + "-dev"
	}()

	// CommitTime returns the time of the commit from which this code was derived.
	// If it's not set (in the case of running the code directly without compilation)
	// then the current time will be returned.
	CommitTime = func() string {
		if commitTime == "" {
			commitTime = strconv.Itoa(int(time.Now().Unix()))
		}
		return commitTime
	}
)
