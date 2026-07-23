---
name: agent-platform-prd
description: Use when turning an agent platform capability idea into a PRD, acceptance criteria, test spec, and trace/evidence requirements.
---

# Agent Platform PRD

Use this skill when a feature request involves agent tasks, skill registry, workflow runtime, trace/audit, evaluation, permissions, or team-mode collaboration.

## Inputs

- User problem and target persona
- Current workflow or failure mode
- Proposed capability or rough idea
- Relevant directories, APIs, data models, or UI surfaces

## Output

Produce a concise PRD with:

- Problem statement and target user
- Narrow MVP scope and explicit non-goals
- User workflow and state transitions
- Data and integration contracts
- Trace, audit, and test evidence requirements
- Acceptance criteria
- Risks and open decisions
- OpenSpec owner recommendation and taskization requirements when durable delivery is needed

## Workflow

1. Clarify the job-to-be-done and the status quo the user is replacing.
2. Define the narrowest shippable wedge.
3. Specify the agent handoff, tool-call, trace, and approval surfaces.
4. Convert the workflow into acceptance criteria and regression tests.
5. Call out what should not be automated yet.
6. Classify the next step as exploratory MVP or formal delivery. Exploratory MVPs may run first when they are reversible and locally verifiable; formal delivery needs an OpenSpec change before hardening or release.

## Yeisme OpenSpec Taskization

When the PRD is based on a CEO, product, architect, or engineering方案 and is accepted for durable follow-up delivery, do not leave it as a standalone document. Route it to the correct OpenSpec owner before hardening, release, archive, parallel handoff, or multi-session execution. Bounded exploratory MVPs, prototypes, spikes, UI/UX extensions, local refactors, and focused bug investigations may run first if they are reversible, locally verified, and do not touch stable contracts, persistence schemas, cross-project boundaries, production behavior, credentials, or external side effects.

- Root `openspec/changes/<design-id>/` for repository-level PRD, architecture, governance, migration, and cross-project handoff.
- `<subproject>/openspec/changes/<change-id>/` for concrete implementation, tests, CLI/Web/TUI behavior, verification evidence, and closeout.

The OpenSpec package must include:

- `proposal.md`, `design.md`, `tasks.md`, and `specs/**/spec.md` in Chinese by default.
- A Mermaid diagram in `design.md` for architecture, flow, state, dependency, or data movement.
- Atomic tasks with owner, scope, dependencies, parallel lanes, acceptance criteria, validation commands, expected results, and failure re-checks.
- Explicit task requirements for Chinese comments in new or changed complex logic, state machines, concurrency, error handling, boundary checks, protocol conversions, and non-obvious test fixtures.

## Boundaries

Do not use this skill for generic product copy, one-off bug fixes, or implementation-only tasks that already have clear requirements.
