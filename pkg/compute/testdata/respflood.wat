;; Sets distinct headers in a loop until the node refuses, which is how the
;; count and byte caps are observed from inside the sandbox.
;;
;; stdout: [4-byte accepted count][4-byte errno that stopped the loop]
(module
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (import "swarm" "swarm_response_header"
    (func $header (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)

  ;; iovec at 8 -> two results at 256
  (data (i32.const 8) "\00\01\00\00\08\00\00\00")
  (data (i32.const 64) "X-Pad")
  (data (i32.const 80) "0123456789")

  (func (export "_start") (local $n i32) (local $code i32)
    (block $done
      (loop $next
        ;; A distinct name is not needed: duplicates are accepted and each one
        ;; charges the caps just the same.
        (local.set $code
          (call $header (i32.const 64) (i32.const 5) (i32.const 80) (i32.const 10)))
        (br_if $done (i32.ne (local.get $code) (i32.const 0)))
        (local.set $n (i32.add (local.get $n) (i32.const 1)))
        ;; Stop well before forever if the caps were somehow not enforced.
        (br_if $done (i32.gt_u (local.get $n) (i32.const 10000)))
        (br $next)))
    (i32.store (i32.const 256) (local.get $n))
    (i32.store (i32.const 260) (local.get $code))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 1) (i32.const 4)))))
