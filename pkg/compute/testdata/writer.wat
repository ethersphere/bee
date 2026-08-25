;; Writes "hello swarm" to stdout (fd 1) and returns from the WASI
;; command entrypoint `_start`.
(module
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)
  (data (i32.const 0) "\08\00\00\00\0b\00\00\00hello swarm")
  (func (export "_start")
    (drop (call $fd_write (i32.const 1) (i32.const 0) (i32.const 1) (i32.const 200)))))
