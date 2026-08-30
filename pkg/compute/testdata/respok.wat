;; Sets a status and three response headers, then writes a body. The third
;; header repeats a name, which is legitimate (Link, Vary) and must be kept in
;; order rather than collapsed.
;;
;; stdout: "hi"
(module
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (import "swarm" "swarm_response_status"
    (func $status (param i32) (result i32)))
  (import "swarm" "swarm_response_header"
    (func $header (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)

  ;; iovec at 8 -> the body at 32
  (data (i32.const 8) "\20\00\00\00\02\00\00\00")
  (data (i32.const 32) "hi")

  ;; header names and values, laid out end to end
  (data (i32.const 64)  "Content-Type")        ;; 64,  len 12
  (data (i32.const 80)  "text/css")            ;; 80,  len 8
  (data (i32.const 96)  "Cache-Control")       ;; 96,  len 13
  (data (i32.const 112) "max-age=60")          ;; 112, len 10
  (data (i32.const 128) "Link")                ;; 128, len 4
  (data (i32.const 144) "</a>; rel=next")      ;; 144, len 14

  (func (export "_start")
    (drop (call $status (i32.const 201)))
    (drop (call $header (i32.const 64)  (i32.const 12) (i32.const 80)  (i32.const 8)))
    (drop (call $header (i32.const 96)  (i32.const 13) (i32.const 112) (i32.const 10)))
    (drop (call $header (i32.const 128) (i32.const 4)  (i32.const 144) (i32.const 14)))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 1) (i32.const 4)))))
