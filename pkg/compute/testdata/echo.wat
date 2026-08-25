;; Reads up to 64 bytes from stdin and writes them straight back to stdout.
;; The read iovec lives at address 0, the write iovec at address 8, and both
;; point at the 64-byte scratch buffer at address 16.
(module
  (import "wasi_snapshot_preview1" "fd_read"
    (func $fd_read (param i32 i32 i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)
  (data (i32.const 0) "\10\00\00\00\40\00\00\00\10\00\00\00\00\00\00\00")
  (func (export "_start")
    (drop (call $fd_read (i32.const 0) (i32.const 0) (i32.const 1) (i32.const 200)))
    ;; copy the number of bytes read into the length field of the write iovec
    (i32.store (i32.const 12) (i32.load (i32.const 200)))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 1) (i32.const 204)))))
