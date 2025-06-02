//go:build js
// +build js

package log

import "io"

// NewLogger is a factory constructor which returns a new logger instance
// based on the given name. If such an instance already exists in the
// logger registry, then this existing instance is returned instead.
// The given options take precedence over the default options set
// by the ModifyDefaults function.
func NewLogger(name string, opts ...Option) Logger {
	// Pin the default settings if
	// they are not already pinned.
	ModifyDefaults()

	options := *defaults.options
	for _, modify := range opts {
		modify(&options)
	}

	if options.sink == io.Discard {
		return Noop
	}

	formatter := defaults.formatter
	if options.fmtOptions != defaults.options.fmtOptions {
		formatter = newFormatter(options.fmtOptions)
	}

	val, ok := loggers.Load(hash(name, 0, "", options.sink))
	if ok {
		return val.(*logger)
	}

	l := &logger{
		formatter:  formatter,
		verbosity:  options.verbosity,
		sink:       options.sink,
		levelHooks: options.levelHooks,
	}
	l.builder = &builder{
		l:        l,
		names:    []string{name},
		namesStr: name,
	}
	return l
}
