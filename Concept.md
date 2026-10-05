# Glypha — Concept

> "Stand out of my sun."  
> — Diogenes

## 1. What is Glypha?

Glypha is a digital signage system with almost no features.

It is not designed to compete by adding more functions.

Instead, Glypha asks a different question:

**How little does a digital signage system need in order to keep displaying?**

Glypha is an answer to feature-driven system design: systems that become larger and more complex as functions, management tools, configuration options, and operational features accumulate.

The goal is not to make a feature-rich signage platform.

The goal is to make a **small, understandable, resilient system whose primary responsibility is simply to keep displaying content.**

---

## 2. Design Philosophy

### Fewer features, fewer failure modes

Features are not free.

Every feature introduces additional states, dependencies, transitions, assumptions, and potential failures.

Glypha deliberately reduces them.

Complexity should only exist where it directly contributes to the essential purpose of the system.

### Correctness before convenience

Glypha is intentionally minimal, but its internal design is not casual.

The system should have explicit responsibilities, invariants, validation rules, and failure behavior.

The objective is not:

> Never fail.

The objective is:

> Remain displayable and recoverable when things fail.

### Failure is normal

Networks fail.

Servers stop.

Processes restart.

Clocks jump forward.

Clocks move backward.

Time synchronization adjusts the system clock.

Content uploads fail.

These are not exceptional situations outside the design.

They are part of the system model.

---

## 3. Only Two Components

Glypha consists of only two software components:

```
              HTTP
               |
               v
        +-------------+
        |   Server    |
        |-------------|
        | Validation  |
        | Scheduling  |
        | Storage     |
        +------+------+
               |
              AST
               |
               v
        +-------------+
        |  Renderer   |
        |-------------|
        |   Render    |
        +-------------+
```

### Server

The server is responsible for:

- accepting content
    
- validating content
    
- storing the current valid content
    
- generating future display schedules
    
- determining whether content should change
    
- providing AST content to the renderer
    

### Renderer

The renderer is responsible for one thing:

> **Render a valid AST.**

The renderer does not calculate schedules.

It does not understand time slots.

It does not need to know why particular content should currently be displayed.

It does not attempt to recover historical display states.

---

## 4. Content Model

A Glypha content package consists of:

- one JSON document
    
- image files referenced by the JSON
    

The content describes simple static scenes.

A scene contains only a small number of primitives:

```
Scene
├── Background
│   ├── Color
│   └── Image
└── Elements
    ├── Text
    └── Image
```

Initially, Glypha deliberately does not support:

- animation
    
- video
    
- external weather APIs
    
- live data integrations
    
- rapidly rotating advertisements
    
- complex interactive content
    

A single time period displays a single content scene.

For example:

```
10:00 ───────── Content A ───────── 13:00
13:00 ───────── Content B ───────── 18:00
18:00 ───────── Content C ───────── ...
```

Glypha is designed around changes between meaningful time periods, not content rotation every few seconds.

---

## 5. Content Validation

Before accepting content, the server validates it.

Validation should include at least:

- JSON structure
    
- referenced assets
    
- ability to construct a valid AST
    

Where practical, validation should also determine whether the resulting content fits within the target display area.

Invalid content must never replace valid content.

Conceptually:

```
Upload
  |
  v
Validate
  |
  +---- invalid ----> reject
  |
 valid
  |
  v
Replace current content
```

Validation failure must leave the currently valid content untouched.

---

## 6. One Current Content

The server stores only one current content package.

There is no:

- content history
    
- draft system
    
- rollback system
    
- version-management UI
    

When a new package passes validation, it replaces the existing package.

Replacement must behave atomically:

```
old valid content
        |
   validate new
        |
    success?
     /     \
   no       yes
   |         |
 keep      replace
 old       atomically
```

The system must never expose partially updated content.

Acceptance also requires that a scene applicable at publication time can be determined. The current package, its schedule, and its initial display target are published together. Validation failure leaves all three unchanged.

