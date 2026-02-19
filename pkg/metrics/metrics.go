// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metrics

import (
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
)

// Namespace is prefixed before every metric. If it is changed, it must be done
// before any metrics collector is registered.
var Namespace = "bee"

type Collector interface {
	Metrics() []prometheus.Collector
}

func PrometheusCollectorsFromFields(i any) (cs []prometheus.Collector) {
	v := reflect.Indirect(reflect.ValueOf(i))
	for _, field := range v.Fields() {
		if !field.CanInterface() {
			continue
		}
		if u, ok := field.Interface().(prometheus.Collector); ok {
			cs = append(cs, u)
		}
	}
	return cs
}
