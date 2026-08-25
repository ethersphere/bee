;; Loops forever; only an external interrupt stops it.
(module
  (func (export "_start")
    (loop $l (br $l))))
