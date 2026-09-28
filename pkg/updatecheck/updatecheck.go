// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package updatecheck periodically asks a swarm-oci-serve registry which bee
// release it serves and reports, through metrics and a log line, whether it is
// newer than the running one. It never downloads or installs anything.
//
// When bee was started by bee-runner, which hands over the release it started
// and the release signing key it trusts, the check reads the signed release
// descriptor (/release.json and /release.sig) and verifies it against that
// key exactly as the runner does. A release is then newer when its descriptor
// version, the Unix time it was published, is higher than the running one and
// it is published on the runner's channel. This is also what the opt-in
// update restart acts on: see RestartOptions.
//
// Without a release key the check is report-only and reads the registry's
// unsigned /info summary, trusting the registry's own "verified" field.
// Outside bee-runner, where there is no descriptor version to compare with,
// the highest plain semver tag is compared with the running version.
package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coreos/go-semver/semver"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "updatecheck"

const (
	// DefaultInterval is the default time between two checks.
	DefaultInterval = 15 * time.Minute

	initialDelayBase   = 30 * time.Second
	initialDelayJitter = 30 * time.Second

	infoPath = "info"
	// maxInfoSize bounds the registry's /info response.
	maxInfoSize = 1 << 20 // 1 MiB

	// defaultChannel is the channel bee-runner follows when none is set.
	defaultChannel = "stable"

	// maxLoggedNotes, maxLoggedValue and maxLoggedChannels bound what a
	// release, which the registry controls, writes to a log line.
	maxLoggedNotes    = 1 << 10
	maxLoggedValue    = 64
	maxLoggedChannels = 8
)

var (
	errUnverified        = errors.New("updatecheck: registry release descriptor is not verified")
	errNoReleaseVersion  = errors.New("updatecheck: registry reports no release descriptor version")
	errNoReleaseTags     = errors.New("updatecheck: registry lists no release tags")
	errUnparsableVersion = errors.New("updatecheck: cannot parse own version")
)

// Runner is what bee-runner tells bee about how it was started, through the
// BEE_RUNNER* environment variables. The zero value means bee was not started
// by bee-runner.
type Runner struct {
	// Started is set when BEE_RUNNER is 1.
	Started bool
	// Registry is BEE_RUNNER_REGISTRY, the base URL of the registry the
	// runner took the release from.
	Registry string
	// Channel is BEE_RUNNER_CHANNEL, the release channel the runner follows.
	Channel string
	// Version is BEE_RUNNER_VERSION, the descriptor version of the release
	// the runner started.
	Version string
	// Pubkey is BEE_RUNNER_PUBKEY, the release signing key the runner
	// verified the release with, as 64 lowercase hex characters.
	Pubkey string
	// RolledBack is BEE_RUNNER_ROLLED_BACK, the descriptor version of a
	// release the runner rolled back from because it kept crashing.
	RolledBack string
}

// Options configure the update check service.
type Options struct {
	// URL is the base URL of the swarm-oci-serve registry. Empty disables
	// the check, unless the update restart is enabled: the registry of
	// bee-runner is then used.
	URL string
	// Interval is the time between checks. Zero means DefaultInterval.
	Interval time.Duration
	// CurrentVersion is the version of the running bee (bee.Version).
	CurrentVersion string
	// Runner is the hand-over from bee-runner.
	Runner Runner
	// Client is the HTTP client used for requests. Optional; by default a
	// client with a request timeout, a small response header limit and no
	// cross-origin redirects.
	Client *http.Client
	// Restart configures the opt-in restart when a newer release is offered.
	Restart RestartOptions
	// Overlay is the node's overlay address. It places the node's restart
	// deterministically within a release's rollout window.
	Overlay swarm.Address
}

