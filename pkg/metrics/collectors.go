// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metrics

import (
	"reflect"
)

func PrometheusCollectorsFromFields(i any) (cs []Collector) {
	v := reflect.Indirect(reflect.ValueOf(i))
	for i := 0; i < v.NumField(); i++ {
		if !v.Field(i).CanInterface() {
			continue
		}

		u, ok := v.Field(i).Interface().(Collector)
		if !ok {
			continue
		}
		cs = append(cs, u)

	}
	return cs
}
