module scheduler

// Concept §§7–10,19. One content generation; seconds are explicit clock inputs.
sig AST {}
sig Entry { start: one Int, ast: one AST }
one sig Scheduler {
  var now: one Int,
  var issued, pending, consumed: set Entry,
  var target: lone AST
}
fact TimeDomain {
  all e: Entry | e.start >= 0 and e.start <= 20
  all disj a, b: Entry | a.start != b.start
  always (Scheduler.now >= 0 and Scheduler.now <= 30)
}
fun due: set Entry { { e: Scheduler.pending | e.start <= Scheduler.now } }
fun winner: set Entry {
  { e: due | no later: due | later.start > e.start }
}
pred init {
  Scheduler.now = 0
  no Scheduler.issued + Scheduler.pending + Scheduler.consumed
  no Scheduler.target
}
pred clock {
  // No monotonicity constraint. Includes stopped time and arbitrary jumps.
  Scheduler.issued' = Scheduler.issued
  Scheduler.pending' = Scheduler.pending
  Scheduler.consumed' = Scheduler.consumed
  Scheduler.target' = Scheduler.target
}
pred generate {
  let batch = Scheduler.pending' | {
    no Scheduler.pending
    some batch
    batch in Entry - Scheduler.issued
    // Frontier survives clock rollback; never regenerate a historical interval.
    all e: batch, old: Scheduler.issued | e.start > old.start
    Scheduler.issued' = Scheduler.issued + batch
    Scheduler.pending' = batch
    Scheduler.consumed' = Scheduler.consumed
    Scheduler.now' = Scheduler.now
    Scheduler.target' = Scheduler.target
  }
}
pred poll {
  Scheduler.pending' = Scheduler.pending - due
  // Consumed includes skipped/obsolete entries, not just rendered entries.
  Scheduler.consumed' = Scheduler.consumed + due
  Scheduler.issued' = Scheduler.issued
  Scheduler.now' = Scheduler.now
  (some winner implies Scheduler.target' = winner.ast)
  (no winner implies Scheduler.target' = Scheduler.target)
}
fact Behavior {
  init
  always (clock or poll or generate)
}
assert Partition {
  always (Scheduler.issued = Scheduler.pending + Scheduler.consumed
    and no Scheduler.pending & Scheduler.consumed)
}
assert NoReplay {
  always Scheduler.consumed in Scheduler.consumed'
}
assert SingleWinner { always lone winner }
assert NoEarlyTransition {
  always (Scheduler.target' != Scheduler.target implies
    (some e: winner | Scheduler.target' = e.ast and e.start <= Scheduler.now))
}
assert PollDrainsObsolete {
  always (poll implies no (due & Scheduler.pending'))
}
assert NoHistoricalRegeneration {
  always (all e: Scheduler.issued' - Scheduler.issued, old: Scheduler.issued |
    e.start > old.start)
}
// Catch up even when the original ten-second window was missed.
assert EveryDuePollHasTarget {
  always ((poll and some due and no Scheduler.target) implies some Scheduler.target')
}
pred RollbackAfterConsumption {
  eventually (some Scheduler.consumed and Scheduler.now' < Scheduler.now)
}
pred CatchUpAfterGap {
  eventually (some e: winner | Scheduler.now >= add[e.start, 10]
    and poll and Scheduler.target' = e.ast)
}
pred SkipIntermediate {
  eventually (#due > 1 and poll and Scheduler.target' = winner.ast)
}
pred RefillPastDue {
  eventually (no Scheduler.pending and some Scheduler.consumed
    and generate and after (some due and poll))
}
pred ExhaustAndRefill {
  eventually (some Scheduler.consumed and no Scheduler.pending
    and some Scheduler.pending')
}
pred StoppedClockProgress {
  eventually (some winner and poll and Scheduler.now' = Scheduler.now
    and Scheduler.consumed' != Scheduler.consumed)
}
check Partition for 4 but 6 Int, 1..8 steps expect 0
check NoReplay for 4 but 6 Int, 1..8 steps expect 0
check SingleWinner for 4 but 6 Int, 1..8 steps expect 0
check NoEarlyTransition for 4 but 6 Int, 1..8 steps expect 0
check PollDrainsObsolete for 4 but 6 Int, 1..8 steps expect 0
check NoHistoricalRegeneration for 4 but 6 Int, 1..8 steps expect 0
check EveryDuePollHasTarget for 3 but 6 Int, 1..6 steps expect 0
run RollbackAfterConsumption for 3 but 6 Int, 1..8 steps expect 1
run CatchUpAfterGap for 3 but 6 Int, 1..6 steps expect 1
run SkipIntermediate for 3 but 6 Int, 1..6 steps expect 1
run RefillPastDue for 3 but 6 Int, 1..8 steps expect 1
run ExhaustAndRefill for 3 but 6 Int, 1..8 steps expect 1
run StoppedClockProgress for 3 but 6 Int, 1..6 steps expect 1
