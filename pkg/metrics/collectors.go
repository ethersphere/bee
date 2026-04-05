// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metrics

import (
	"reflect"
)

func PrometheusCollectorsFromFields(i any) (cs []Collector) {
	v := reflect.Indirect(reflect.ValueOf(i))
	for _, field := range v.Fields() {
		if !field.CanInterface() {
			continue
		}
		if u, ok := field.Interface().(Collector); ok {
			cs = append(cs, u)
		}
	}
	return cs
}
