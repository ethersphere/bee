// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package updatecheck

import (
	"regexp"
	"strconv"

	"github.com/coreos/go-semver/semver"
)

var (
	// releaseTagRe matches a plain release tag: 2.8.2 or v2.8.2.
	releaseTagRe = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)$`)
	// currentVersionRe matches the leading version of bee.Version, which is
	// "<git describe tag>-<commit hash>", e.g. "2.8.2-7e703f49" or
	// "2.9.0-rc1-7e703f49-dirty". Only an "-rcN" suffix directly after the
	// patch number is treated as a pre-release; anything else is build
	// metadata and does not take part in ordering.
	currentVersionRe = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)(?:-rc\.?(\d+))?(?:[-+].*)?$`)
)

// parseReleaseTag parses a plain release tag. Pre-release tags are not
// releases a node is told to upgrade to.
func parseReleaseTag(s string) (*semver.Version, bool) {
	m := releaseTagRe.FindStringSubmatch(s)
	if m == nil {
		return nil, false
	}
	v, err := semver.NewVersion(m[1])
	if err != nil {
		return nil, false
	}
	return v, true
}

// parseCurrentVersion parses the running bee version. A release candidate
// "-rcN" becomes the pre-release "rc.N", which orders numerically (rc.2
// before rc.10) and before the final release.
func parseCurrentVersion(s string) (*semver.Version, bool) {
	m := currentVersionRe.FindStringSubmatch(s)
	if m == nil {
		return nil, false
	}
	v, err := semver.NewVersion(m[1])
	if err != nil {
		return nil, false
	}
	if m[2] != "" {
		rc, err := strconv.ParseUint(m[2], 10, 32)
		if err != nil || rc == 0 {
			return nil, false
		}
		v.PreRelease = semver.PreRelease("rc." + strconv.FormatUint(rc, 10))
	}
	return v, true
}
