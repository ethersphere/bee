//go:build !js
// +build !js

package cmd

import (
	"fmt"
	"io"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/spf13/cobra"
)

func newCommand(opts ...option) (c *command, err error) {
	c = &command{
		root: &cobra.Command{
			Use:           "bee",
			Short:         "Ethereum Swarm Bee",
			SilenceErrors: true,
			SilenceUsage:  true,
			PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
				return c.initConfig()
			},
		},
	}

	for _, o := range opts {
		o(c)
	}
	if c.passwordReader == nil {
		c.passwordReader = new(stdInPasswordReader)
	}

	// Find home directory.
	if err := c.setHomeDir(); err != nil {
		return nil, err
	}

	c.initGlobalFlags()

	if err := c.initStartCmd(); err != nil {
		return nil, err
	}

	if err := c.initStartDevCmd(); err != nil {
		return nil, err
	}

	if err := c.initInitCmd(); err != nil {
		return nil, err
	}

	if err := c.initDeployCmd(); err != nil {
		return nil, err
	}

	c.initVersionCmd()
	c.initDBCmd()
	if err := c.initSplitCmd(); err != nil {
		return nil, err
	}

	if err := c.initConfigurateOptionsCmd(); err != nil {
		return nil, err
	}

	return c, nil
}

func newLogger(cmd *cobra.Command, verbosity string) (log.Logger, error) {
	var (
		sink   = cmd.OutOrStdout()
		vLevel = log.VerbosityNone
	)

	switch verbosity {
	case "0", "silent":
		sink = io.Discard
	case "1", "error":
		vLevel = log.VerbosityError
	case "2", "warn":
		vLevel = log.VerbosityWarning
	case "3", "info":
		vLevel = log.VerbosityInfo
	case "4", "debug":
		vLevel = log.VerbosityDebug
	case "5", "trace":
		vLevel = log.VerbosityDebug + 1 // For backwards compatibility, just enable v1 debugging as trace.
	default:
		return nil, fmt.Errorf("unknown verbosity level %q", verbosity)
	}

	log.ModifyDefaults(
		log.WithTimestamp(),
		log.WithLogMetrics(),
	)

	return log.NewLogger(
		node.LoggerName,
		log.WithSink(sink),
		log.WithVerbosity(vLevel),
	).Register(), nil
}
