// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package updatecheck

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	Available   *prometheus.GaugeVec
	CheckErrors prometheus.Counter
	LastSuccess prometheus.Gauge

	RunningReleaseVersion prometheus.Gauge
	LatestReleaseVersion  prometheus.Gauge

	RestartScheduled  prometheus.Gauge
	RestartSuppressed prometheus.Gauge

	PrestageDownloads prometheus.Counter
	PrestageErrors    prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "update"

	return metrics{
		Available: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "available",
			Help:      "1 if the registry offers a bee release newer than the running one, 0 otherwise. Under bee-runner newer means a higher release descriptor version on the runner's channel; otherwise a higher semver tag. The labels are the running semver and the highest release tag offered.",
		}, []string{"current", "latest"}),
		CheckErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "check_errors_total",
			Help:      "Number of failed or ignored update checks.",
		}),
		LastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "last_success_timestamp_seconds",
			Help:      "Unix time of the last successful update check.",
		}),
		RunningReleaseVersion: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "running_release_version",
			Help:      "Release descriptor version bee-runner started (BEE_RUNNER_VERSION), 0 if unknown.",
		}),
		LatestReleaseVersion: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "latest_release_version",
			Help:      "Release descriptor version the registry offered at the last successful check, 0 if none.",
		}),
		RestartScheduled: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "restart_scheduled_timestamp_seconds",
			Help:      "Unix time at which a scheduled restart to update bee fires, 0 if none is scheduled.",
		}),
		RestartSuppressed: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "restart_suppressed",
			Help:      "1 if restarting to update is suppressed because previous restarts did not deliver the release or bee-runner rolled back from it, 0 otherwise.",
		}),
		PrestageDownloads: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "prestage_downloads_total",
			Help:      "Number of release binaries downloaded into the bee-runner cache before an update restart.",
		}),
		PrestageErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "prestage_errors_total",
			Help:      "Number of failed attempts to pre-stage a release binary before an update restart.",
		}),
	}
}

// Metrics returns the collectors of the service.
func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
