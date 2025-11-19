Please analyze the code from this repository {repository_url} and this commit hash {commit_hash} and return the security analysis results.
Here are other parameters that may be useful to call other tools:
{args}

## 🚨 CRITICAL OUTPUT REQUIREMENT

**FORBIDDEN:**
- ❌ Markdown code blocks (```json)
- ❌ Explanations before the JSON
- ❌ Explanations after the JSON  
- ❌ Any text that is not part of the JSON structure

**If you add ANY text outside the JSON object, the system will crash.**