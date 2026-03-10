# Rule: Use Ephemeral Worktrees

Agents must perform non-trivial code modifications inside an ephemeral git worktree.

Direct modifications in the developer workspace should be avoided.

## Workflow

When implementing a change:

1. create an ephemeral worktree
2. perform development inside the worktree
3. run tests
4. create checkpoint commits during development
5. present the result for developer review

Checkpoint commits are required while working in the worktree.

See rule:

rules/checkpoint.md

## Implementation

Use the skill:

skills/ephemeral-worktree/SKILL.md
