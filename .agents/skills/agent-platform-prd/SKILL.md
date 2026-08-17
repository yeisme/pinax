---
name: agent-platform-prd
description: Use when turning an agent platform capability idea into a PRD with an early owner-fit decision, required-capability ledger, acceptance criteria, test spec, and trace/evidence requirements.
---

# Agent Platform PRD

Use this skill when a feature request involves agent tasks, skill registry, workflow runtime, trace/audit, evaluation, permissions, or team-mode collaboration.

## Inputs

- User problem and target persona
- Current workflow or failure mode
- Proposed capability or rough idea
- User-required capabilities that later reviews must preserve
- `fit`, `split-owner`, or `reject-now` admission decision from `yeisme-repo-routing`
- Relevant directories, APIs, data models, or UI surfaces

## Output

Produce a concise PRD with:

- Problem statement and target user
- Required Capability Ledger with delivery status and canonical owner
- Owner/consumer boundary decision and experience-composition model
- Narrow first delivery slice, retained later capabilities, and explicit non-goals
- User workflow and state transitions
- Data and integration contracts
- Trace, audit, and test evidence requirements
- Acceptance criteria
- Risks and open decisions
- OpenSpec owner recommendation and taskization requirements when durable delivery is needed

## Workflow

1. Clarify the job-to-be-done and the status quo the user is replacing.
2. Before narrowing scope, run or consume `yeisme-repo-routing` and record `fit`, `split-owner`, or `reject-now`. Raise an ownership mismatch immediately, not after the PRD or spec is finished.
3. Build the Required Capability Ledger. Mark each item as `required`, `committed`, `exploratory`, `optional`, or `rejected-with-user-decision`; record canonical owner, visible host, delivery slice, and acceptance evidence.
4. Define the narrowest shippable delivery slice without silently converting required capabilities into non-goals. Distinguish `deliver-now`, `retain-next`, and `not-requested`.
5. Specify the agent handoff, tool-call, trace, approval, owner receipt, and experience-composition surfaces.
6. Convert the workflow into acceptance criteria and regression tests.
7. Call out what should not be automated yet without removing the corresponding required user job.
8. Classify the next step as exploratory MVP or formal delivery. Exploratory MVPs may run first when they are reversible and locally verifiable; formal delivery needs an OpenSpec change before hardening or release.

## Yeisme OpenSpec Taskization

When the PRD is based on a CEO, product, architect, or engineering方案 and is accepted for durable follow-up delivery, do not leave it as a standalone document. Route it to the correct OpenSpec owner before hardening, release, archive, parallel handoff, or multi-session execution. Bounded exploratory MVPs, prototypes, spikes, UI/UX extensions, local refactors, and focused bug investigations may run first if they are reversible, locally verified, and do not touch stable contracts, persistence schemas, cross-project boundaries, production behavior, credentials, or external side effects.

- Root `openspec/changes/<design-id>/` for repository-level PRD, architecture, governance, migration, and cross-project handoff.
- `<subproject>/openspec/changes/<change-id>/` for concrete implementation, tests, CLI/Web/TUI behavior, verification evidence, and closeout.

The OpenSpec package must include:

- `proposal.md`, `design.md`, `tasks.md`, and `specs/**/spec.md` in Chinese by default.
- A Mermaid diagram in `design.md` for architecture, flow, state, dependency, or data movement.
- Atomic tasks with owner, scope, dependencies, parallel lanes, acceptance criteria, validation commands, expected results, and failure re-checks.
- A Required Capability Ledger, boundary decisions, and scope-change log that make retained, staged, moved, rejected, and user-approved removals explicit.
- Explicit task requirements for Chinese comments in new or changed complex logic, state machines, concurrency, error handling, boundary checks, protocol conversions, and non-obvious test fixtures.

## Boundaries

Do not use this skill for generic product copy, one-off bug fixes, or implementation-only tasks that already have clear requirements.

Do not let “narrow MVP” or CEO review erase a capability the user explicitly requires. A review may stage delivery, consolidate entry points, remove duplicate infrastructure, or move canonical ownership, but permanent removal requires a separate user decision. When a capability does not belong in the proposed owner, state that immediately and preserve it through the correct owner/consumer split where feasible.
