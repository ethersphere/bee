// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/go-playground/validator/v10"
)

const (
	RedundancyLevelTag = "rLevel"
)

// setupValidation configures custom validation rules and their custom error messages.
func (s *Service) setupValidation() {
	s.validate.RegisterValidation(RedundancyLevelTag, func(fl validator.FieldLevel) bool {
		level := redundancy.Level(fl.Field().Uint())
		return level.Validate()
	})

	s.validationCustomErrorMessages = map[string]func(err validator.FieldError) error{
		RedundancyLevelTag: func(err validator.FieldError) error {
			return fmt.Errorf("want redundancy level to be in the range of %d and %d", int(redundancy.NONE), int(redundancy.PARANOID))
		},
	}
}
