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

The development process should not depend on preserving a particular AI conversation.

The repository contains the context required to continue development:

- concept
- invariants
- Alloy models
- design decisions
- source code
- tests

An AI should be able to enter the project by reading these artifacts rather than inheriting the entire conversation that produced them.

This also means that the AI itself is replaceable.

Different models or agents may perform different parts of the development process as long as they work from the same explicit constraints.

## Principle

The objective is not to write increasingly detailed prompts that tell an AI how to implement the system.

The objective is to make the system's intent and constraints precise enough that implementation becomes a replaceable activity.

In short:

> **Humans own intent and invariants.  
> AI explores and implements.  
> Formal models challenge both.**
