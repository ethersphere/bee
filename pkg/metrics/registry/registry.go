// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"time"

	"github.com/ethersphere/bee/v2"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

type Registry struct {
	register     *prometheus.Registry
	pushRegister *prometheus.Registry
	cpuGauge     prometheus.Gauge
	memGauge     prometheus.Gauge
}

func NewRegistry(push bool, mode string) *Registry {
	r := &Registry{
		register:     prometheus.NewRegistry(),
		pushRegister: prometheus.NewRegistry(),
	}

	c := collectors.NewProcessCollector(collectors.ProcessCollectorOpts{
		Namespace: metrics.Namespace,
	})

	g := collectors.NewGoCollector()

	// Create CPU and memory gauge metrics
	cpuGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metrics.Namespace,
		Name:      "system_cpu_usage_percent",
		Help:      "System CPU usage percentage",
	})

	memGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metrics.Namespace,
		Name:      "system_memory_usage_percent",
		Help:      "System memory usage percentage",
	})

	v := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metrics.Namespace,
		Name:      "info",
		Help:      "Bee information.",
		ConstLabels: prometheus.Labels{
			"version": bee.Version,
			"mode":    mode,
		},
	})

	r.MustRegister(c, g, v, cpuGauge, memGauge)

	if push {
		r.MustPushRegister(c, g, v, cpuGauge, memGauge)
	}

	// Store references to gauges for updating in PushWorker
	r.cpuGauge = cpuGauge
	r.memGauge = memGauge

	return r
}

func (r *Registry) MetricsRegistry() *prometheus.Registry {
	return r.register
}

func (r *Registry) PushWorker(ctx context.Context, path string, job string, logger log.Logger) func() error {
	metricsPusher := push.New(path, job).Collector(r.pushRegister)

	ctx, cancel := context.WithCancel(ctx)

	go func() {
        ticker := time.NewTicker(time.Minute)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                // Collect CPU metrics
                percentages, err := cpu.Percent(0, false)
                if err == nil && len(percentages) > 0 {
                    r.cpuGauge.Set(percentages[0])
                } else if err != nil {
                    logger.Debug("failed to collect CPU metrics", "error", err)
                }

                // Collect memory metrics
                vm, err := mem.VirtualMemory()
                if err == nil {
                    r.memGauge.Set(vm.UsedPercent)
                } else {
                    logger.Debug("failed to collect memory metrics", "error", err)
                }

                // Push metrics
                if err := metricsPusher.Push(); err != nil {
                    logger.Debug("metrics push failed", "error", err)
                }
            }
        }
    }()

	return func() error {
		cancel()
		return metricsPusher.Push()
	}
}

func (r *Registry) MustRegister(cs ...prometheus.Collector) {
	r.register.MustRegister(cs...)
}

func (r *Registry) MustPushRegister(cs ...prometheus.Collector) {
	r.pushRegister.MustRegister(cs...)
}
