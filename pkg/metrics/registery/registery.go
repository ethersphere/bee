package registery

import (
	"context"
	"time"

	"github.com/ethersphere/bee/v2"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/push"
)

type Registry struct {
	register     *prometheus.Registry
	pushRegister *prometheus.Registry
}

func NewRegistry(push bool) *Registry {
	r := &Registry{
		register:     prometheus.NewRegistry(),
		pushRegister: prometheus.NewRegistry(),
	}

	c := collectors.NewProcessCollector(collectors.ProcessCollectorOpts{
		Namespace: metrics.Namespace,
	})

	g := collectors.NewGoCollector()

	v := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metrics.Namespace,
		Name:      "info",
		Help:      "Bee information.",
		ConstLabels: prometheus.Labels{
			"version": bee.Version,
		},
	})

	r.MustRegister(c, g, v)

	if push {
		r.MustPushRegister(c, g, v)
	}

	return r
}

func (r *Registry) MetricsRegistry() *prometheus.Registry {
	return r.register
}

func (r *Registry) PushWorker(ctx context.Context, path string, job string, logger log.Logger) func() error {
	metricsPusher := push.New(path, job).Collector(r.pushRegister)

	ctx, cancel := context.WithCancel(ctx)

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
			if err := metricsPusher.Push(); err != nil {
				logger.Debug("metrics push failed", "error", err)
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
