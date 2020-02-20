package streamtest

import "time"

func SetFullCloseTimeout(t time.Duration) {
	FullCloseTimeout = t
}

func ResetFullCloseTimeout() {
	FullCloseTimeout = fullCLoseTimeoutDefault
}
