# Rule: Commit Style

Agents must use clear commit messages. More info here: https://raw.githubusercontent.com/conventional-commits/conventionalcommits.org/refs/heads/master/content/v1.0.0/index.md

## Format

<type>(scope): <description>

Examples:

feat(auth): add login endpoint
fix(cache): prevent stale entries
refactor(api): simplify service layer

## Checkpoint commits

Checkpoint commits must use:

checkpoint(agent): <session description>

When used for ephemeral worktree development, checkpoint commits are local safety commits and must not be pushed unless the user explicitly requests it.
