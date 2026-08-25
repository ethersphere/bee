;; Writes the single environment entry the sandbox provides
;; ("REQUEST_METHOD=<method>") to stdout, so tests can observe that request
;; metadata reaches the guest.
;;
;; environ_sizes_get stores the entry count at address 0 and the buffer size at
;; address 4; environ_get stores the pointer array at 64 and the NUL-terminated
;; strings at 128. The write iovec lives at address 8 and its length is the
;; buffer size minus the trailing NUL (zero when no environment is provided).
(module
  (import "wasi_snapshot_preview1" "environ_sizes_get"
    (func $environ_sizes_get (param i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "environ_get"
    (func $environ_get (param i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)
  (func (export "_start")
    (drop (call $environ_sizes_get (i32.const 0) (i32.const 4)))
    (drop (call $environ_get (i32.const 64) (i32.const 128)))
    (i32.store (i32.const 8) (i32.const 128))
    (i32.store (i32.const 12)
      (select
        (i32.const 0)
        (i32.sub (i32.load (i32.const 4)) (i32.const 1))
        (i32.eqz (i32.load (i32.const 4)))))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 1) (i32.const 16)))))
