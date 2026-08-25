;; Exits with a non-zero WASI exit code.
(module
  (import "wasi_snapshot_preview1" "proc_exit" (func $proc_exit (param i32)))
  (func (export "_start") (call $proc_exit (i32.const 1))))
