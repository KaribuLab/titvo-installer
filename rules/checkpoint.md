# Rule: Create Checkpoints During Development

Agents must create checkpoint commits while modifying the repository.

For non-trivial changes, this applies while working in an ephemeral worktree.

These checkpoint commits are authorized by repository policy, even if the user did not explicitly request commits.

Checkpoint commits ensure progress can be safely reverted if later changes break the code.

## Context

Checkpoint commits are typically created while working inside an ephemeral worktree.

Checkpoint commits are safety snapshots for iterative development. They are local unless the user explicitly requests a push.

See:

rules/ephemeral-worktrees.md

## Workflow

After completing a logical change:

1. run project tests
2. stage changes
3. create checkpoint commit

Example:

git add -A
git commit -m "checkpoint(agent): <description>"

## Repository must remain valid

Agents must not continue development if the repository is failing.
