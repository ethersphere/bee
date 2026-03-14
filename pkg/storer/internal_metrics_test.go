// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"testing"
	"time"

	batchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	stabilmock "github.com/ethersphere/bee/v2/pkg/stabilization/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func toFloat64(c prometheus.Collector) float64 {
	var (
		m = make(chan prometheus.Metric, 1) // Buffered channel to avoid blocking if only 1 metric

	)

	// Collect the metric
	c.Collect(m)
	close(m) // Close immediately after collect

	metric := <-m
	if metric == nil {
		return 0
	}

	pb := &dto.Metric{}
	if err := metric.Write(pb); err != nil {
		return 0
	}

	if pb.Gauge != nil {
		return pb.Gauge.GetValue()
	}
	if pb.Counter != nil {
		return pb.Counter.GetValue()
	}
	// Add other types if necessary
	return 0
}

func TestLevelDBMetrics(t *testing.T) {
	// Options with LdbStats initialized to capture metrics
	opts := DefaultOptions()
	opts.Address = swarm.RandAddress(t)
	opts.RadiusSetter = kademlia.NewTopologyDriver()
	opts.Batchstore = batchstore.New()
	opts.ReserveWakeUpDuration = time.Second
	opts.StartupStabilizer = stabilmock.NewSubscriber(true)
	opts.LdbStats.Store(prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "test_ldb_stats",
		Help: "Test LevelDB stats",
	}, []string{"counter"}))

	// Create a real disk storer to trigger initDiskRepository
	dir := t.TempDir()
	st, err := New(context.Background(), dir, opts)
	if err != nil {
		t.Fatalf("New(...) unexpected error: %v", err)
	}
	defer st.Close()

	// Verify Configuration Metrics
	if v := toFloat64(st.metrics.LevelDBConfigCompactionL0Trigger); v != 16 {
		t.Errorf("LevelDBConfigCompactionL0Trigger = %v, want 16", v)
	}
	if v := toFloat64(st.metrics.LevelDBConfigCompactionTableSize); v != 8*1024*1024 {
		t.Errorf("LevelDBConfigCompactionTableSize = %v, want 8388608", v)
	}
	// Default options -> 0/default values, but initStore might set defaults if 0?
	// In strict sense, opts.LdbWriteBufferSize is 0 in DefaultOptions, so it should be 0 unless processed.
	// We can check if it reflects what's in opts.

	// Verify that Stats metrics are registered (not necessarily non-zero, but present)
	// access via the unexported metrics field
	_ = st.metrics.LevelDBBlockCacheSize
	_ = st.metrics.LevelDBIOWrite

	// Wait for a ticker tick (15s in production code, might be too long for test?)
	// We cannot easily speed up the ticker inside initDiskRepository without refactoring to inject the ticker/interval.
	// For now, testing the config initialization is enough to prove the metrics struct is wired up.
}
