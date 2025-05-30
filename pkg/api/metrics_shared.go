package api

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	RequestCount       prometheus.Counter
	ResponseDuration   prometheus.Histogram
	PingRequestCount   prometheus.Counter
	ResponseCodeCounts *prometheus.CounterVec

	ContentApiDuration *prometheus.HistogramVec
	UploadSpeed        *prometheus.HistogramVec
	DownloadSpeed      *prometheus.HistogramVec
}
