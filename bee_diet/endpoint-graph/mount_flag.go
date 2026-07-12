// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "strings"

// mountFlag is a repeatable -mount flag value (flag.Value).
type mountFlag []string

func (m *mountFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *mountFlag) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		*m = append(*m, part)
	}
	return nil
}
