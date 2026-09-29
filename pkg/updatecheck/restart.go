// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package updatecheck

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
)

const (
	// DefaultRolloutWindow is the rollout window of a release that does not
	// set one.
	DefaultRolloutWindow = 24 * time.Hour
	// maxRolloutWindow bounds the rollout window a release may ask for.
	maxRolloutWindow = 30 * 24 * time.Hour

	// A node whose slot has already passed restarts after a random delay of
	// a tenth of the window, bounded by minLateJitter and maxLateJitter, so
	// that late nodes do not all restart at once.
	minLateJitter = time.Minute
	maxLateJitter = time.Hour

	// While a restart is pending, the registry is polled every quarter of
	// the rollout window, but not more often than minPollInterval.
	minPollInterval = time.Minute

	// MarkerFileName is the file in the data directory that records the
	// release a restart was made for.
	MarkerFileName = "update-restart.json"
	maxMarkerSize  = 4 << 10

	// maxRestartAttempts is how many restarts are made for one release that
	// bee-runner does not start, such as when the runner could not reach the
	// registry. retryBackoff is the least time between two of them.
	maxRestartAttempts = 2
	retryBackoff       = 30 * time.Minute

	// gateRounds is how many storage incentives rounds a restart waits for a
	// safe point before restarting anyway.
	gateRounds = 2
	// gatePollInterval is how often the safe point is re-evaluated.
	gatePollInterval = 30 * time.Second
)

var errMarkerNotRegular = errors.New("updatecheck: restart marker is not a regular file")

// Gate reports whether now is a good moment to restart and, if not, why.
type Gate func() (safe bool, reason string)

// RestartOptions configure the opt-in restart when a newer release is
// available. The restart is only active when Enabled is set and bee was
// started by bee-runner with a release version and a valid release signing
// key: bee then exits cleanly and relies on systemd, Docker or Kubernetes
// restarting the runner, which fetches, verifies and execs the newest release.
type RestartOptions struct {
	// Enabled is the update-restart option.
	Enabled bool
	// DataDir holds the restart marker file. Restart is inactive without it.
	DataDir string
	// Gate, when set, delays the restart until it reports a safe point, for
	// at most gateRounds rounds of RoundDuration.
	Gate Gate
	// RoundDuration is the storage incentives round length, from the
	// chain's block time. It is required with Gate.
	RoundDuration time.Duration
	// Shutdown triggers the node's graceful shutdown. It is called on its own
	// goroutine and must not wait for this service to close.
	Shutdown func()
}

// marker records the release a restart was made for. TargetVersion, the
// release descriptor version, is what counts; Target is its semver tag, kept
// for logs. Attempts counts the restarts made for it.
type marker struct {
	TargetVersion uint64 `json:"targetVersion"`
	Target        string `json:"target"`
	At            string `json:"at"`
	Attempts      int    `json:"attempts"`
}

// restartState is the part of Service that implements the restart. The
// fields from pending on are guarded by Service.mu; the others are set before
// the first check and read-only afterwards.
type restartState struct {
	active        bool
	overlay       []byte
	dataDir       string
	markerPath    string
	gate          Gate
	roundDuration time.Duration
	shutdown      func()
	stager        *stager // nil when not pre-staging

	pending         bool          // a restart is scheduled or in progress
	pendingPoll     time.Duration // poll interval while pending
	suppressed      bool          // restarts for releases up to suppressVersion are suppressed
	suppressVersion uint64        // release descriptor version
	// retryVersion is a release a previous restart did not deliver, which
	// may be retried not before retryNotBefore; retryAttempts restarts were
	// already made for it.
	retryVersion   uint64
	retryAttempts  int
	retryNotBefore time.Time
}

// suppressUpTo suppresses restarts for releases up to and including version.
func (rs *restartState) suppressUpTo(version uint64) {
	if !rs.suppressed || version > rs.suppressVersion {
		rs.suppressed, rs.suppressVersion = true, version
	}
}

