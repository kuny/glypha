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
