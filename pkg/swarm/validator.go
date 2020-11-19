// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

type Validator interface {
	Validate(ch Chunk) (valid bool)
}

type ValidatorWithCallback interface {
	ValidWithCallback(ch Chunk) (valid bool, callback func())
	Validator
}

var _ Validator = (*validatorWithCallback)(nil)

type validatorWithCallback struct {
	v        Validator
	callback func(Chunk)
}

func (v *validatorWithCallback) Validate(ch Chunk) bool {
	valid := v.v.Validate(ch)
	if valid {
		go v.callback(ch)
	}
	return valid
}

var _ ValidatorWithCallback = (*multiValidator)(nil)

type multiValidator struct {
	validators []Validator
	callbacks  []func(Chunk)
}

func NewMultiValidator(validators []Validator, callbacks ...func(Chunk)) ValidatorWithCallback {
	return &multiValidator{validators, callbacks}
}

func (mv *multiValidator) Validate(ch Chunk) bool {
	for _, v := range mv.validators {
		if v.Validate(ch) {
			return true
		}
	}

	return false
}
func (mv *multiValidator) ValidWithCallback(ch Chunk) (bool, func()) {
	for i, v := range mv.validators {
		if v.Validate(ch) {
			if i < len(mv.callbacks) {
				return true, func() { mv.callbacks[i](ch) }
			}
			return true, nil
		}
	}

	return false, nil
}
