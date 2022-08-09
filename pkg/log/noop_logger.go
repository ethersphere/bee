// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

// Noop is an implementation of a logger that does not log.
var Noop Logger = new(noopLogger)

type noopLogger struct{}

func (nl *noopLogger) V(_ uint) Builder                    { return nl }
func (nl *noopLogger) WithName(_ string) Builder           { return nl }
func (nl *noopLogger) WithValues(_ ...interface{}) Builder { return nl }
func (nl *noopLogger) Build() Logger                       { return nl }
func (nl *noopLogger) Register() Logger                    { return nl }

func (nl *noopLogger) Verbosity() Level                          { return VerbosityNone }
func (nl *noopLogger) Debug(_ string, _ ...interface{})          {}
func (nl *noopLogger) Info(_ string, _ ...interface{})           {}
func (nl *noopLogger) Warning(_ string, _ ...interface{})        {}
func (nl *noopLogger) Error(_ error, _ string, _ ...interface{}) {}
