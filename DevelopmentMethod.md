# Development Method

Glypha was developed without a conventional software specification.

The development process started with intent and invariants rather than a detailed list of functional requirements.

```text
Intent
  ↓
Concept
  ↓
Formal Model (Alloy)
  ↓
Counterexamples
  ↓
Design Decisions
  ↓
Implementation
  ↓
Tests
```

## Human Responsibility

The human defines the meaning of the system.

Examples:

- What is Glypha responsible for?
- What is deliberately outside its responsibility?
- What must always remain true?
- What should happen when time moves backward?
- What should remain visible when something fails?

These decisions are not delegated to the AI.

## AI Responsibility

AI is used to transform intent into increasingly concrete artifacts:

- formal models
- design alternatives
- implementation
- tests
- documentation

The AI is not treated as the authority on correctness.

Its output is constrained and challenged by formal models, counterexamples, tests, and explicit design invariants.

## Formal Methods

Alloy is used during design, not merely as a final verification step.

The initial model is allowed to be wrong.

Counterexamples are used to expose missing assumptions and ambiguous behavior before those assumptions become implementation details.

The process is therefore iterative:

```text
Intent
  ↓
Model
  ↓
Counterexample
  ↓
Decision
  ↓
Refined Model
```

## Review Between Two AI Roles

After implementation, the human relayed findings between two AI roles: AI 1 evaluated the models, tests, and their correspondence; AI 2 checked those findings against the repository and made corrections or added tests. AI 1 then reviewed the changes, and the cycle continued until the identified issues were resolved.

```text
AI 1: Evaluate models, tests, and coverage claims
  ↓
Human: Relay findings
  ↓
AI 2: Check findings, revise documentation, and add tests where useful
  ↓
AI 1: Re-evaluate the changes
  ↓
Repeat when actionable issues remain
```

In this review, the correspondence table in `model/README.md` made the claims concrete enough to challenge. AI 2 corrected descriptions that overstated what existing tests observed and added two browser scenarios: rendering an old response after the target changed, then adopting the new target; and recovering from fetch and render failures while the target stayed fixed, then retaining the recovered frame through conditional responses.

AI 1 reported running the browser checks and type checks, and using temporary mutations to assess whether tests detected injected faults. The review also found that the new target-replacement test used identical image assets in both generations. It therefore did not cover the asset-difference condition in Alloy's `ReplaceDuringFetch` scenario. AI 2 removed that citation and documented the limitation; AI 1 reviewed and accepted the correction.

This cycle improved both the test scenarios and the accuracy of the coverage claims. It did not make agreement between AIs a proof of correctness. Finite tests remain observations of selected executions, and Alloy checks remain subject to their modeled assumptions and checked bounds. Some findings required clearer documentation rather than more tests. Target-device trials and CI remained deferred.

## The Repository as Context

The repository records the main artifacts produced during development:

- concept
- invariants
- Alloy models
- design decisions
- source code
- tests

These artifacts provide a starting point for understanding the project and continuing work. Keeping them up to date can reduce reliance on the original AI conversation.

Some reasoning, assumptions, and discussion may still need to be recovered from the conversation or clarified with the human. A handover to another model or agent would need to be evaluated in practice.

## Principle

The aim is to make intent, constraints, and decisions explicit enough to support implementation and review.

AI chat helped develop the concept, explore alternatives, and turn agreed decisions into code and tests. Human review, formal models, and implementation tests each contributed to checking that work within their respective limits.

This describes the process used for Glypha. Its effectiveness in other projects or with different AI tools remains to be evaluated.
