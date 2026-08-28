;; Calls swarm_bytes_get with an address and buffer length taken from stdin.
;;
;; stdin:  [32-byte address][4-byte buffer length, little-endian]
;; stdout: [4-byte errno][4-byte required length][payload, only when errno is 0]
;;
;; Reporting the required length unconditionally is what lets a test assert the
;; two-call pattern: a buffer length of 0 comes back BUFFER_TOO_SMALL (4) with
;; the length the guest would have to allocate.
;;
;; memory map: 0 read iovec, 8/16 write iovecs, 24 nread, 28 nwritten,
;; 32 errno, 36 out_len, 64 address, 96 buffer length, 256 payload buffer.
(module
  (import "wasi_snapshot_preview1" "fd_read"
    (func $fd_read (param i32 i32 i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (import "swarm" "swarm_bytes_get"
    (func $bytes_get (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)
  (data (i32.const 0) "\40\00\00\00\24\00\00\00\20\00\00\00\08\00\00\00\00\01\00\00\00\00\00\00")
  (func (export "_start")
    (drop (call $fd_read (i32.const 0) (i32.const 0) (i32.const 1) (i32.const 24)))
    (i32.store (i32.const 32)
      (call $bytes_get (i32.const 64) (i32.const 256) (i32.load (i32.const 96)) (i32.const 36)))
    ;; the payload iovec stays empty unless the call succeeded
    (if (i32.eqz (i32.load (i32.const 32)))
      (then (i32.store (i32.const 20) (i32.load (i32.const 36)))))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 2) (i32.const 28)))))
