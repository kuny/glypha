module renderer

// Concept §§11–14. AST validity and actual render success are separate.
sig AST {}
one sig Builtin extends AST {}
one sig Valid { asts: set AST }
fact BuiltinValid { Builtin in Valid.asts }
one sig System {
  var target: lone AST,
  var response: lone AST,
  var display: one AST
}
pred init {
  no System.target + System.response
  System.display = Builtin
}
pred select[a: AST] {
  a in Valid.asts
  System.target' = a
  // In-flight responses may refer to a previously selected AST.
  System.response' = System.response
  System.display' = System.display
}
pred fetch {
  some System.target
  System.response' = System.target
  System.target' = System.target
  System.display' = System.display
}
pred invalidResponse[a: AST] {
  a not in Valid.asts
  System.response' = a
  System.target' = System.target
  System.display' = System.display
}
pred renderSuccess {
  some System.response
  System.response in Valid.asts
  System.display' = System.response
  System.target' = System.target
  no System.response'
}
pred failure {
  // Network/server failure, invalid response, or failure while rendering valid AST.
  System.display' = System.display
  System.target' = System.target
  no System.response'
}
pred idle {
  System.target' = System.target
  System.response' = System.response
  System.display' = System.display
}
fact Behavior {
  init
  always ((some a: AST | select[a] or invalidResponse[a])
    or fetch or renderSuccess or failure or idle)
}
assert DisplayAlwaysValid { always System.display in Valid.asts }
assert FailurePreservesDisplay {
  always (failure implies System.display' = System.display)
}
assert ChangeRequiresSuccessfulRender {
  always (System.display' != System.display implies renderSuccess)
}
// This is intentionally false: no fairness or eventual network recovery assumed.
assert UnconditionalDelivery {
  always (all a: System.target | eventually System.display = a)
}
// A conditional progress claim: stable target and repeated successful fetch/render.
assert DeliveryWithRecovery {
  all a: AST | (eventually always (System.target = a
    and eventually (fetch and after renderSuccess)))
    implies eventually always System.display = a
}
// Even after recovery, one old in-flight response can be successfully rendered.
assert AlwaysLatest {
  always (renderSuccess implies System.display' = System.target)
}
pred RecoverAfterFailure {
  eventually (System.display != Builtin and some System.response and failure
    and after (fetch and after renderSuccess))
}
pred InvalidWhileDisplaying {
  eventually (System.display != Builtin and some (System.response - Valid.asts)
    and failure)
}
pred ValidButRenderFails {
  eventually (some System.response and System.response in Valid.asts
    and System.response != System.display and failure)
}
check DisplayAlwaysValid for 4 but 1..8 steps expect 0
check FailurePreservesDisplay for 4 but 1..8 steps expect 0
check ChangeRequiresSuccessfulRender for 4 but 1..8 steps expect 0
check UnconditionalDelivery for 4 but 1..8 steps expect 1
check AlwaysLatest for 4 but 1..8 steps expect 1
check DeliveryWithRecovery for 4 but 1..8 steps expect 0
run RecoverAfterFailure for 4 but 1..10 steps expect 1
run InvalidWhileDisplaying for 4 but 1..8 steps expect 1
run ValidButRenderFails for 4 but 1..8 steps expect 1
