;; Stores a chunk with swarm_chunk_put and reads it straight back with
;; swarm_chunk_get, so a test sees the raw chunk pair round-trip.
;;
;; stdin:  [32-byte batch id][chunk data, span included]
;; stdout: [4-byte put errno][4-byte get errno][retrieved chunk data]
;;
;; memory map: 0 read iovec, 8/16 write iovecs, 24 nread, 28 nwritten,
;; 32 put errno, 36 get errno, 40 out_len, 512 reference, 1024 batch id,
;; 1056 chunk data, 16384 retrieved buffer.
(module
  (import "wasi_snapshot_preview1" "fd_read"
    (func $fd_read (param i32 i32 i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (import "swarm" "swarm_chunk_put"
    (func $chunk_put (param i32 i32 i32 i32) (result i32)))
  (import "swarm" "swarm_chunk_get"
    (func $chunk_get (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)
  (data (i32.const 0) "\00\04\00\00\00\20\00\00\20\00\00\00\08\00\00\00\00\40\00\00\00\00\00\00")
  (func (export "_start")
    (drop (call $fd_read (i32.const 0) (i32.const 0) (i32.const 1) (i32.const 24)))
    (i32.store (i32.const 32)
      (call $chunk_put (i32.const 1024) (i32.const 1056)
        (i32.sub (i32.load (i32.const 24)) (i32.const 32)) (i32.const 512)))
    (if (i32.eqz (i32.load (i32.const 32)))
      (then
        (i32.store (i32.const 36)
          (call $chunk_get (i32.const 512) (i32.const 16384) (i32.const 4104) (i32.const 40)))
        (if (i32.eqz (i32.load (i32.const 36)))
          (then (i32.store (i32.const 20) (i32.load (i32.const 40)))))))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 2) (i32.const 28)))))
