;; Imports a function the swarm host module does not define. Imports are checked
;; before instantiation, so this is StatusInvalidModule and never a link trap.
(module
  (import "swarm" "swarm_nope" (func $nope (param i32) (result i32)))
  (memory (export "memory") 1)
  (func (export "_start")
    (drop (call $nope (i32.const 0)))))
