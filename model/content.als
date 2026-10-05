module content

// Concept §§5–6. Immutable packages; staging is not a second current package.
sig Asset {}
sig AST { refs: set Asset }
sig Package { ast: one AST, assets: set Asset }
one sig Validation {
  jsonOK, astOK: set Package
}
fun valid: set Package {
  { p: Package | p in Validation.jsonOK & Validation.astOK
    and p.ast.refs in p.assets }
}
one sig Store {
  var current, staged, checked: lone Package,
  var visibleAST: lone AST,
  var visibleAssets: set Asset
}
pred init {
  no Store.current + Store.staged + Store.checked
  no Store.visibleAST + Store.visibleAssets
}
pred unchangedPublished {
  Store.current' = Store.current
  Store.visibleAST' = Store.visibleAST
  Store.visibleAssets' = Store.visibleAssets
}
pred upload[p: Package] {
  Store.staged' = p
  no Store.checked' // A new upload invalidates an older validation result.
  unchangedPublished
}
pred validate {
  some Store.staged
  Store.staged' = Store.staged
  Store.checked' = Store.staged & valid
  unchangedPublished
}
pred commit {
  some Store.checked
  Store.current' = Store.checked
  Store.visibleAST' = Store.checked.ast
  Store.visibleAssets' = Store.checked.assets
  no Store.staged' + Store.checked'
}
pred crash {
  // Durable atomic publication is an assumption to implement, not a filesystem proof.
  unchangedPublished
  no Store.staged' + Store.checked'
}
pred idle {
  unchangedPublished
  Store.staged' = Store.staged
  Store.checked' = Store.checked
}
fact Behavior {
  init
  always ((some p: Package | upload[p]) or validate or commit or crash or idle)
}
assert OnlyValidatedPublished {
  always Store.current in valid
}
assert NoMixedPackage {
  always (Store.visibleAST = Store.current.ast
    and Store.visibleAssets = Store.current.assets)
}
assert NoDanglingAsset {
  always Store.visibleAST.refs in Store.visibleAssets
}
assert ValidationBoundToStaging {
  always Store.checked in Store.staged & valid
}
assert RejectionPreservesCurrent {
  always ((validate and Store.staged not in valid) implies
    Store.current' = Store.current)
}
pred ReplaceAfterRejection {
  eventually (some Store.current and some (Store.staged - valid)
    and validate and after (eventually (Store.current' != Store.current)))
}
pred CrashAfterValidation {
  eventually (some Store.checked and crash)
}
pred MissingAssetRejected {
  eventually (some Store.current and some p: Store.staged |
    p in Validation.jsonOK & Validation.astOK
    and some p.ast.refs - p.assets and validate)
}
check OnlyValidatedPublished for 4 but 1..8 steps expect 0
check NoMixedPackage for 4 but 1..8 steps expect 0
check NoDanglingAsset for 4 but 1..8 steps expect 0
check ValidationBoundToStaging for 4 but 1..8 steps expect 0
check RejectionPreservesCurrent for 4 but 1..8 steps expect 0
run ReplaceAfterRejection for 4 but 1..10 steps expect 1
run CrashAfterValidation for 4 but 1..8 steps expect 1
run MissingAssetRejected for 4 but 1..8 steps expect 1
