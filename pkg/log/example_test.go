// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log_test

import (
	"errors"
	"fmt"
	"os"

	"github.com/ethersphere/bee/v2/pkg/log"
)

func Example() {
	log.ModifyDefaults(
		log.WithSink(os.Stdout),
		log.WithVerbosity(log.VerbosityAll),
	)

	logger := log.NewLogger("example").Build()
	logger.Error(nil, "error should work", "k", 1, "k", 2)
	logger.Warning("warning should work", "address", 12345)
	logger.Info("info should work", "k", 3, "k", 4)
	logger.Debug("debug should work", "error", errors.New("failed"))
	fmt.Println()

	loggerV1 := logger.V(1).Build()
	loggerV1.Error(nil, "v1 error should work")
	loggerV1.Warning("v1 warning should work")
	loggerV1.Info("v1 info should work")
	loggerV1.Debug("v1 debug should work")
	fmt.Println()

	_ = log.SetVerbosity(loggerV1, log.VerbosityWarning)
	loggerV1.Error(nil, "v1 error should work")
	loggerV1.Warning("v1 warning should work")
	loggerV1.Info("v1 info shouldn't work")
	loggerV1.Debug("v1 debug shouldn't work")
	fmt.Println()

	loggerV1WithName := loggerV1.WithName("example_name").Build()
	// The verbosity was inherited from loggerV1 so we have to change it.
	_ = log.SetVerbosity(loggerV1WithName, log.VerbosityAll)
	loggerV1WithName.Error(nil, "v1 with name error should work")
	loggerV1WithName.Warning("v1 with name warning should work")
	loggerV1WithName.Info("v1 with name info should work")
	loggerV1WithName.Debug("v1 with name debug should work")

	// Output:
	// "level"="error" "logger"="example" "msg"="error should work" "k"=1 "k"=2
	// "level"="warning" "logger"="example" "msg"="warning should work" "address"=12345
	// "level"="info" "logger"="example" "msg"="info should work" "k"=3 "k"=4
	// "level"="debug" "logger"="example" "msg"="debug should work" "error"="failed"
	//
	// "level"="error" "logger"="example" "msg"="v1 error should work"
	// "level"="warning" "logger"="example" "msg"="v1 warning should work"
	// "level"="info" "logger"="example" "msg"="v1 info should work"
	// "level"="debug" "logger"="example" "v"=1 "msg"="v1 debug should work"
	//
	// "level"="error" "logger"="example" "msg"="v1 error should work"
	// "level"="warning" "logger"="example" "msg"="v1 warning should work"
	//
	// "level"="error" "logger"="example/example_name" "msg"="v1 with name error should work"
	// "level"="warning" "logger"="example/example_name" "msg"="v1 with name warning should work"
	// "level"="info" "logger"="example/example_name" "msg"="v1 with name info should work"
	// "level"="debug" "logger"="example/example_name" "v"=1 "msg"="v1 with name debug should work"
}
