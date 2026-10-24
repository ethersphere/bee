// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package updatecheck

import (
	"context"
	"net/http"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ErrNotOK               = errNotOK
	ErrResponseTooLarge    = errResponseTooLarge
	ErrCrossOriginRedirect = errCrossOriginRedirect
	ErrUnverified          = errUnverified
	ErrNoReleaseVersion    = errNoReleaseVersion
	ErrNoReleaseTags       = errNoReleaseTags
	ErrUnparsableVersion   = errUnparsableVersion
	ErrBadSignatures       = errBadSignatures
	ErrNotSigned           = errNotSigned
	ErrSchemaVersion       = errSchemaVersion
	ErrVersionAhead        = errVersionAhead

	MaxInfoSize       = maxInfoSize
	MaxDescriptorSize = maxDescriptorSize
	MaxSignatureSize  = maxSignatureSize
	MaxHeaderBytes    = maxHeaderBytes

	RestartSlot  = restartSlot
	LateJitter   = lateJitter
	RetryBackoff = retryBackoff
)

// NewUnstarted returns a service whose periodic check is not started, so tests
// call Check themselves.
func NewUnstarted(logger log.Logger, o Options) (*Service, error) {
	return newService(logger, o)
}

// Result is the outcome of a check.
type Result struct {
	Current, Latest string
	LatestVersion   uint64
	Available       bool
	OtherChannel    bool
	BelowNoRollback bool
}

func (s *Service) Check(ctx context.Context) (Result, error) {
	r, err := s.check(ctx)
	return Result{
		Current:         r.current,
		Latest:          r.latest,
		LatestVersion:   r.latestVersion,
		Available:       r.available,
		OtherChannel:    r.otherChannel,
		BelowNoRollback: r.belowNoRollback,
	}, err
}

func (s *Service) RestartActive() bool { return s.restart.active }

func (s *Service) Registry() string { return s.registry }

// Metrics holds the metrics state of the service.
type Metrics struct {
	Available         float64
	CheckErrors       float64
	LatestVersion     float64
	RestartScheduled  float64
	RestartSuppressed float64
	PrestageDownloads float64
	PrestageErrors    float64
}

func (s *Service) MetricValues() Metrics {
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(s.Metrics()...)
	mfs, err := reg.Gather()
	if err != nil {
		panic(err)
	}
	var m Metrics
	for _, mf := range mfs {
		for _, v := range mf.GetMetric() {
			switch mf.GetName() {
			case "bee_update_available":
				m.Available = v.GetGauge().GetValue()
			case "bee_update_check_errors_total":
				m.CheckErrors = v.GetCounter().GetValue()
			case "bee_update_latest_release_version":
				m.LatestVersion = v.GetGauge().GetValue()
			case "bee_update_restart_scheduled_timestamp_seconds":
				m.RestartScheduled = v.GetGauge().GetValue()
			case "bee_update_restart_suppressed":
				m.RestartSuppressed = v.GetGauge().GetValue()
			case "bee_update_prestage_downloads_total":
				m.PrestageDownloads = v.GetCounter().GetValue()
			case "bee_update_prestage_errors_total":
				m.PrestageErrors = v.GetCounter().GetValue()
			}
		}
	}
	return m
}

// DefaultDownloadHeaderTimeout is the response header timeout of the default
// download client.
func DefaultDownloadHeaderTimeout() time.Duration {
	return newDownloadClient(nil).Transport.(*http.Transport).ResponseHeaderTimeout
}

// DefaultGet fetches url with the default client. It then closes the client's
// idle connections.
func DefaultGet(ctx context.Context, url string, limit int) ([]byte, error) {
	g := newHTTPGetter(nil)
	defer g.client.CloseIdleConnections()
	return g.get(ctx, url, limit)
}

// Get fetches url with client. It reuses the getter g across calls.
type Getter struct{ g *httpGetter }

func NewGetter(client *http.Client) Getter { return Getter{newHTTPGetter(client)} }

func (g Getter) Get(ctx context.Context, url string, limit int) ([]byte, error) {
	return g.g.get(ctx, url, limit)
}

// CompareVersions parses a and b as running bee versions and compares them.
func CompareVersions(a, b string) (int, bool) {
	va, ok := parseCurrentVersion(a)
	if !ok {
		return 0, false
	}
	vb, ok := parseCurrentVersion(b)
	if !ok {
		return 0, false
	}
	return va.Compare(*vb), true
}
