// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package updatecheck periodically asks a swarm-oci-serve registry which bee
// release it serves. Through metrics and a log line, it reports whether that
// release is newer than the running one. It never installs anything. Only
// with the opt-in update restart under bee-runner does it download the new
// binary, into the runner's cache, where the runner verifies it again.
//
// When bee was started by bee-runner, the runner hands over the release it
// started and the release signing key it trusts. The check then reads the
// signed release descriptor (/release.json and /release.sig) and verifies it
// against that key, exactly as the runner does. A release is newer when its
// descriptor version (the Unix time it was published) is higher than the
// running one and it is published on the runner's channel. The opt-in update
// restart also acts on this: see RestartOptions.
//
// Without a release key, the check is report-only. It reads the registry's
// unsigned /info summary and trusts the registry's own "verified" field.
// Outside bee-runner there is no descriptor version to compare with, so the
// highest plain semver tag is compared with the running version.
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
	// maxInfoSize bounds the size of the registry's /info response.
	maxInfoSize = 1 << 20 // 1 MiB

	// defaultChannel is the channel bee-runner follows when none is set.
	defaultChannel = "stable"

	// maxLoggedNotes, maxLoggedValue and maxLoggedChannels bound how much of a
	// release a log line may contain. The registry controls the release.
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
	// Version is BEE_RUNNER_VERSION: the descriptor version of the release the
	// runner started.
	Version string
	// Pubkey is BEE_RUNNER_PUBKEY: the release signing key the runner used to
	// verify the release, as 64 lowercase hex characters.
	Pubkey string
	// RolledBack is BEE_RUNNER_ROLLED_BACK: the descriptor version of a release
	// the runner rolled back from because it kept crashing.
	RolledBack string
	// Cache is BEE_RUNNER_CACHE, the runner's binary cache (an absolute path).
	// Binary is BEE_RUNNER_BINARY, the path of this platform's binary inside a
	// release. When both are set, the update restart first stages the new
	// binary in the cache, then exits.
	Cache  string
	Binary string
	// NoRollback is BEE_RUNNER_NO_ROLLBACK: the bee version of a no-rollback
	// release this node ran. bee-runner refuses every release that carries an
	// older bee, so bee must not restart for one.
	NoRollback string
}

// Options configure the update check service.
type Options struct {
	// URL is the base URL of the swarm-oci-serve registry. If empty, the check
	// is disabled, unless the update restart is enabled. In that case the
	// bee-runner registry is used.
	URL string
	// Interval is the time between checks. Zero means DefaultInterval.
	Interval time.Duration
	// CurrentVersion is the version of the running bee (bee.Version).
	CurrentVersion string
	// Runner holds the values handed over from bee-runner.
	Runner Runner
	// Client is the HTTP client used for requests. It is optional. The default
	// client has a request timeout, a small response header limit and follows
	// no cross-origin redirects.
	Client *http.Client
	// Restart configures the opt-in restart when a newer release is offered.
	Restart RestartOptions
	// Overlay is the node's overlay address. It gives the node a fixed,
	// repeatable place for its restart inside a release's rollout window.
	Overlay swarm.Address
}

// release is what a check learns about the release the registry offers. The
// data comes from the signed descriptor or from the registry's /info summary.
type release struct {
	Verified bool `json:"verified"`
	// Version is the release descriptor version: the Unix time at which the
	// release was published. bee-runner orders releases by this value, never by
	// tags.
	Version uint64 `json:"version"`
	// Channels lists the channels the release is published on. Empty means
	// every channel.
	Channels []string  `json:"channels"`
	Tags     []tagInfo `json:"tags"`
	// Notes is optional free text, for example an action the operator must take
	// before upgrading.
	Notes string `json:"notes"`
	// CreatedAt is when the release was signed (RFC 3339). The rollout window
	// starts at this time, and update restarts are spread across it.
	CreatedAt string `json:"createdAt"`
	// RolloutWindowSeconds is how long the publisher wants the fleet to take to
	// restart for this release. If absent, DefaultRolloutWindow applies. Zero
	// means restart as soon as it is safe.
	RolloutWindowSeconds *uint64 `json:"rolloutWindowSeconds"`
	// Files maps a path in the release to its sha256 digest. Only the
	// signed descriptor has it.
	Files map[string]string `json:"-"`
}

