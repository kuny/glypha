module lifecycle
open util/ordering[Tick]

// Integration of the six accepted decisions; Tick abstracts second timestamps.
sig Tick {}
sig Asset {}
sig AST { refs: set Asset }
one sig Builtin extends AST {}
sig Package { assets: set Asset }
sig Entry { owner: one Package, at: one Tick, ast: one AST }
one sig Valid { packages: set Package, scenes: set AST }
abstract sig Availability {}
one sig Up, Down extends Availability {}
fact Input {
  no Builtin.refs
  Builtin in Valid.scenes
  all p: Valid.packages, e: owner.p |
    e.ast in Valid.scenes and e.ast.refs in p.assets
  all disj a, b: Entry | a.owner = b.owner implies a.at != b.at
}
one sig State {
  var now: one Tick,
  var online: one Availability,
  // Durable transaction: current package, target and scheduling progress.
  var current: lone Package,
  var target: lone AST,
  var pending, consumed: set Entry,
  var used: set Package,
  // Renderer-owned snapshots; never references server asset lifetime.
  var response: lone AST,
  var copied: set Asset,
  var display: one AST,
  var retained: set Asset
}
fun elapsed[p: Package]: set Entry {
  { e: owner.p | lte[e.at, State.now] }
}
fun latest[es: set Entry]: set Entry {
  { e: es | no later: es | gt[later.at, e.at] }
}
fun due: set Entry { { e: State.pending | lte[e.at, State.now] } }
pred durableUnchanged {
  State.current' = State.current
  State.target' = State.target
  State.pending' = State.pending
  State.consumed' = State.consumed
  State.used' = State.used
}
pred rendererUnchanged {
  State.response' = State.response
  State.copied' = State.copied
  State.display' = State.display
  State.retained' = State.retained
}
pred environmentUnchanged {
  State.now' = State.now
  State.online' = State.online
}
pred init {
  State.now = first
  State.online = Up
  no State.current + State.target + State.pending + State.consumed + State.used
  no State.response + State.copied + State.retained
  State.display = Builtin
}
pred activate[p: Package] {
  State.online = Up
  p in Valid.packages - State.used
  some elapsed[p] // Re-evaluate at commit time, not only at upload time.
  State.current' = p
  State.target' = latest[elapsed[p]].ast
  State.pending' = owner.p - elapsed[p]
  State.consumed' = State.consumed + elapsed[p]
  State.used' = State.used + p
  rendererUnchanged
  environmentUnchanged
}
pred advance {
  State.online = Up
  some due
  State.target' = latest[due].ast
  State.pending' = State.pending - due
  State.consumed' = State.consumed + due
  State.current' = State.current
  State.used' = State.used
  rendererUnchanged
  environmentUnchanged
}
pred fetch {
  State.online = Up
  some State.target
  no State.response // At most one acquisition/render operation in flight.
  State.response' = State.target
  State.copied' = State.target.refs
  State.display' = State.display
  State.retained' = State.retained
  durableUnchanged
  environmentUnchanged
}
pred render {
  some State.response
  State.response in Valid.scenes
  State.response.refs in State.copied
  State.display' = State.response
  State.retained' = State.copied
  no State.response' + State.copied'
  durableUnchanged
  environmentUnchanged
}
pred failure {
  // Failed/partial acquisition or preparation is discarded before screen swap.
  no State.response' + State.copied'
  State.display' = State.display
  State.retained' = State.retained
  durableUnchanged
  environmentUnchanged
}
pred clock {
  durableUnchanged
  rendererUnchanged
  State.online' = State.online
  // now' arbitrary: forward, backward, or stopped.
}
pred restart {
  durableUnchanged
  rendererUnchanged
  State.now' = State.now
  State.online' != State.online
}
fact Behavior {
  init
  always ((some p: Package | activate[p]) or advance or fetch or render
    or failure or clock or restart)
}
assert CoherentGeneration {
  always (State.current in Valid.packages
    and State.target in (owner.(State.current)).ast
    and State.pending in owner.(State.current)
    and (some State.current implies one State.target))
}
assert DurableNoReplay {
  always (State.consumed in State.consumed'
    and no State.pending & State.consumed)
}
assert DisplayOwnsAssets {
  always (State.display in Valid.scenes and State.display.refs in State.retained)
}
assert ResponseOwnsAssets {
  always State.response.refs in State.copied
}
assert RestartPreservesTarget {
  always (restart implies (State.target' = State.target
    and State.pending' = State.pending and State.consumed' = State.consumed))
}
assert CatchUpLatest {
  always (advance implies State.target' = latest[due].ast)
}
assert FailurePreservesScene {
  always (failure implies (State.display' = State.display
    and State.retained' = State.retained))
}
pred ReplaceDuringFetch {
  eventually (some State.response and some p: Package | activate[p]
    and some State.copied - p.assets and after render)
}
pred RestartThenRollback {
  eventually (some State.consumed and State.online = Up and restart
    and after (clock and lt[State.now', State.now]
      and after (restart and after (State.online = Up))))
}
pred RetryAfterLoss {
  eventually (fetch and after (failure and after (fetch and after render)))
}
pred SkipSeveral {
  eventually (#due > 1 and advance)
}
check CoherentGeneration for 3 but 1..7 steps expect 0
check DurableNoReplay for 3 but 1..7 steps expect 0
check DisplayOwnsAssets for 3 but 1..7 steps expect 0
check ResponseOwnsAssets for 3 but 1..7 steps expect 0
check RestartPreservesTarget for 3 but 1..7 steps expect 0
check CatchUpLatest for 3 but 1..7 steps expect 0
check FailurePreservesScene for 3 but 1..7 steps expect 0
run ReplaceDuringFetch for 3 but 1..8 steps expect 1
run RestartThenRollback for 3 but 1..8 steps expect 1
run RetryAfterLoss for 3 but 1..8 steps expect 1
run SkipSeveral for 3 but 1..8 steps expect 1
