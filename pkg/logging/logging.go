// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package logging

import (
	"io"

	"github.com/sirupsen/logrus"
)

func New(w io.Writer) *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(w)
	logger.Formatter = &logrus.TextFormatter{
		FullTimestamp: true,
	}
	return logger
}
