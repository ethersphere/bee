// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package swarm

import "context"

type ValidatorFunc func(context.Context, Chunk) bool

type Validator struct {
	Valid    func(ch Chunk) (valid bool)
	Callback func(context.Context, Chunk)
}

func NewValidator(vs ...Validator) ValidatorFunc {
	return func(ctx context.Context, ch Chunk) bool {
		for _, v := range vs {
			if v.Valid(ch) {
				if v.Callback != nil {
					go v.Callback(ctx, ch)
				}
				return true
			}
		}
		return false
	}
}
