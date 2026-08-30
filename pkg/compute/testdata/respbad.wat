;; Every way a response call is refused, in one module. Nothing traps: each call
;; reports a code the guest could branch on.
;;
;; stdout: [6 x 4-byte errno] for, in order:
;;   a name containing CR/LF      -> INVALID (5)
;;   a Swarm-Wasm-Status override -> DENIED  (2)
;;   Access-Control-Allow-Origin  -> DENIED  (2)
;;   Set-Cookie                   -> DENIED  (2)
;;   status 99                    -> INVALID (5)
;;   a value length of 0xffffffff -> INVALID (5)
(module
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (import "swarm" "swarm_response_status"
    (func $status (param i32) (result i32)))
  (import "swarm" "swarm_response_header"
    (func $header (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 1)

  ;; iovec at 8 -> six results at 256
  (data (i32.const 8) "\00\01\00\00\18\00\00\00")

  (data (i32.const 64)  "X\0d\0aInjected")               ;; 64,  len 11
  (data (i32.const 80)  "v")                             ;; 80,  len 1
  (data (i32.const 96)  "Swarm-Wasm-Status")             ;; 96,  len 17
  (data (i32.const 128) "trap")                          ;; 128, len 4
  (data (i32.const 144) "Access-Control-Allow-Origin")   ;; 144, len 27
  (data (i32.const 176) "*")                             ;; 176, len 1
  (data (i32.const 192) "Set-Cookie")                    ;; 192, len 10
  (data (i32.const 208) "a=b")                           ;; 208, len 3
  (data (i32.const 224) "X-Ok")                          ;; 224, len 4

  (func (export "_start")
    (i32.store (i32.const 256)
      (call $header (i32.const 64) (i32.const 11) (i32.const 80) (i32.const 1)))
    (i32.store (i32.const 260)
      (call $header (i32.const 96) (i32.const 17) (i32.const 128) (i32.const 4)))
    (i32.store (i32.const 264)
      (call $header (i32.const 144) (i32.const 27) (i32.const 176) (i32.const 1)))
    (i32.store (i32.const 268)
      (call $header (i32.const 192) (i32.const 10) (i32.const 208) (i32.const 3)))
    (i32.store (i32.const 272)
      (call $status (i32.const 99)))
    ;; An absurd length must be refused before any memory is read.
    (i32.store (i32.const 276)
      (call $header (i32.const 224) (i32.const 4) (i32.const 80) (i32.const 0xffffffff)))
    (drop (call $fd_write (i32.const 1) (i32.const 8) (i32.const 1) (i32.const 4)))))
