;; Runs another module with swarm_execute and forwards its output.
;;
;; stdin:  [32-byte module address][input for the nested module]
;; stdout: [4-byte errno][4-byte nested output length][nested output]
;;
;; memory map: 0 read iovec, 8/16 write iovecs, 24 nread, 28 nwritten,
;; 32 errno, 36 out_len, 1024 module address, 1056 nested input,
;; 16384 nested output buffer.
(module
  (import "wasi_snapshot_preview1" "fd_read"
    (func $fd_read (param i32 i32 i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (import "swarm" "swarm_execute"
    (func $execute (param i32 i32 i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)
  (data (i32.const 0) "\00\04\00\00\00\20\00\00\20\00\00\00\08\00\00\00\00\40\00\00\00\00\00\00")
  (func (export "_start")
    (drop (call $fd_read (i32.const 0) (i32.const 0) (i32.const 1) (i32.const 24)))
    (i32.store (i32.const 32)
      (call $execute (i32.const 1024) (i32.const 1056)
        (i32.sub (i32.load (i32.const 24)) (i32.const 32))
        (i32.const 16384) (i32.const 8192) (i32.const 36)))
    (if (i32.eqz (i32.load (i32.const 32)))
      (then (i32.store (i32.const 20) (i32.load (i32.const 36)))))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 2) (i32.const 28)))))
