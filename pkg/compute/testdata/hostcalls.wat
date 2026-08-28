;; Fetches the same address over and over until a host call is refused, so a
;; test can see exactly where the call budget cuts the module off.
;;
;; stdin:  [32-byte address]
;; stdout: [4-byte successful call count][4-byte errno that stopped the loop]
;;
;; memory map: 0 read iovec, 8 write iovec, 24 nread, 28 nwritten, 32 count,
;; 36 errno, 40 out_len, 64 address, 256 payload buffer.
(module
  (import "wasi_snapshot_preview1" "fd_read"
    (func $fd_read (param i32 i32 i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (import "swarm" "swarm_bytes_get"
    (func $bytes_get (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)
  (data (i32.const 0) "\40\00\00\00\20\00\00\00\20\00\00\00\08\00\00\00")
  (func (export "_start")
    (local $n i32)
    (drop (call $fd_read (i32.const 0) (i32.const 0) (i32.const 1) (i32.const 24)))
    (block $done
      (loop $again
        (i32.store (i32.const 36)
          (call $bytes_get (i32.const 64) (i32.const 256) (i32.const 4096) (i32.const 40)))
        (br_if $done (i32.ne (i32.load (i32.const 36)) (i32.const 0)))
        (local.set $n (i32.add (local.get $n) (i32.const 1)))
        (i32.store (i32.const 32) (local.get $n))
        ;; a safety stop, so a budget that never bites cannot hang the test
        (br_if $again (i32.lt_u (local.get $n) (i32.const 10000)))))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 1) (i32.const 28)))))
