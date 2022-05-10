package goroutine

import "runtime"

func Dump() string {
	buf := make([]byte, 1<<20)
	stacklen := runtime.Stack(buf, true)
	return string(buf[:stacklen])
}