// resolveRestart decides whether the restart is active and, when it is and no
// registry URL is configured, takes the registry of bee-runner. It logs why
// the restart is inactive when it was asked for.
func resolveRestart(logger log.Logger, o *Options, runner, verifiable bool) (bool, error) {
	r := o.Restart
	if !r.Enabled {
		return false, nil
	}
	if r.Shutdown == nil {
		return false, errors.New("updatecheck: update restart needs a shutdown function")
	}
	if r.Gate != nil && r.RoundDuration <= 0 {
		return false, errors.New("updatecheck: update restart gate needs a round duration")
	}

	const inactive = "update-restart is enabled but inactive"
	switch {
	case !o.Runner.Started:
		logger.Warning(inactive + ": bee was not started by bee-runner")
		return false, nil
	case !runner:
		logger.Warning(inactive+": bee-runner reported no valid release version", "runner_version", truncate(o.Runner.Version, maxLoggedValue))
		return false, nil
	case !verifiable:
		logger.Warning(inactive + ": bee-runner handed over no valid release signing key")
		return false, nil
	case r.DataDir == "":
		logger.Warning(inactive + ": no data directory for the restart marker")
		return false, nil
	}
	if _, ok := parseRolledBack(o.Runner); !ok {
		logger.Warning(inactive+": bee-runner rolled back from a release it did not name", "rolled_back_version", truncate(o.Runner.RolledBack, maxLoggedValue))
		return false, nil
	}
	if o.URL == "" {
		o.URL = o.Runner.Registry
	}
	if o.URL == "" {
		logger.Warning(inactive + ": no update-check-url and no registry from bee-runner")
		return false, nil
	}
	return true, nil
}

// parseRolledBack parses BEE_RUNNER_ROLLED_BACK. version is 0 when it is not
// set; ok is false when it is set but not a release version.
func parseRolledBack(r Runner) (version uint64, ok bool) {
	if r.RolledBack == "" {
		return 0, true
	}
	v, err := strconv.ParseUint(r.RolledBack, 10, 64)
	if err != nil || v == 0 {
		return 0, false
	}
	return v, true
}

// initRestart sets up the active restart: it applies a rollback reported by
// bee-runner and the marker of a previous restart.
func (s *Service) initRestart(o Options) {
	r := o.Restart
	rs := &s.restart
	rs.active = true
	rs.overlay = o.Overlay.Bytes()
	rs.dataDir = r.DataDir
	rs.markerPath = filepath.Join(r.DataDir, MarkerFileName)
	rs.gate = r.Gate
	rs.roundDuration = r.RoundDuration
	rs.shutdown = r.Shutdown

	logger := s.logger
	logger.Info("automatic restart for bee updates is enabled",
		"channel", truncate(s.channel, maxLoggedValue),
		"runner_version", s.runnerVersion,
	)

	if rolledBack, _ := parseRolledBack(o.Runner); rolledBack != 0 {
		rs.suppressUpTo(rolledBack)
		logger.Warning("bee-runner rolled back from a failing release; not restarting for it or any older release", "rolled_back_version", rolledBack)
	}

	m, err := readMarker(rs.markerPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		logger.Warning("ignoring unusable update restart marker", "path", rs.markerPath, "error", err)
	case m.TargetVersion == 0 || s.runnerVersion >= m.TargetVersion:
		logger.Info("running the release a previous restart was made for",
			"target", truncate(m.Target, maxLoggedValue), "target_version", m.TargetVersion, "running_version", s.runnerVersion)
		removeMarker(logger, rs.markerPath)
	case max(m.Attempts, 1) < maxRestartAttempts:
		at, err := time.Parse(time.RFC3339, m.At)
		if err != nil || at.After(time.Now()) {
			at = time.Now()
		}
		rs.retryVersion, rs.retryAttempts, rs.retryNotBefore = m.TargetVersion, max(m.Attempts, 1), at.Add(retryBackoff)
		logger.Warning("a previous restart did not deliver the newer release; restarting for it once more",
			"target", truncate(m.Target, maxLoggedValue), "target_version", m.TargetVersion, "running_version", s.runnerVersion,
			"not_before", rs.retryNotBefore.UTC().Format(time.RFC3339))
	default:
		rs.suppressUpTo(m.TargetVersion)
		logger.Warning("previous restarts did not deliver the newer release; not restarting for it again",
			"target", truncate(m.Target, maxLoggedValue), "target_version", m.TargetVersion, "running_version", s.runnerVersion, "attempts", m.Attempts)
	}
	s.setSuppressedMetric()
}

func removeMarker(logger log.Logger, path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Warning("cannot remove update restart marker", "path", path, "error", err)
	}
}

// readMarker reads the marker without following a symbolic link.
func readMarker(path string) (marker, error) {
	var m marker
	fi, err := os.Lstat(path)
	if err != nil {
		return m, err
	}
	if !fi.Mode().IsRegular() {
		return m, errMarkerNotRegular
	}
	if fi.Size() > maxMarkerSize {
		return m, fmt.Errorf("updatecheck: restart marker is %d bytes", fi.Size())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("decode: %w", err)
	}
	return m, nil
}

