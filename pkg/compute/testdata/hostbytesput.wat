;; Uploads the payload from stdin with swarm_bytes_put.
;;
;; stdin:  [32-byte batch id][payload]
;; stdout: [4-byte errno][32-byte reference, only when errno is 0]
;;
;; memory map: 0 read iovec, 8/16 write iovecs, 24 nread, 28 nwritten,
;; 32 errno, 512 reference, 1024 batch id, 1056 payload.
(module
  (import "wasi_snapshot_preview1" "fd_read"
    (func $fd_read (param i32 i32 i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (import "swarm" "swarm_bytes_put"
    (func $bytes_put (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)
  (data (i32.const 0) "\00\04\00\00\00\40\00\00\20\00\00\00\04\00\00\00\00\02\00\00\20\00\00\00")
  (func (export "_start")
    (drop (call $fd_read (i32.const 0) (i32.const 0) (i32.const 1) (i32.const 24)))
    (i32.store (i32.const 32)
      (call $bytes_put (i32.const 1024) (i32.const 1056)
        (i32.sub (i32.load (i32.const 24)) (i32.const 32)) (i32.const 512)))
    ;; the reference iovec stays empty unless the call succeeded
    (if (i32.ne (i32.load (i32.const 32)) (i32.const 0))
      (then (i32.store (i32.const 20) (i32.const 0))))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 2) (i32.const 28)))))
