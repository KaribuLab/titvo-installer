Analyze the code from the following repository and commit for security vulnerabilities.

<<<UNTRUSTED_INPUT>>>
Repository: {repository_url}
Commit: {commit_hash}

Additional parameters:
{args}
<<<END_UNTRUSTED_INPUT>>>

---

## STRICT EXECUTION REQUIRED

- You MUST execute ALL required tool calls
- You MUST NOT skip steps
- You MUST NOT fabricate tool outputs
- You MUST follow the exact execution flow

If any step is skipped → RESULT IS INVALID

---

## Required Execution Flow

1. Retrieve commit files using:
   `mcp.tool.git.commit-files`
   → poll until SUCCESS
   → extract `files_paths`

2. Read ALL files:
   - Call `mcp.tool.files` for EACH path
   - Collect all contents

3. Analyze ALL files:
   - Identify real vulnerabilities only
   - Classify severity

4. If issues are found:
   - Call `mcp.tool.issue.report`
   - Poll until SUCCESS
   - Extract `reportURL`

5. Return ONLY the final JSON

---

## Critical Rules

- You CANNOT skip tool execution
- You CANNOT fabricate reportURL
- You CANNOT finish without polling tools when required
- You MUST analyze ALL files before deciding

---

## Output

Return ONLY a valid JSON object.
No explanations. No markdown. No extra text.