---
name: using-superpowers
description: Use at session start or when skill routing is unclear; perform lightweight skill discovery before task work, then invoke only explicitly requested or clearly applicable skills.
---

<SUBAGENT-STOP>
If you were dispatched as a subagent to execute a specific task, ignore this skill.
</SUBAGENT-STOP>

<IMPORTANT>
Use this skill as the lightweight routing pass for the session. It should prevent missed domain skills, not turn every small request into a long workflow.

Invoke skills when the user names them, when repository instructions require them, or when the task clearly matches a skill's description. Do not invoke a process skill only because it is remotely related.
</IMPORTANT>

## The Rule

**Check for relevant or requested skills before task work.** Invoke the skill before implementation, repo exploration, or tool execution when the match is explicit or clear. A short user-facing acknowledgement or one necessary routing clarification may happen without loading a long skill chain.

**Before entering plan mode:** if you haven't already brainstormed, invoke the brainstorming skill first.

Then announce "Using [skill] to [purpose]" and follow the skill. If it has a checklist and the current task is multi-step, track the checklist; do not create heavy process artifacts for a one-command or one-answer task.

## Skill Priority

When multiple skills apply, choose the narrowest useful set. Process skills come first only when they materially change how the work should be done; domain owner skills take precedence for explicit tool or subproject requests.

- "Let's build X" → superpowers:brainstorming first, then implementation skills.
- "Fix this bug" → superpowers:systematic-debugging first, then domain skills.
- "Use Eikona to generate an image" → Eikona routing/runtime skill first; do not replace it with a generic image-generation skill.

## Red Flags

These thoughts mean STOP—you're rationalizing:

| Thought | Reality |
|---------|---------|
| "The user explicitly named a tool/subproject, but a generic skill is nearby" | Use the named tool/subproject route first. |
| "Let me explore the codebase first" | Do the lightweight skill check first; then explore only as needed. |
| "This doesn't need a formal skill" | If a skill is explicitly requested or clearly applies, use it. |
| "I remember this skill" | Skills evolve. Read current version. |
| "This feels productive" | Undisciplined action wastes time; over-triggered process also wastes time. Route narrowly. |
| "I know what that means" | Knowing the concept ≠ using the skill. Invoke it. |

## Platform Adaptation

If your harness appears here, read its reference file for special instructions:

- Codex: `references/codex-tools.md`
- Pi: `references/pi-tools.md`
- Antigravity: `references/antigravity-tools.md`

## User Instructions

User instructions (CLAUDE.md, AGENTS.md, GEMINI.md, etc, direct requests) take precedence over skills, which in turn override default behavior. If the user names a concrete tool such as `eikona`, route there even when a generic built-in capability could also perform a similar action.
