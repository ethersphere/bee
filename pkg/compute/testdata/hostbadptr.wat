;; Passes an address pointer far outside linear memory. The host bounds-checks
;; every offset, so this must come back INVALID (5) rather than trapping.
;;
;; stdout: [4-byte errno]
(module
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (import "swarm" "swarm_bytes_get"
    (func $bytes_get (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)
  (data (i32.const 8) "\20\00\00\00\04\00\00\00")
  (func (export "_start")
    (i32.store (i32.const 32)
      (call $bytes_get (i32.const 0xffff0000) (i32.const 256) (i32.const 4096) (i32.const 40)))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 1) (i32.const 28)))))