// release is what a check learns about the release the registry offers,
// either from the signed descriptor or from the registry's /info summary.
type release struct {
	Verified bool `json:"verified"`
	// Version is the release descriptor version: the Unix time at which it
	// was published. bee-runner orders releases by it, never by tags.
	Version uint64 `json:"version"`
	// Channels the release is published on. Empty means every channel.
	Channels []string  `json:"channels"`
	Tags     []tagInfo `json:"tags"`
	// Notes is optional free text, such as an operator action needed
	// before upgrading.
	Notes string `json:"notes"`
	// CreatedAt is when the release was signed (RFC 3339). Update restarts
	// are spread over the rollout window starting at this time.
	CreatedAt string `json:"createdAt"`
	// RolloutWindowSeconds is the time over which the publisher wants the
	// fleet to restart for the release. Absent means DefaultRolloutWindow;
	// zero means as soon as safe.
	RolloutWindowSeconds *uint64 `json:"rolloutWindowSeconds"`
}

// rolloutWindow returns the release's rollout window and whether the release
// sets one. Absurdly large windows are bounded by maxRolloutWindow.
func (r *release) rolloutWindow() (time.Duration, bool) {
	if r.RolloutWindowSeconds == nil {
		return DefaultRolloutWindow, false
	}
	secs := *r.RolloutWindowSeconds
	if secs > uint64(maxRolloutWindow/time.Second) {
		return maxRolloutWindow, true
	}
	return time.Duration(secs) * time.Second, true
}

// onChannel reports whether the release is offered on channel.
func (r *release) onChannel(channel string) bool {
	return len(r.Channels) == 0 || slices.Contains(r.Channels, channel)
}

type tagInfo struct {
	Tag string `json:"tag"`
}

// Service periodically checks for a newer bee release.
type Service struct {
	logger log.Logger
	http   *httpGetter
	// registry is the registry URL with any userinfo redacted, for logs.
	registry      string
	infoURL       string
	descriptorURL string
	signatureURL  string
	// trust holds the release signing key from bee-runner. When set, the
	// signed descriptor is fetched instead of /info.
	trust    *releaseKey
	interval time.Duration
	current  string
	metrics  metrics

	// runner is set when bee was started by bee-runner with a parsable
	// BEE_RUNNER_VERSION. Updates are then decided by descriptor version
	// (runnerVersion) and channel instead of by semver tags.
	runner        bool
	runnerVersion uint64
	channel       string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu               sync.Mutex
	lastAnnounce     string // latest release already announced in the log
	lastOtherChannel uint64 // latest version on another channel already logged
	restart          restartState
}

// New validates the options and starts the periodic check. The caller must
// Close the returned service to stop it. When the check is disabled, New
// starts nothing and returns a nil service and a nil error.
func New(logger log.Logger, o Options) (*Service, error) {
	s, err := newService(logger, o)
	if s == nil || err != nil {
		return nil, err
	}

	s.wg.Add(1)
	go s.run(initialDelayBase + rand.N(initialDelayJitter))

	return s, nil
}

// newService returns the service without starting it, or a nil service and a
// nil error when the check is disabled.
func newService(logger log.Logger, o Options) (*Service, error) {
	logger = logger.WithName(loggerName).Register()

	if o.Interval < 0 {
		return nil, fmt.Errorf("updatecheck: invalid interval %s", o.Interval)
	}
	interval := o.Interval
	if interval == 0 {
		interval = DefaultInterval
	}

	runnerVersion, runner := parseRunnerVersion(o.Runner)
	channel := o.Runner.Channel
	if channel == "" {
		channel = defaultChannel
	}

	var trust *releaseKey
	if runner && o.Runner.Pubkey != "" {
		k, err := parseReleaseKey(o.Runner.Pubkey)
		if err != nil {
			logger.Warning("ignoring the release signing key from bee-runner; update availability is reported from the unsigned registry info only", "error", err)
		}
		trust = k
	}

	restartActive, err := resolveRestart(logger, &o, runner, trust != nil)
	if err != nil {
		return nil, err
	}
	if o.URL == "" {
		return nil, nil
	}

	base, err := parseBaseURL(o.URL)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		logger:        logger,
		http:          newHTTPGetter(o.Client),
		registry:      base.Redacted(),
		infoURL:       base.JoinPath(infoPath).String(),
		descriptorURL: base.JoinPath(descriptorPath).String(),
		signatureURL:  base.JoinPath(signaturePath).String(),
		trust:         trust,
		interval:      interval,
		current:       o.CurrentVersion,
		metrics:       newMetrics(),
		runner:        runner,
		runnerVersion: runnerVersion,
		channel:       channel,
		ctx:           ctx,
		cancel:        cancel,
	}
	if restartActive {
		s.initRestart(o)
	}
	s.metrics.RunningReleaseVersion.Set(float64(runnerVersion))

	logger.Info("checking for newer bee releases", "registry", s.registry, "interval", interval, "signed", trust != nil, "update_restart", restartActive)
	return s, nil
}

