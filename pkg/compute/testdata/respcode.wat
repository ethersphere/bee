;; Sets one valid header and reports the code it got. Run outermost this is OK
;; (0); reached through swarm_execute it is DENIED (2), because the response
;; belongs to the outermost execution alone.
;;
;; stdout: [4-byte errno]
(module
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (import "swarm" "swarm_response_header"
    (func $header (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)

  (data (i32.const 8) "\20\00\00\00\04\00\00\00")
  (data (i32.const 64) "X-Depth")
  (data (i32.const 80) "here")

  (func (export "_start")
    (i32.store (i32.const 32)
      (call $header (i32.const 64) (i32.const 7) (i32.const 80) (i32.const 4)))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 1) (i32.const 28)))))