After successful replacement, previous content may be discarded once any ongoing transfer no longer depends on it. The renderer owns the assets needed by its displayed scene.

If history or version management is required, it belongs outside Glypha. Git or another external tool may be used for that purpose.

---

## 7. Scheduling

Scheduling belongs entirely to the server.

The renderer never receives a schedule.

The server transforms the current content definition into a finite list of future display changes.

Conceptually:

```
Content JSON
     |
     v
Schedule generation
     |
     v
(start time, AST)
(start time, AST)
(start time, AST)
       ...
```

For example:

```
10:00 → AST-A
13:00 → AST-B
18:00 → AST-C
```

The generated schedule covers a finite future period.

When the schedule has been consumed, the server generates another schedule for the next period.

---

## 8. Time Is an Explicit Dependency

Scheduling logic must not directly depend on the operating system clock.

The Go implementation should access time through an abstraction such as a Clock.

Conceptually:

```
Clock
  |
  | Now()
  v
Scheduler
```

This allows time itself to become an explicit input to the system.

The scheduler operates at second-level precision.

Sub-second values returned by the underlying clock are discarded.

This makes behavior deterministic enough to test and model without depending on wall-clock timing details.

---

## 9. Schedule Matching

When the renderer asks for content, the server samples the Clock at second-level precision.

Among unconsumed schedule entries whose start time is at or before that time, the server selects the latest one. Earlier due entries are skipped and consumed together with the selected entry.

There is no matching-window expiry. A delayed request or a forward clock jump must still catch up to the latest applicable unconsumed scene. If the generated period has been exhausted, schedule generation must account for the elapsed interval before selecting that scene.

If no unconsumed entry is due, the current display target remains unchanged. Moving the clock backward does not restore consumed entries.

The server retains the current target AST so that the renderer can retrieve it again after a lost response. Consuming a schedule entry does not mean that the renderer has received or displayed it.

---

## 10. Time May Behave Badly

Glypha does not assume that wall-clock time is monotonically increasing.

Time may:

```
advance
stop
jump forward
move backward
```

These events must not corrupt the scheduler.

However, Glypha deliberately does **not** attempt to reconstruct historical display state when time moves backward.

For example:

```
10:00 → A
13:00 → B
18:00 → C
```

If A and B have already been consumed and the clock later moves backward, those schedule entries are not restored.

A clock rollback is not a replay command.

Previously consumed schedule entries remain consumed, including across server restarts. The server durably preserves the consumption boundary and current display target together with the current content generation.

Conceptually:

```
Pending → Consumed

Consumed → Pending   // forbidden
```

The consumed portion of the schedule therefore moves in only one direction even when the clock does not.

A useful invariant is:

```
Consumed' ⊇ Consumed
```

In other words:

> **Time may move backward. System history does not.**

---

## 11. Renderer

The renderer should be deliberately simple.

Its job is not to manage a signage system.

Its job is to render an AST.

It has no scheduling logic and should contain as little business logic as practical.

Conceptually:

```
AST
 |
 v
Renderer
 |
 v
Display
```

A valid AST describes a static scene containing background, text, and image elements.

---

## 12. Renderer State

The renderer should preserve the last successfully rendered content.

Conceptually:

```
Initial
   |
   | valid AST
   v
Displaying
   |
   +---- valid new AST ----> Displaying(new)
   |
   +---- anything else ----> Displaying(current)
```

A display should change only when a new valid AST can be successfully rendered.

The renderer obtains all referenced assets and completes rendering preparation before atomically replacing the visible scene. It retains the resources needed by the last successful scene independently of the server's current package.

Acquisition and rendering are serialized. A valid response from an earlier server state may temporarily be displayed; subsequent retrieval converges to the current target when communication and rendering succeed and that target remains stable.

Failures should not replace working content.

This gives the renderer a central invariant:

> **A failure must never replace successfully rendered content.**

Examples include:

- server unavailable
    
