---
name: ephemeral-worktree
description: safely implement code changes using a temporary git worktree to isolate agent modifications from the developer workspace
---

# ephemeral-worktree

Use an ephemeral git worktree to implement changes safely without modifying the developer's main workspace.

This skill provides a safe development workflow for AI agents when implementing non-trivial code changes.

## When to use

Use this skill when:

* implementing features
* performing refactors
* fixing bugs affecting multiple files
* making experimental or risky changes
* modifying multiple parts of the repository

Do **not** use this skill for trivial edits such as:

* documentation fixes
* formatting changes
* single-line modifications

### Practical threshold

Treat a task as non-trivial when one or more of these apply:

* changes touch multiple files or modules
* behavior changes require test validation
* refactors alter structure or call paths
* rollback risk is meaningful if a change fails

For clearly trivial edits (single location, docs-only, formatting-only), a worktree is optional.

## Instructions

1. Create a temporary worktree session.

2. Perform all development work inside the worktree.

3. Run project tests to ensure the repository remains valid.

4. Create checkpoint commits while developing.

See:

rules/checkpoint.md

5. Present the resulting changes for developer review.

6. After review, the developer may merge or discard the worktree.

## Rule precedence and commit policy

For this repository, non-trivial worktree usage is mandatory even if the user does not explicitly request it.

Checkpoint commits are required inside the worktree as local safety snapshots.

Do not push checkpoint commits unless the user explicitly requests pushing.

## Dirty workspace handling

If the main workspace already has local changes:

1. Create the worktree from `HEAD`.
2. Do not modify, stage, or revert the developer's local changes.
3. Keep all agent edits inside the worktree.

## Additional References

Detailed implementation steps, scripts, and examples are available in:

references/worktree-workflow.md
