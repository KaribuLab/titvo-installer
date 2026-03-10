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

## Instructions

1. Create a temporary worktree session.

2. Perform all development work inside the worktree.

3. Run project tests to ensure the repository remains valid.

4. Create checkpoint commits while developing.

See:

rules/checkpoint.md

5. Present the resulting changes for developer review.

6. After review, the developer may merge or discard the worktree.

## Additional References

Detailed implementation steps, scripts, and examples are available in:

references/worktree-workflow.md
