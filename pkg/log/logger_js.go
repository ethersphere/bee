//go:build js
// +build js

package log

import "io"

// logger implements the Logger interface.
type logger struct {
	*builder

	// id is the unique identifier of a logger.
	// It identifies the instance of a logger in the logger registry.
	id string

	// formatter formats log messages before they are written to the sink.
	formatter *formatter

	// verbosity represents the current verbosity level.
	// This variable is used to modify the verbosity of the logger instance.
	// Higher values enable more logs. Logs at or below this level
	// will be written, while logs above this level will be discarded.
	verbosity Level

	// sink represents the stream where the logs are written.
	sink io.Writer

	// levelHooks allow triggering of registered hooks
	// on their associated severity log levels.
	levelHooks levelHooks
}