// parseRunnerVersion parses BEE_RUNNER_VERSION. ok is false when bee was not
// started by bee-runner or the version is absent or invalid; bee is then not
// treated as started by the runner.
func parseRunnerVersion(r Runner) (version uint64, ok bool) {
	if !r.Started || r.Version == "" {
		return 0, false
	}
	version, err := strconv.ParseUint(r.Version, 10, 64)
	if err != nil {
		return 0, false
	}
	return version, true
}

func parseBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		// url.Parse errors quote the input, which may hold credentials.
		return nil, errors.New("updatecheck: invalid registry url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("updatecheck: registry url scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("updatecheck: registry url has no host")
	}
	u.RawQuery, u.Fragment = "", ""
	return u, nil
}

func (s *Service) run(delay time.Duration) {
	defer s.wg.Done()

	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
		}

		s.checkOnce(s.ctx)
		timer.Reset(jitter(s.nextInterval()))
	}
}

// jitter returns d changed by a random amount of up to 10%, so that nodes
// started together do not poll the registry together.
func jitter(d time.Duration) time.Duration {
	return d - d/10 + rand.N(d/5+1)
}

// checkOnce runs a single check and records its outcome in metrics and logs.
func (s *Service) checkOnce(ctx context.Context) {
	res, err := s.check(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return // shutting down
		}
		// Counted in a metric; the registry being unreachable now and then
		// is not something an operator has to act on.
		s.metrics.CheckErrors.Inc()
		s.logger.Debug("update check failed", "registry", s.registry, "error", err)
		return
	}

	s.metrics.LastSuccess.SetToCurrentTime()
	s.metrics.LatestReleaseVersion.Set(float64(res.latestVersion))

	available := 0.0
	if res.available {
		available = 1
	}
	// Reset so that a previous {current,latest} pair does not linger once the
	// registry moves on.
	s.metrics.Available.Reset()
	s.metrics.Available.WithLabelValues(res.current, res.latest).Set(available)

	if !res.available {
		if res.otherChannel {
			s.announceOtherChannel(res)
		}
		s.logger.Debug("no newer bee release available", res.logValues()...)
		return
	}

	// Under the runner a republish of the same tag is a new release, so it
	// is announced again.
	key := res.latest
	if s.runner {
		key += "@" + strconv.FormatUint(res.latestVersion, 10)
	}
	s.mu.Lock()
	announce := s.lastAnnounce != key
	s.lastAnnounce = key
	s.mu.Unlock()

	if announce {
		s.logger.Info("a newer bee release is available", append(res.logValues(), "notes", truncate(res.notes, maxLoggedNotes))...)
	}

	s.maybeScheduleRestart(res)
}

// announceOtherChannel logs, once per descriptor version, a newer release that
// is not offered on the runner's channel and is therefore not taken.
func (s *Service) announceOtherChannel(res result) {
	s.mu.Lock()
	announce := s.lastOtherChannel != res.latestVersion
	s.lastOtherChannel = res.latestVersion
	s.mu.Unlock()

	if announce {
		s.logger.Info("a newer bee release exists on another channel",
			"channel", truncate(s.channel, maxLoggedValue),
			"latest", res.latest,
			"latest_version", res.latestVersion,
			"channels", truncateList(res.channels),
			"notes", truncate(res.notes, maxLoggedNotes),
		)
	}
}

// truncate shortens s to at most n bytes, without splitting a UTF-8
// sequence.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	const suffix = "..."
	cut := n - len(suffix)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + suffix
}

