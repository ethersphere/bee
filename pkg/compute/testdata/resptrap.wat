;; Sets a status and a header, writes output, then traps. The output survives as
;; evidence; the response metadata does not, exactly as a trapped module's
;; uploads do not.
(module
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (import "swarm" "swarm_response_status"
    (func $status (param i32) (result i32)))
  (import "swarm" "swarm_response_header"
    (func $header (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)

  (data (i32.const 8) "\20\00\00\00\04\00\00\00")
  (data (i32.const 32) "part")
  (data (i32.const 64) "Content-Type")
  (data (i32.const 80) "text/plain")

  (func (export "_start")
    (drop (call $status (i32.const 418)))
    (drop (call $header (i32.const 64) (i32.const 12) (i32.const 80) (i32.const 10)))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 1) (i32.const 4)))
    (unreachable)))
