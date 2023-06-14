// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageutil

import (
	"strings"
)

// JoinFields joins the given fields with a slash.
func JoinFields(fields ...string) string {
	return strings.Join(fields, "/")
}