// writeMarker writes the marker to a new temporary file in the data directory
// and renames it into place, so that a crash never leaves a truncated marker
// and an existing file or symbolic link at the marker path is replaced, never
// written through.
func writeMarker(dir, path string, m marker) (err error) {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, MarkerFileName+".*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(f.Name())
		}
	}()
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// setSuppressedMetric must be called with s.mu held or before the service
// starts.
func (s *Service) setSuppressedMetric() {
	v := 0.0
	if s.restart.suppressed {
		v = 1
	}
	s.metrics.RestartSuppressed.Set(v)
}

// suppressedLocked reports whether a restart for the offered release is
// suppressed, clearing the suppression when its descriptor version is
// strictly higher than the suppressed one. It must be called with s.mu held.
func (s *Service) suppressedLocked(res result) bool {
	rs := &s.restart
	if !rs.suppressed {
		return false
	}
	defer s.setSuppressedMetric()
	if res.latestVersion > rs.suppressVersion {
		rs.suppressed = false
		s.logger.Info("a release newer than the suppressed one is available; update restart allowed again",
			"suppressed_version", rs.suppressVersion, "latest", res.latest, "latest_version", res.latestVersion)
		return false
	}
	s.logger.Debug("update restart suppressed",
		"latest", res.latest, "latest_version", res.latestVersion, "suppressed_version", rs.suppressVersion)
	return true
}

// restartSlot returns the node's place in the rollout window that starts at
// base: base plus the first 8 bytes of sha256(overlay || version), taken
// modulo the window. It is deterministic and spreads a fleet evenly, whenever
// its nodes notice the release.
func restartSlot(overlay []byte, version uint64, base time.Time, window time.Duration) time.Time {
	if window <= 0 {
		return base
	}
	h := sha256.New()
	_, _ = h.Write(overlay)
	var v [8]byte
	binary.BigEndian.PutUint64(v[:], version)
	_, _ = h.Write(v[:])
	sum := h.Sum(nil)
	return base.Add(time.Duration(binary.BigEndian.Uint64(sum[:8]) % uint64(window)))
}

// lateJitter bounds the random delay of a node whose slot has passed.
func lateJitter(window time.Duration) time.Duration {
	return min(max(window/10, minLateJitter), maxLateJitter)
}

// restartPlan is when a restart for a release fires and why.
type restartPlan struct {
	slot time.Time
	late bool      // the slot had passed when the restart was planned
	at   time.Time // when the restart fires
}

// planRestart places the restart for the release in its rollout window. It
// must be called with s.mu held.
func (s *Service) planRestart(res result, now time.Time) restartPlan {
	rs := &s.restart

	base, err := time.Parse(time.RFC3339, res.createdAt)
	if err != nil || base.After(now) {
		// A release without a valid createdAt, or from the future (clock
		// skew or a bogus descriptor), is spread from now, so that it does
		// not postpone the restart past its window.
		s.logger.Debug("spreading the update restart from now", "created_at", truncate(res.createdAt, maxLoggedValue))
		base = now
	}

	p := restartPlan{slot: restartSlot(rs.overlay, res.latestVersion, base, res.window)}
	if now.Before(p.slot) {
		p.at = p.slot
	} else {
		p.late = true
		p.at = now.Add(rand.N(lateJitter(res.window) + 1)) // uniform in [0, jitter]
	}
	if res.latestVersion == rs.retryVersion && p.at.Before(rs.retryNotBefore) {
		p.at = rs.retryNotBefore
	}
	return p
}

// nextInterval is the time until the next check, before jitter: the
// configured interval, shortened while a restart is pending so that a
// withdrawn or newer release is seen before it fires.
func (s *Service) nextInterval() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restart.pending {
		return min(s.interval, s.restart.pendingPoll)
	}
	return s.interval
}

