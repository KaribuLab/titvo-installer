# Worktree Development Workflow

This document describes how to implement code changes using a temporary git worktree.

Agents should consult this reference when implementing the **ephemeral-worktree skill**.

---

# Goal

Perform development in an isolated workspace that can be safely reviewed, merged, or discarded without affecting the developer's main working directory.

---

# Step 1: Create a session name

Generate a unique name for the worktree session.

Example:

SESSION=ai-session-$(date +%s)

The session name should:

* be unique
* identify that it belongs to an AI agent
* avoid collisions with existing branches

---

# Step 2: Create the worktree

Create a new branch and worktree.

Example:

git worktree add ../$SESSION -b $SESSION

This creates:

* a new branch
* a new working directory outside the repository

Example result:

repo/
../ai-session-1710000000/

---

# Step 3: Enter the worktree

Move into the new workspace.

Example:

cd ../$SESSION

All development work must occur inside this directory.

---

# Step 4: Implement the change

Modify the repository according to the user request.

Guidelines:

* prefer minimal diffs
* avoid modifying unrelated files
* keep commits focused

---

# Step 5: Run project tests

Ensure the project builds and tests pass before creating checkpoints.

Examples:

Node:

npm test

Python:

pytest

If tests fail:

* fix the issue
* rerun tests
* do not continue until tests pass

---

# Step 6: Create checkpoint commits

While implementing the change, create checkpoint commits after completing logical changes.

Example:

git add -A
git commit -m "checkpoint(agent): <description>"

Checkpoint commits help developers review progress.

---

# Step 7: Show the changes

Before finishing the task, show the diff relative to the base branch.

Examples:

git diff main

or

git log

This allows developers to review the changes.

---

# Step 8: Developer decision

Developers may choose to:

* merge the branch
* squash commits
* discard the worktree

Checkpoint commits are often squashed before merging.

---

# Step 9: Cleanup

If the worktree is not needed, remove it.

Example:

git worktree remove ../$SESSION
git branch -D $SESSION

This removes the temporary workspace safely.

---

# Summary

Typical development session:

1. create ephemeral worktree
2. enter worktree
3. implement change
4. run tests
5. create checkpoints
6. present diff
7. cleanup if needed
