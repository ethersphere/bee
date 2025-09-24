// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !nometrics
// +build !nometrics

package exporter

import (
	exporter "contrib.go.opencensus.io/exporter/prometheus"
	"github.com/prometheus/client_golang/prometheus"
)

func NewExporter(o ExporterOptions) error {
	r, _ := o.Registry.(*prometheus.Registry)
	opts := exporter.Options{
		Namespace: o.Namespace,
		Registry:  r,
	}
	_, err := exporter.NewExporter(opts)
	return err
}
