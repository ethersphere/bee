// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package updatecheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// maxBinarySize is bee-runner's download limit. A download of this many
	// bytes or more is refused.
	maxBinarySize = 512 << 20
	// stageTimeout bounds one pre-staging download. A cold download of ~75 MB
	// through a Swarm gateway takes a minute or two.
	stageTimeout = 15 * time.Minute
	// partialPrefix names an unfinished download in the runner's cache the same
	// way bee-runner names its own. That way the runner removes one left behind
	// by a crash.
	partialPrefix = ".partial-"
	// digestPrefix starts every digest in a release descriptor.
	digestPrefix = "sha256:"
)

var (
	errStageDigest   = errors.New("updatecheck: staged binary digest mismatch")
	errStageNoBinary = errors.New("updatecheck: release lists no binary for this platform")
	errStageTooLarge = errors.New("updatecheck: binary too large")

	// stageBinaryPattern is the binary path that bee-runner hands over:
	// binaries/<platform>/<name>.
	stageBinaryPattern = regexp.MustCompile(`^binaries/[a-z0-9_-]+/[a-zA-Z0-9._-]+$`)
	digestPattern      = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// stager pre-stages the next release. While bee is still running, it downloads
// the binary into bee-runner's cache, under the name the runner looks for (the
// lowercase hex of its sha256). At its next start the runner finds the file and
// downloads nothing. The runner re-verifies every cache entry against the
// signed descriptor, so a staged file is never trusted just because bee wrote
// it.
type stager struct {
	client *http.Client
	dir    string // bee-runner's cache, absolute
	binary string // the binary's path in a release
	url    string // the binary's URL in the registry
}

// newStager returns a stager for the cache and binary that bee-runner handed
// over. It returns nil if the runner handed over none, or if they are not
// usable.
func newStager(r Runner, base *url.URL, client *http.Client) (*stager, error) {
	if r.Cache == "" && r.Binary == "" {
		return nil, nil
	}
	if !filepath.IsAbs(r.Cache) || filepath.Clean(r.Cache) != r.Cache {
		return nil, errors.New("updatecheck: bee-runner cache is not a clean absolute path")
	}
	if !stageBinaryPattern.MatchString(r.Binary) || strings.Contains(r.Binary, "..") {
		return nil, errors.New("updatecheck: bee-runner binary path is not binaries/<platform>/<name>")
	}
	return &stager{client: client, dir: r.Cache, binary: r.Binary, url: base.JoinPath(r.Binary).String()}, nil
}

// stage makes sure the binary with digest is in the cache. It reports whether
// it was already there.
func (st *stager) stage(ctx context.Context, digest string) (cached bool, err error) {
	if !digestPattern.MatchString(digest) {
		return false, fmt.Errorf("%w: malformed digest", errStageNoBinary)
	}
	dst := filepath.Join(st.dir, strings.TrimPrefix(digest, digestPrefix))
	if err := verifyFile(dst, digest); err == nil {
		return true, nil
	}

	ctx, cancel := context.WithTimeout(ctx, stageTimeout)
	defer cancel()
	return false, st.download(ctx, dst, digest)
}

// download streams the binary into a temporary file next to dst, checks its
// digest, and renames it into place. The runner therefore never sees a partial
// or unverified file under the name it looks for.
func (st *stager) download(ctx context.Context, dst, digest string) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, st.url, nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	resp, err := st.client.Do(req)
	if err != nil {
		return fmt.Errorf("get %s: %w", st.binary, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: %w: %d", st.binary, errNotOK, resp.StatusCode)
	}

	f, err := os.CreateTemp(st.dir, partialPrefix+"*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxBinarySize))
	if err != nil {
		return fmt.Errorf("get %s: %w", st.binary, err)
	}
	if n >= maxBinarySize {
		return fmt.Errorf("get %s: %w", st.binary, errStageTooLarge)
	}
	if got := digestPrefix + hex.EncodeToString(h.Sum(nil)); got != digest {
		return fmt.Errorf("get %s: %w", st.binary, errStageDigest)
	}
	if err = f.Chmod(0o755); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), dst)
}

// verifyFile checks that path is a regular file with the given digest.
func verifyFile(path, digest string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return errors.New("updatecheck: staged binary is not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, maxBinarySize)); err != nil {
		return err
	}
	if digestPrefix+hex.EncodeToString(h.Sum(nil)) != digest {
		return errStageDigest
	}
	return nil
}
