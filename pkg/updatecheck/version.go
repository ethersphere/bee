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
	// releaseTagRe matches a plain release tag, such as 2.8.2 or v2.8.2.
	releaseTagRe = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)$`)
	// currentVersionRe matches the leading version of bee.Version. That string
	// has the form "<git describe tag>-<commit hash>", for example
	// "2.8.2-7e703f49" or "2.9.0-rc1-7e703f49-dirty". Only an "-rcN" suffix
	// directly after the patch number counts as a pre-release. Anything else is
	// build metadata and is ignored when ordering versions. releaseOrRCTagRe
	// matches a release or release candidate tag. bee-runner uses this form to
	// order a noRollback barrier. Examples: 2.9.0, v2.9.0-rc1, 2.9.0-rc.1, or a
	// dev build with base 2.9.0-unofficial-<12 hex>.
	releaseOrRCTagRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:-rc\.?\d+)?(?:-unofficial-[0-9a-f]{12})?$`)
	currentVersionRe = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)(?:-rc\.?(\d+))?(?:[-+].*)?$`)
)

// parseReleaseTag parses a plain release tag. Pre-release tags are not releases
// that a node is told to upgrade to.
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
// "-rcN" becomes the pre-release "rc.N". These order numerically (rc.2 comes
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

// parseReleaseOrRC parses a release or release candidate tag exactly as
// bee-runner does for a noRollback barrier.
func parseReleaseOrRC(s string) (*semver.Version, bool) {
	if !releaseOrRCTagRe.MatchString(s) {
		return nil, false
	}
	return parseCurrentVersion(s)
}
