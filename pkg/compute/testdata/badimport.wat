;; Imports a host function the sandbox does not provide.
(module
  (import "env" "does_not_exist" (func $missing))
  (func (export "_start") (call $missing)))
