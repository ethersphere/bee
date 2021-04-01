// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"errors"
)

var ErrOverflow = errors.New("BMT hash capacity exceeded")