// maybeScheduleRestart schedules a restart for the offered release unless one
// is already pending or restarts for it are suppressed. A pending restart is
// kept as planned; when it fires it restarts for the newest release offered
// then.
func (s *Service) maybeScheduleRestart(res result) {
	if !s.restart.active {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	rs := &s.restart
	if rs.pending || s.suppressedLocked(res) {
		return
	}

	now := time.Now()
	p := s.planRestart(res, now)
	rs.pending = true
	rs.pendingPoll = max(res.window/4, minPollInterval)
	s.metrics.RestartScheduled.Set(float64(p.at.Unix()))
	s.logger.Info("scheduled a restart to update bee",
		"target", res.latest, "target_version", res.latestVersion,
		"window", res.window, "slot", p.slot.UTC().Format(time.RFC3339),
		"late", p.late, "at", p.at.UTC().Format(time.RFC3339))

	s.wg.Add(1)
	go s.restartAt(p.at, res.latestVersion)
}

func (s *Service) clearPending() {
	s.mu.Lock()
	s.restart.pending = false
	s.metrics.RestartScheduled.Set(0)
	s.mu.Unlock()
}

// restartAt waits until at and for a storage incentives safe point, then
// re-checks the release and, if it is still offered, records the marker and
// shuts bee down.
func (s *Service) restartAt(at time.Time, targetVersion uint64) {
	defer s.wg.Done()
	ctx := s.ctx
	rs := &s.restart

	timer := time.NewTimer(time.Until(at))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	// Download the new binary while bee still runs, and before waiting for a
	// safe point so that the download does not use it up. A failure only
	// costs the restart its speed: bee-runner downloads the binary itself.
	s.prestage(ctx, targetVersion)

	if !s.waitSafePoint(ctx) {
		return
	}

	// The registry may have withdrawn the release in the meantime. A
	// canceled restart is planned again by a later check that offers it.
	res, err := s.check(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Info("update restart postponed: cannot re-check the release", "error", err)
			s.clearPending()
		}
		return
	}
	if !res.available || res.latestVersion < targetVersion {
		s.logger.Info("update restart canceled: release no longer offered", append(res.logValues(), "target_version", targetVersion)...)
		s.clearPending()
		return
	}
	s.mu.Lock()
	suppressed := s.suppressedLocked(res)
	attempts := 1
	if res.latestVersion == rs.retryVersion {
		attempts = rs.retryAttempts + 1
	}
	s.mu.Unlock()
	if suppressed {
		s.clearPending()
		return
	}

	m := marker{TargetVersion: res.latestVersion, Target: res.latest, At: time.Now().UTC().Format(time.RFC3339), Attempts: attempts}
	if err := writeMarker(rs.dataDir, rs.markerPath, m); err != nil {
		// Without the marker a release that is never delivered would cause
		// a restart loop, so do not restart.
		s.logger.Error(err, "update restart canceled: cannot write restart marker", "path", rs.markerPath)
		s.clearPending()
		return
	}

	s.logger.Info("restarting to update bee", append(res.logValues(), "notes", truncate(res.notes, maxLoggedNotes))...)
	// The shutdown path closes this service and waits for this goroutine, so
	// it must not be called synchronously from here.
	go rs.shutdown()
}

// waitSafePoint waits until the gate reports a safe point, for at most
// gateRounds rounds. It returns false only when the service is closing.
func (s *Service) waitSafePoint(ctx context.Context) bool {
	rs := &s.restart
	if rs.gate == nil {
		return true
	}

	maxWait := gateRounds * rs.roundDuration
	deadline := time.Now().Add(maxWait)
	for first := true; ; first = false {
		safe, reason := rs.gate()
		if safe {
			return true
		}
		if !time.Now().Before(deadline) {
			s.logger.Warning("no storage incentives safe point found; restarting to update anyway", "waited", maxWait, "reason", reason)
			return true
		}
		if first {
			s.logger.Info("update restart waits for a storage incentives safe point", "reason", reason)
		} else {
			s.logger.Debug("update restart still waits for a storage incentives safe point", "reason", reason)
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(gatePollInterval):
		}
	}
}

// prestage downloads the binary of the release offered now into bee-runner's
// cache, if it is still the target release or a newer one.
func (s *Service) prestage(ctx context.Context, targetVersion uint64) {
	st := s.restart.stager
	if st == nil {
		return
	}
	res, err := s.check(ctx)
	if err != nil || !res.available || res.latestVersion < targetVersion {
		// restartAt re-checks and decides; nothing to stage for now.
		return
	}
	digest, ok := res.files[st.binary]
	if !ok {
		s.metrics.PrestageErrors.Inc()
		s.logger.Warning("not pre-staging the release: it has no binary for this platform", "binary", st.binary, "target_version", res.latestVersion)
		return
	}
	start := time.Now()
	cached, err := st.stage(ctx, digest)
	switch {
	case err != nil:
		if ctx.Err() == nil {
			s.metrics.PrestageErrors.Inc()
			s.logger.Warning("pre-staging the release failed; bee-runner will download it at the restart", "target_version", res.latestVersion, "error", err)
		}
	case cached:
		s.logger.Info("release already staged", "target_version", res.latestVersion, "digest", truncate(digest, 19))
	default:
		s.metrics.PrestageDownloads.Inc()
		s.logger.Info("pre-staged the release", "target_version", res.latestVersion, "digest", truncate(digest, 19), "took", time.Since(start).Round(time.Second))
	}
}
