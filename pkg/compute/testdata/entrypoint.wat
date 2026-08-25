;; Same as writer.wat but the function is exported as `run` and there is no
;; `_start`, so it can only be reached via an explicit entrypoint.
(module
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)
  (data (i32.const 0) "\08\00\00\00\11\00\00\00entrypoint output")
  (func (export "run")
    (drop (call $fd_write (i32.const 1) (i32.const 0) (i32.const 1) (i32.const 200)))))
