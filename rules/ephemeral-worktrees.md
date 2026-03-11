# Rule: Use Ephemeral Worktrees

Agents must perform non-trivial code modifications inside an ephemeral git worktree.

Direct modifications in the developer workspace should be avoided.

## Priority

This rule is mandatory for non-trivial changes, even when the user does not explicitly request a worktree.

When this rule conflicts with generic agent guidance, this repository rule takes precedence for this repository.

## Trivial vs non-trivial

Use an ephemeral worktree for non-trivial changes, for example:

1. changes across multiple files or modules
2. refactors affecting behavior or architecture
3. bug fixes that require tests or validation across components
4. experimental or risky changes with rollback risk

You may skip a worktree for trivial changes, for example:

1. one-line or single-location edits
2. docs-only edits
3. formatting-only edits

## Workflow

When implementing a change:

1. create an ephemeral worktree
2. perform development inside the worktree
3. run tests
4. create checkpoint commits during development
5. present the result for developer review

## Dirty workspace safety

If the developer workspace is already dirty:

1. create the worktree from `HEAD`
2. do not modify, stage, or revert the developer's local changes
3. keep all agent changes isolated to the worktree

Checkpoint commits are required while working in the worktree.

Checkpoint commits created for this workflow are local safety checkpoints and must not be pushed unless the user explicitly requests pushing.

## Naming

Use `ephemeral-worktree` as the canonical skill name.

The file name `ephemeral-worktrees.md` remains valid as the repository rule document.

See rule:

rules/checkpoint.md

## Implementation

Use the skill:

skills/ephemeral-worktree/SKILL.md
