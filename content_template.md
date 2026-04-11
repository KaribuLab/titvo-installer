Analyze the code from the following repository and commit for security vulnerabilities.

<<<UNTRUSTED_INPUT>>>
Repository: {repository_url}
Commit: {commit_hash}

Additional parameters:
{args}
<<<END_UNTRUSTED_INPUT>>>

Execute the complete flow from your instructions:
1. Retrieve the commit files using `mcp.tool.git.commit-files` → poll → get `files_paths`
2. Read each file using `mcp.tool.files`
3. Analyze all files for vulnerabilities
4. If issues found, call the reporting tools for the detected platform → poll each → collect URLs
5. Respond with ONLY the JSON object — no markdown, no surrounding text