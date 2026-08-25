;; Declares 100 pages (6.4 MiB) of linear memory so it can be rejected by a
;; lower memory limit.
(module
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 100)
  (data (i32.const 0) "\08\00\00\00\03\00\00\00big")
  (func (export "_start")
    (drop (call $fd_write (i32.const 1) (i32.const 0) (i32.const 1) (i32.const 200)))))