// truncateList bounds a list of registry-provided values for a log line.
func truncateList(l []string) []string {
	out := make([]string, 0, min(len(l), maxLoggedChannels+1))
	for i, v := range l {
		if i == maxLoggedChannels {
			out = append(out, fmt.Sprintf("(%d more)", len(l)-i))
			break
		}
		out = append(out, truncate(v, maxLoggedValue))
	}
	return out
}

type result struct {
	// current and latest are the semver of the running bee and the highest
	// release tag offered, for humans. Under the runner they may be the raw
	// own version and empty, as they do not decide anything there.
	current string
	latest  string
	// runningVersion and latestVersion are the descriptor versions of the
	// release the runner started (0 outside the runner) and of the release
	// the registry offers (0 if it reports none).
	runningVersion uint64
	latestVersion  uint64
	available      bool
	// otherChannel is set under the runner when the offered release has a
	// higher descriptor version but is not published on the runner's
	// channel.
	otherChannel bool
	channels     []string
	notes        string
	// createdAt is the release's raw createdAt; window is its rollout window
	// and windowSet whether the release sets it explicitly.
	createdAt string
	window    time.Duration
	windowSet bool
}

func (r result) logValues() []any {
	return []any{
		"current", truncate(r.current, maxLoggedValue),
		"latest", r.latest,
		"running_version", r.runningVersion,
		"latest_version", r.latestVersion,
	}
}

// check fetches the offered release and decides whether it is newer than the
// running one: by descriptor version and channel under bee-runner, by semver
// tag otherwise.
func (s *Service) check(ctx context.Context) (result, error) {
	current, currentOK := parseCurrentVersion(s.current)
	if !currentOK && !s.runner {
		return result{}, fmt.Errorf("%w %q", errUnparsableVersion, truncate(s.current, maxLoggedValue))
	}

	var (
		r   *release
		err error
	)
	if s.trust != nil {
		r, err = s.fetchSigned(ctx)
	} else {
		r, err = s.fetchInfo(ctx)
	}
	if err != nil {
		return result{}, err
	}

	res := result{
		current:        s.current,
		runningVersion: s.runnerVersion,
		latestVersion:  r.Version,
		channels:       r.Channels,
		notes:          r.Notes,
		createdAt:      r.CreatedAt,
	}
	res.window, res.windowSet = r.rolloutWindow()
	if currentOK {
		res.current = current.String()
	}
	latest := latestRelease(r)
	if latest != nil {
		res.latest = latest.String()
	}

	if s.runner {
		if r.Version == 0 {
			return result{}, errNoReleaseVersion
		}
		newer, onChannel := r.Version > s.runnerVersion, r.onChannel(s.channel)
		res.available = newer && onChannel
		res.otherChannel = newer && !onChannel
		return res, nil
	}

	if latest == nil {
		return result{}, errNoReleaseTags
	}
	res.available = latest.Compare(*current) > 0
	return res, nil
}

// fetchInfo fetches the registry's unsigned /info summary and accepts it only
// when the registry reports that it verified the release descriptor.
func (s *Service) fetchInfo(ctx context.Context) (*release, error) {
	body, err := s.http.get(ctx, s.infoURL, maxInfoSize)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", infoPath, err)
	}
	var r release
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("decode %s: %w", infoPath, err)
	}
	if !r.Verified {
		return nil, errUnverified
	}
	return &r, nil
}

// latestRelease returns the highest plain release version among the tags, or
// nil if there is none. Other tags, pre-releases such as 2.9.0-rc1 included,
// are skipped, so a node is never told to "upgrade" to a release candidate.
func latestRelease(r *release) *semver.Version {
	var latest *semver.Version
	for _, t := range r.Tags {
		v, ok := parseReleaseTag(t.Tag)
		if ok && (latest == nil || v.Compare(*latest) > 0) {
			latest = v
		}
	}
	return latest
}

// Close stops the periodic check and waits for it to finish.
func (s *Service) Close() error {
	s.cancel()
	s.wg.Wait()
	return nil
}
