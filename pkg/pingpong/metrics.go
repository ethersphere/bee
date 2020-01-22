package pingpong

import (
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	PingSentCount     prometheus.Counter
	PongSentCount     prometheus.Counter
	PingReceivedCount prometheus.Counter
	PongReceivedCount prometheus.Counter
}

func newMetrics() (m metrics) {
	return metrics{
		PingSentCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pingpong_ping_sent_count",
			Help: "Number ping requests sent.",
		}),
		PongSentCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pingpong_pong_sent_count",
			Help: "Number of pong responses sent.",
		}),
		PingReceivedCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pingpong_ping_received_count",
			Help: "Number ping requests received.",
		}),
		PongReceivedCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pingpong_pong_received_count",
			Help: "Number of pong responses received.",
		}),
	}
}

func (s *Service) Metrics() (cs []prometheus.Collector) {
	v := reflect.Indirect(reflect.ValueOf(s.metrics))
	for i := 0; i < v.NumField(); i++ {
		if !v.Field(i).CanInterface() {
			continue
		}
		if u, ok := v.Field(i).Interface().(prometheus.Collector); ok {
			cs = append(cs, u)
		}
	}
	return cs
}