- network unavailable
    
- request failure
    
- invalid response
    
- unusable AST
    
- other transient failures
    

In these situations, the renderer normally continues displaying the last known good content.

---

## 13. No Error Screen

Glypha should not normally display technical error messages to viewers.

A signage viewer gains nothing from seeing:

```
SERVER CONNECTION ERROR
```

or:

```
HTTP 500
```

Such messages are operational information, not display content.

Errors should therefore not become signage content.

When possible:

> **Keep displaying the last known good scene.**

Operational diagnostics may exist outside the visible signage surface, but they should not interfere with the primary responsibility of the renderer.

---

## 14. Initial Display

Before the renderer has received its first valid AST, it displays a built-in minimal scene.

The initial scene is:

```
Glypha

"Stand out of my sun."
— Diogenes
```

This is not an error screen.

It is simply the initial valid display state of Glypha.

If the server is temporarily unavailable during startup, the renderer can remain in this state until valid content becomes available.

---

## 15. No Dedicated Client

Glypha has no dedicated client application.

There is:

- no administration application
    
- no content-management UI
    
- no Glypha CLI
    

The server exposes a small HTTP API.

Existing tools such as `curl` are sufficient for uploading content.

Conceptually:

```
curl -X PUT \
  -F 'content=@content.json' \
  -F 'assets=@background.jpg' \
  http://glypha.local/content
```

The API should return structured, machine-readable validation errors.

Humans and AI agents use the same interface.

---

## 16. AI as Tooling

Glypha does not provide elaborate tooling to compensate for its minimal interface.

Instead, its:

- source code
    
- content format
    
- HTTP API
    
- architecture
    
- invariants
    

should be understandable enough for an AI to work with directly.

An AI agent should be able to:

```
Generate content
      |
      v
Upload using HTTP
      |
      v
Validation result
      |
      +---- rejected ----> modify content ----+
      |                                      |
      +---- accepted                         |
                                             |
                     <-----------------------+
```

Installation, modification, extension, and operation may similarly be assisted by AI using the repository itself as context.

Documentation should prioritize **intent, structure, interfaces, and invariants** over exhaustive procedural instructions.

---

## 17. Open Source

Glypha will be open source.

Its source code is part of its documentation.

There should be no architectural mystery required to operate the system and no intentional vendor lock-in.

The system should remain small enough that another engineer—or an AI working with that engineer—can understand it.

---

## 18. Non-Goals

Glypha is not intended to become:

- a CMS
    
- a content creation application
    
- a workflow system
    
- a version-management system
    
- a general-purpose signage management platform
    
- a real-time information dashboard
    
- an advertising rotation engine
    
- a feature catalogue
    

New functionality should not be added simply because competing signage products have it.

Before adding something to Glypha, ask:

> **Does this need to be Glypha's responsibility?**

If the answer is no, it should remain outside the system.

---

## 19. Modeling Before Implementation

The scheduling and failure behavior should be modeled before implementation.

In particular, the model should not assume that time always advances normally.

It should explore cases such as:

```
normal progression
clock stopped
clock moved backward
clock jumped forward
request delayed past one or more transitions
schedule exhausted
server unavailable
invalid AST received
```

The purpose of the model is not to prove that the initial design is correct.

The model is allowed to show that the design is wrong.

Counterexamples should be treated as input to the design.

The implementation should follow only after the important invariants and transitions are understood.

---

## 20. The Question Behind Glypha

Glypha is also an experiment in system design.

Many systems compete by accumulating features.

At the same time, systems can pass through requirements definition, design, implementation, review, and testing—and still contain fundamental design defects.

Glypha starts from a different direction.

Reduce states.

Reduce responsibilities.

Reduce dependencies.

Define invariants.

Assume failures will occur.

Make recovery part of the design.

Then implement only what remains.

Glypha is one possible answer to the question:

> **What if the quality of a system were measured not by how much it can do, but by how clearly it knows what it must do?**