// rolloutWindow returns the release's rollout window and whether the release
// sets one. Very large windows are capped at maxRolloutWindow.
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

// Service checks for a newer bee release at regular intervals.
type Service struct {
	logger log.Logger
	http   *httpGetter
	// registry is the registry URL with any userinfo removed, for logs.
	registry      string
	infoURL       string
	descriptorURL string
	signatureURL  string
	// trust holds the release signing key from bee-runner. When it is set, the
	// signed descriptor is fetched instead of /info.
	trust    *releaseKey
	interval time.Duration
	current  string
	metrics  metrics

	// runner is set when bee was started by bee-runner with a parsable
	// BEE_RUNNER_VERSION. Updates are then decided by descriptor version
	// (runnerVersion) and channel, not by semver tags.
	runner        bool
	runnerVersion uint64
	channel       string
	// noRollback is Runner.NoRollback parsed, or nil.
	noRollback *semver.Version

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu               sync.Mutex
	lastAnnounce     string // latest release already announced in the log
	lastOtherChannel uint64 // latest version on another channel already logged
	lastBelowBarrier uint64 // latest version below the no-rollback barrier already logged
	restart          restartState
}

// New validates the options and starts the periodic check. The caller must
// Close the returned service to stop it. If the check is disabled, New starts
// nothing and returns a nil service and a nil error.
func New(logger log.Logger, o Options) (*Service, error) {
	s, err := newService(logger, o)
	if s == nil || err != nil {
		return nil, err
	}

	s.wg.Add(1)
	go s.run(initialDelayBase + rand.N(initialDelayJitter))

	return s, nil
}

// newService returns the service without starting it. If the check is disabled,
// it returns a nil service and a nil error.
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
		noRollback:    parseNoRollback(logger, o.Runner),
		ctx:           ctx,
		cancel:        cancel,
	}
	if restartActive {
		s.initRestart(o)
		st, err := newStager(o.Runner, base, newDownloadClient(o.Client))
		if err != nil {
			logger.Warning("not pre-staging releases", "error", err)
		}
		s.restart.stager = st
	}
	s.metrics.RunningReleaseVersion.Set(float64(runnerVersion))

	logger.Info("checking for newer bee releases", "registry", s.registry, "interval", interval, "signed", trust != nil, "update_restart", restartActive)
	return s, nil
}

// parseNoRollback parses BEE_RUNNER_NO_ROLLBACK. A value that cannot be parsed
// is ignored with a warning. bee-runner enforces the barrier either way, so the
// worst outcome is that bee restarts for a release the runner then refuses.
func parseNoRollback(logger log.Logger, r Runner) *semver.Version {
	if r.NoRollback == "" {
		return nil
	}
	v, ok := parseReleaseOrRC(r.NoRollback)
	if !ok {
		logger.Warning("ignoring unparsable no-rollback version from bee-runner", "no_rollback", truncate(r.NoRollback, maxLoggedValue))
		return nil
	}
	return v
}

// parseRunnerVersion parses BEE_RUNNER_VERSION. ok is false if bee was not
// started by bee-runner, or if the version is missing or invalid. In that case
// bee does not treat itself as started by the runner.
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
		// url.Parse errors quote the input, which may contain credentials.
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

// jitter returns d changed by a random amount of up to 10%. This keeps nodes
// that started together from polling the registry at the same time.
func jitter(d time.Duration) time.Duration {
	return d - d/10 + rand.N(d/5+1)
}

// checkOnce runs one check and records the result in metrics and logs.
func (s *Service) checkOnce(ctx context.Context) {
	res, err := s.check(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return // shutting down
		}
		// Counted in a metric. An operator does not need to act when the
		// registry is unreachable now and then.
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
	// Reset, so an old {current,latest} pair does not stay once the registry
	// moves on.
	s.metrics.Available.Reset()
	s.metrics.Available.WithLabelValues(res.current, res.latest).Set(available)

	if !res.available {
		if res.otherChannel {
			s.announceOtherChannel(res)
		}
		if res.belowNoRollback {
			s.announceBelowNoRollback(res)
		}
		s.logger.Debug("no newer bee release available", res.logValues()...)
		return
	}

	// Under the runner, republishing the same tag is a new release, so it is
	// announced again.
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

// announceOtherChannel logs a newer release once per descriptor version. The
// release is not offered on the runner's channel, so it is not taken.
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

// announceBelowNoRollback logs a newer release once per descriptor version.
// bee-runner will refuse it because its bee is older than the no-rollback
// release this node ran.
func (s *Service) announceBelowNoRollback(res result) {
	s.mu.Lock()
	announce := s.lastBelowBarrier != res.latestVersion
	s.lastBelowBarrier = res.latestVersion
	s.mu.Unlock()

	if announce {
		s.logger.Warning("not restarting for a newer release: it carries an older bee than the no-rollback release this node ran, which bee-runner refuses",
			"no_rollback", s.noRollback.String(),
			"latest", res.latest,
			"latest_version", res.latestVersion,
			"notes", truncate(res.notes, maxLoggedNotes),
		)
	}
}

// truncate shortens s to at most n bytes without splitting a UTF-8 sequence.
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

// truncateList limits a list of registry-provided values for a log line.
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
	// offered release tag, for humans to read. Under the runner they may be the
	// raw own version and empty, because they decide nothing there.
	current string
	latest  string
	// runningVersion and latestVersion are descriptor versions. runningVersion
	// is that of the release the runner started (0 outside the runner).
	// latestVersion is that of the release the registry offers (0 if it reports
	// none).
	runningVersion uint64
	latestVersion  uint64
	available      bool
	// otherChannel is set under the runner when the offered release has a
	// higher descriptor version but is not published on the runner's
	// channel.
	otherChannel bool
	// belowNoRollback is set under the runner when the offered release is
	// newer but carries a bee older than the no-rollback barrier, or no bee
	// version at all.
	belowNoRollback bool
	channels        []string
	notes           string
	// createdAt is the release's raw createdAt. window is its rollout window,
	// and windowSet says whether the release sets it explicitly.
	createdAt string
	window    time.Duration
	windowSet bool
	// files are the release's file digests. They come only from the signed
	// descriptor.
	files map[string]string
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
// running one. Under bee-runner it compares descriptor version and channel.
// Otherwise it compares the semver tag.
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
		files:          r.Files,
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
		// A newer release version can still carry an older bee (a revert).
		// bee-runner refuses such a release past a no-rollback release.
		// Restarting for it would only bring back this same binary.
		if newer && onChannel && s.noRollback != nil {
			if v := highestReleaseOrRC(r); v == nil || v.LessThan(*s.noRollback) {
				res.belowNoRollback = true
			}
		}
		res.available = newer && onChannel && !res.belowNoRollback
		res.otherChannel = newer && !onChannel
		return res, nil
	}

	if latest == nil {
		return result{}, errNoReleaseTags
	}
	res.available = latest.Compare(*current) > 0
	return res, nil
}

// fetchInfo fetches the registry's unsigned /info summary. It accepts the
// summary only if the registry reports that it verified the release descriptor.
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
// nil if there is none. It skips other tags, including pre-releases such as
// 2.9.0-rc1, so a node is never told to "upgrade" to a release candidate.
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

// highestReleaseOrRC returns the highest release or release candidate among the
// tags, or nil if there is none. bee-runner uses this to read a release's bee
// version when comparing it with a no-rollback barrier.
func highestReleaseOrRC(r *release) *semver.Version {
	var best *semver.Version
	for _, t := range r.Tags {
		v, ok := parseReleaseOrRC(t.Tag)
		if ok && (best == nil || v.Compare(*best) > 0) {
			best = v
		}
	}
	return best
}

// Close stops the periodic check and waits for it to finish.
func (s *Service) Close() error {
	s.cancel()
	s.wg.Wait()
	return nil
}
