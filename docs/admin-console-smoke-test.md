# Admin Console — Smoke Test & Rollback Checklist

Phase 6 of the `titvo-admin-console` change (tasks 6.1–6.2). Requires a real deployed
stack — cannot be exercised from a local checkout. Run this after `titvo-installer`
finishes deploying `titvo-admin-bff-aws` and `titvo-admin-web`.

## 6.1 — Smoke test

1. **Locate the console URL.** Printed at the end of `titvo-installer` setup
   (CloudFront domain, added in Phase 5's `runStartConfiguration` summary). Open it
   in a browser.
2. **Login.** Use the admin email printed by `titvo-installer seed-admin` (Phase 2)
   and the password chosen during that step.
   - Expect: redirected to the dashboard, session cookie set (`httpOnly`, `Secure`,
     `SameSite`), no JWT visible in `localStorage`/`sessionStorage`/network response
     body.
3. **Add a secret.** Go to Configuration → New, enter a `parameter_id`, mark it as
   secret, submit.
   - Expect: 201, value never echoed back in the list view.
4. **Add a plaintext parameter.** Same flow, unmarked as secret.
   - Expect: 201, value visible only as metadata (name), not the raw value, in the
     list view (write-only UI regardless of type, per spec).
5. **Edit a value written by the old CLI wizard.** Pick a `parameter_id` that was
   originally created via `titvo-installer secret`/`parameter` (pre-existing data),
   update its value through the web form.
   - Expect: succeeds, and any existing consumer (e.g. rag-indexer's
     `EncryptionService`, titvo-auth's config reads) still decrypts/reads it
     correctly afterward — this is the byte-compatibility guarantee locked in by
     the Phase 0 shared AES test vectors.
6. **Attempt an overwrite without acknowledging it.** Submit an add with an existing
   `parameter_id`.
   - Expect: `409`, explicit "this key already exists" — not a silent clobber.
7. **Log in as (or demote a session to) a `member`.**
   - Expect: config list is visible (read-only), add/update controls are hidden or
     disabled — not just visually greyed out while still submittable. A direct
     `PUT`/`POST` to the BFF from a `member` session returns `403`.
8. **Logout.**
   - Expect: session cookie cleared, subsequent `GET /api/admin/*` calls return
     `401`, browser redirected to login.
9. **Session expiry mid-form.** Let a session expire (or manually invalidate it)
   while a form has unsaved input.
   - Expect: a warning before redirecting to login — input is not silently
     discarded (per spec; mechanism built in Phase 4 as `useSessionExpiryGuard`,
     verify it's actually wired into the live form).
10. **Existing CI path unaffected.** Confirm a GitHub Action / Bitbucket Pipeline
    run (using the pre-existing `X-API-Key` flow) still authenticates and runs a
    scan successfully — the admin console's login is additive, not a replacement.

## 6.2 — Rollback confirmation

Both new components are purely additive; verify rollback is clean and doesn't
touch anything else:

1. Remove the `titvo-admin-bff-aws` and `titvo-admin-web` entries from
   `titvo-installer`'s component list (`internal/deploy_runtime.go`,
   `internal/deploy.go`) and destroy their Terragrunt stacks
   (`terragrunt destroy` in each component's `aws/` directories).
2. Confirm the `account` API Gateway (shared, pre-existing, previously
   route-free) has no leftover `/api/admin/*` routes after the BFF's Terragrunt
   stack is destroyed.
3. Confirm `titvo-auth`'s login/session code and the `role`/`password_hash` fields
   on the `user` entity can be left in place dormant (additive, no other consumer
   depends on their absence) — no forced revert needed there.
4. Confirm the existing `X-API-Key` machine-auth path in `titvo-auth` is
   completely unaffected by either the deploy or the rollback (it was never
   modified, per every implementation batch's explicit constraint).
5. Confirm config data written through the admin console (the
   `tvo-security-scan-parameter-prod` table) remains intact and readable via the
   original `titvo-installer secret`/`parameter` CLI wizard after rollback — the
   CLI path was never removed, only supplemented.
6. Confirm the admin user seeded by `titvo-installer seed-admin` (Phase 2) can be
   deleted independently of the rollback, if desired — it's just a row in the
   `user` table, not coupled to the BFF/SPA's infrastructure lifecycle.

## 6.3 — Phase 2 + 3 smoke test (repo visibility, API keys, users)

`titvo-admin-console-phase2-4` change (9 work units, tasks 1.1–9.3). Adds
`admin-repo-visibility`, `admin-apikey-management`, and `admin-user-management`
on top of the Phase 0/1 console above. Same precondition as 6.1: a real
deployed stack, including the `task` table's `repository_id_index` GSI having
finished backfilling before repo/scan reads are exercised (see Known gaps).

### Repo & scan visibility

1. **View the repo list.** Log in as either `admin` or `member`, open Repos.
   - Expect: every connected repo is listed with its last-scan status; a repo
     with zero scans shows an explicit "never scanned" state, not blank or
     error.
2. **Scan status fidelity.** Find (or trigger via the existing scan path) a
   repo with an `IN_PROGRESS`, failed, or timed-out scan.
   - Expect: each renders as a visually distinct state — never collapsed into
     a generic "unknown".
3. **Orphan data resilience.** If reachable in the deployed data (or seeded for
   the test), a `task` row whose `repository_id` no longer resolves to any
   `repository` row.
   - Expect: the list renders an unresolved-repo indicator instead of a 500 or
     a crash.
4. **View scan detail.** Open a completed scan's detail view.
   - Expect: the full task record is shown.
5. **Empty state.** If no repos are connected at all (fresh environment).
   - Expect: an explicit "no repos" message, not a blank table.
6. **Read-only boundary — member.** As `member`.
   - Expect: no "Run scan" control is rendered anywhere on this screen (it is
     admin-only, per the trigger-a-scan feature below).

### Trigger a scan

Adds one write control to the otherwise read-only repo list: a "Run scan"
button, admin-only, shown for repos whose `provider` is `'github'` OR
`'bitbucket'` (any other/missing provider gets no button at all — no
provider beyond these two is supported yet). It reuses the exact same
`POST /run-scan` endpoint on `titvo-task-trigger-aws` that GitHub
Actions/Bitbucket Pipelines already call, so a successfully triggered scan
behaves identically to a CI-triggered one (async, `IN_PROGRESS` until the
AWS Batch job finishes). Provider dispatch is entirely server-side
(`GithubScanTriggerStrategy`/`BitbucketScanTriggerStrategy` in the BFF) —
the SPA dialog and its branch input are identical for both providers.

**One-time manual setup** (required before this feature can succeed — do
this once per environment, via the admin console's EXISTING screens, no new
UI was built for it):

1. **GitHub repos: set `default_github_assignee`.** Config → New →
   `parameter_id` = `default_github_assignee`, a plain (non-secret)
   parameter whose value is the GitHub username to assign issues to when
   scans surface findings. Not needed for Bitbucket — its `/run-scan`
   contract has no assignee field.
2. **GitHub repos: confirm `github_access_token` is set.** This is the SAME
   secret titvo-installer's setup flow already asks for
   (`github_access_token` — see `titvo-installer`'s README/`SetupConfigFile`);
   the trigger-scan feature reads it via the config screen's existing
   decrypt path, it does not introduce a new token.
3. **Bitbucket repos: confirm `bitbucket_api_token` is set.** Same idea —
   the SAME secret titvo-installer's setup flow already asks for
   (`bitbucket_api_token`). One extra Bitbucket API call
   (`GET /repositories/{workspace}/{repo_slug}`) resolves the repo's
   `project_key` automatically — no separate project-key parameter to set.
4. **Create a service API key.** API Keys → New, label it something
   identifiable like "BFF scan-trigger service key". Copy the raw key value
   shown once in the reveal modal — this is standard API-key creation, no
   new mechanism. Shared by both providers.
5. **Store that key as a config secret.** Config → New → `parameter_id` =
   `bff_scan_trigger_api_key`, mark it as secret, paste the raw key value
   from step 4 as `value`. The BFF's trigger-scan handler reads this secret
   (same decrypt path as the provider tokens above) and sends it as the
   `X-API-Key` header when it calls `titvo-task-trigger-aws`.
6. **IMPORTANT — who creates the service key in step 4 matters.**
   titvo-task-trigger-aws computes a scan's `repositoryId` as
   `${apiKey.userId}:${md5(repository_slug)}` (see
   `trigger/src/app/task/task.service.ts`) — keyed by WHICH admin user's
   key authenticated the `/run-scan` call, not just the repo. If the admin
   who creates the "BFF scan-trigger service key" in step 4 is a DIFFERENT
   user than whoever's key CI has been using for a given repo, scans
   triggered from the admin console for that repo will get a different
   `repositoryId` than that repo's existing scan history and won't show up
   as its "last scan" — the BFF detects this automatically and returns a
   `warning` field (surfaced as a toast in the SPA) rather than failing
   silently, but it cannot fix the mismatch itself (that would need a
   design change in titvo-task-trigger-aws, out of scope here). Simplest
   avoidance: have the SAME admin who originally set up CI's key also
   create the BFF service key.

**Smoke test steps:**

1. **Config secret missing (closest-to-real check without a live
   task-trigger-aws call) — GitHub.** Before completing the one-time setup
   above (or with one of the GitHub parameters deliberately unset), as
   `admin`, open a GitHub repo's "Run scan" dialog, enter a branch, submit.
   - Expect: `422 config_missing`, a clear message naming the EXACT missing
     parameter (e.g. `default_github_assignee`) — not a generic 500 or a
     silent failure.
2. **Config secret missing — Bitbucket.** Same as step 1 but on a Bitbucket
   repo, with `bitbucket_api_token` unset.
   - Expect: `422 config_missing` naming `bitbucket_api_token` specifically.
3. **Full happy path — GitHub (after completing the one-time setup).** As
   `admin`, open "Run scan" on a GitHub repo, the branch field is pre-filled
   `main` (editable), submit.
   - Expect: success toast, redirected to `/scans/:scanId`, the new scan
     renders `IN_PROGRESS` (same rendering the existing scan-detail page
     already uses for CI-triggered scans).
4. **Full happy path — Bitbucket.** Same as step 3 but on a Bitbucket repo.
   - Expect: identical behavior — success toast, redirected to
     `/scans/:scanId`, `IN_PROGRESS`.
5. **Unsupported provider.** As `admin`, look at a repo whose `provider` is
   neither `github` nor `bitbucket` (e.g. `gitlab`, or missing).
   - Expect: no "Run scan" control at all for that row.
6. **Member session.** As `member`, look at any repo's row (any provider).
   - Expect: no "Run scan" control anywhere; a direct
     `POST /api/admin/repos/:id/trigger-scan` from a `member` session
     returns `403`.
7. **Invalid branch.** As `admin`, submit a branch name that does not exist,
   for both a GitHub and a Bitbucket repo.
   - Expect: `422 branch_resolution_failed`, a clear message — not a raw
     GitHub/Bitbucket API error body.
8. **Full scan toggle.** As `admin`, check "Full scan" in the dialog before
   submitting.
   - Expect: the triggered scan's `args.scan_mode` is `"full"` (visible on
     the scan detail page's `args` panel); unchecked (default) omits it.
9. **Auto-detected branch.** As `admin`, open the dialog for a repo whose
   actual default branch is NOT `main` (e.g. `master` or `develop`).
   - Expect: the branch field replaces `"main"` with the real default
     shortly after the dialog opens (a "Detecting default branch…" hint
     shows briefly); typing before it resolves is never overwritten.
10. **repositoryId mismatch warning.** As `admin`, with the service key
    (step 6 above) belonging to a DIFFERENT user than the one whose key
    produced a repo's existing scan history, trigger a scan for that repo.
    - Expect: the scan still starts successfully (success toast, redirect
      to `/scans/:scanId`) AND a separate warning toast explains it won't
      be linked to that repo's existing history. Confirm this warning does
      NOT appear when the service key's owner matches.

### API key management

1. **Create a key.** Log in as `admin`, go to Keys → New, enter a label,
   submit.
   - Expect: the raw key (`tvok-` + 43 alphanumeric chars, 48 total) is shown
     exactly once in a modal; the modal has no close icon, no backdrop-click
     dismiss, and no Escape-key dismiss — only an explicit "I've copied this
     key" acknowledgment unlocks the Done button.
2. **Raw value never resurfaces.** After dismissing the modal, reload the key
   list (or refetch).
   - Expect: only metadata (label, status, timestamps) is ever shown again —
     the raw value is not present in the page, in any API response body, or
     in browser storage.
3. **Console-issued key authenticates.** Use the key just created against the
   existing machine-auth (`X-API-Key`) path (e.g. trigger a scan via CI or a
   direct API call).
   - Expect: it authenticates identically to an installer-issued key — no new
     format was introduced.
4. **List as `member`.** Log in as `member`, open Keys.
   - Expect: the same metadata-only list is visible, but no "Create key" link
     and no "Revoke" control are rendered anywhere in the DOM.
5. **Revoke a key.** As `admin`, revoke an active key that is not the
   platform's last active key.
   - Expect: a two-step confirm (arm → confirm) before the revoke call fires;
     after revoking, the next attempt to authenticate with that key's raw
     value fails immediately.
6. **Last-active-key guard.** As `admin`, attempt to revoke the platform's
   only remaining active key.
   - Expect: `409`, a clear "revoking it would lock the platform out" banner —
     not a silent failure and not a silent success.
7. **Member attempts a write.** As `member`, attempt to hit the create/revoke
   endpoints directly (not just via hidden UI controls).
   - Expect: `403` with a clear message.

### User management

1. **List users.** Log in as either `admin` or `member`, open Users.
   - Expect: with only the seed admin present, just that one row is shown —
     not an error or a blank screen.
2. **Create a user (admin-set-password invite).** As `admin`, go to Users →
   Create user, enter an email, an initial password directly in the form
   (there is no email/SMTP step — decision #3), pick a role, submit.
   - Expect: the user is created and can immediately log in with that exact
     password; no reveal-once modal is shown here (unlike API keys) because
     the admin already knows the value they just typed.
3. **Duplicate email.** Attempt to create a second user with an email that
   already exists.
   - Expect: `409`, a clear "already exists" message — not a silent
     overwrite or a generic error.
4. **Deactivate a non-last-admin user.** As `admin`, deactivate an active
   `member` or a non-last `admin`.
   - Expect: two-step confirm before the call fires; the deactivated user's
     next login attempt fails immediately.
5. **Last-admin block — deactivate.** As `admin`, with exactly one active
   admin remaining, attempt to deactivate that admin.
   - Expect: `409 last_admin`, surfaced as a clear banner — the same
     convention as the API key screen's `409 last_active_key`, not a generic
     error.
6. **Last-admin block — de-admin.** As `admin`, with exactly one active admin
   remaining, attempt to change that admin's role to `member`.
   - Expect: also blocked with the same `409 last_admin` banner.
7. **Member attempts a write.** As `member`, attempt to hit the
   create/deactivate/role-change endpoints directly.
   - Expect: `403` with a clear message; the member's own list view shows no
     create/deactivate/role controls at all.
8. **Nav and dashboard parity.** As an authenticated session (either role).
   - Expect: a "Users" link in the persistent header nav (next to Config,
     Repos, Keys) and a "User Management" tile on the dashboard, both routing
     to the same user list screen.

## Known gaps to account for during this smoke test

(carried over from implementation batches — not blockers, but expect these)

- No `terraform validate`/`terraform plan` was run against the new
  `modules/s3-cloudfront` Terraform module or the BFF's Terragrunt stack in any
  implementation batch (no `terraform`/AWS credentials available in that
  environment) — this smoke test is the first real validation of that
  infrastructure.
- SSM parameter names for `titvo-admin-web`'s S3 bucket/CloudFront
  distribution/domain were inferred by naming convention during Phase 5, not
  independently confirmed against the actual Terraform outputs.
- `useSessionExpiryGuard` (Phase 4) was built and unit-tested in isolation but not
  confirmed wired into the live `ConfigFormPage` — step 9 above is the first real
  check of that integration.
- The `task` table's `repository_id_index` GSI (Phase 2+3 change, Work Unit 1)
  was applied via `terragrunt hcl format --check` plus manual diff review only
  — no real `terragrunt plan`/`apply` or online-backfill timing was observed
  in any implementation environment (no AWS credentials available). Confirm
  the GSI has finished backfilling before exercising 6.3's repo/scan steps;
  the BFF is written to tolerate an index-not-ready error, but this smoke
  test is the first real check of that tolerance against a genuinely
  backfilling index.
- Every Phase 2+3 BFF and SPA batch (Work Units 4–9) was implemented and
  verified with unit/integration tests only (`jest`, `vitest`+RTL) — no
  Docker/LocalStack or live BFF instance was reachable in any implementation
  environment, so no batch had a real end-to-end HTTP round-trip against a
  running server. This smoke test is the first real integration check for
  all of repo/scan visibility, API key management, and user management.
- The BFF's user-update endpoint (Work Unit 6) is a single combined
  `PATCH /api/admin/users/:id` handling both role reassignment and
  deactivation/reactivation, a deliberate deviation from the original
  per-action-route design — confirm both the role-change and
  deactivate/reactivate smoke-test steps above (6.3 steps 4–6) exercise this
  one endpoint correctly, not two separate ones.
- **Trigger-a-scan feature**: `TASK_TRIGGER_API_URL` (the BFF's new env var,
  wired in `titvo-admin-bff-aws/aws/lambda/terragrunt.hcl`) reads
  `infra/apigateway/task/api_gateway_api_full_endpoint` from the shared
  `/tvo/security-scan/<stage>` SSM namespace — a value titvo-task-trigger-aws
  already publishes for its own use, confirmed by reading that repo's
  `aws/apigateway/terragrunt.hcl` and titvo-security-scan-infra-aws's SSM
  upsert stack, but never independently verified against a real `terraform
  apply` in any implementation session (no AWS credentials available).
  Confirm this resolves to the correct URL on first deploy.
- Neither `titvo-admin-bff-aws` nor `titvo-task-trigger-aws` is deployed
  inside a VPC (no `vpc_config`/subnet/security-group in either's
  Terragrunt lambda stack) — this was verified by reading both stacks
  directly, so no NAT/security-group changes were made for this feature's
  outbound calls to GitHub's API and to titvo-task-trigger-aws. If either
  Lambda is ever moved into a VPC for an unrelated reason, that would need
  NAT egress added for this feature to keep working.
- The full trigger-scan happy path (config secrets present, real
  GitHub/Bitbucket branch resolution, real `/run-scan` call) was never
  exercised end-to-end in any implementation session — LocalStack does not
  mock GitHub's/Bitbucket's APIs or a real task-trigger-aws deployment. The
  "config secret missing" error path WAS mechanically verified against real
  LocalStack DynamoDB for both providers (a seeded GitHub repo and a
  manually-inserted Bitbucket repo each correctly returned `422
  config_missing` naming their own first missing parameter,
  `github_access_token`/`default_github_assignee` then
  `bitbucket_api_token` respectively) — steps 3–4 above (full happy path)
  are the first real check of the rest of this feature.
- Bitbucket parity (`BitbucketScanTriggerStrategy`,
  `AxiosBitbucketApiClient`) mirrors titvo-git-commit-files-aws's live
  `BitbucketClientService` for the auth header (`Authorization: Basic
  <bitbucket_api_token>` — NOT `Bearer`, unlike GitHub) and the branch→hash
  endpoint (`GET /repositories/{workspace}/{repo_slug}/refs/branches/{branch}`
  → `target.hash`), both confirmed against that repo's actual working code.
  The project-key lookup (`GET /repositories/{workspace}/{repo_slug}` →
  `project.key`) is new — Bitbucket's Cloud REST API v2.0 documents this
  field, but no other repo in this codebase reads it today, so it was never
  cross-checked against a second live example the way the other two calls
  were.
- **repositoryId ownership mismatch — detected, not eliminated.**
  titvo-task-trigger-aws keys `repositoryId` off the CALLING API key's
  owner, not just the repo (see setup step 6 above). `TriggerScanUseCase`
  now reproduces that exact formula
  (`computeExpectedRepositoryId`/`deriveRepositorySlug` in
  `src/app/scan/compute-expected-repository-id.ts`, verified byte-for-byte
  against `task.service.ts` in a unit test) and compares it to the target
  repo's existing `repositoryId` before calling `/run-scan`, returning a
  non-blocking `warning` when they differ. This turns a SILENT failure mode
  into a LOUD one; it does not and cannot eliminate the mismatch itself
  without a design change in titvo-task-trigger-aws (out of scope — a
  different repo, and changing its `repositoryId` formula would affect
  every existing CI-triggered scan's correlation too, not just this admin
  console). Required a new IAM grant: `api_key_gsi` index ARN added to the
  BFF's existing `apikey`-table policy statement in `aws/lambda/terragrunt.hcl`
  (the table/`user_id_gsi` grant already existed for the API-key management
  screens; `api_key_gsi` is the first real caller of `findByApiKey` from
  this BFF and was previously ungranted — would have silently AccessDenied'd
  in real AWS had this not been added). Verified: the DI wiring resolves
  cleanly against a live LocalStack `apikey` table with both GSIs present
  (`TriggerScanModule dependencies initialized` with no errors); the actual
  mismatch-detection logic itself is unit-tested but was NOT exercised
  end-to-end against a real deployed stack (would need a second real admin
  user + a real task-trigger-aws deployment to observe both the match and
  mismatch cases live) — smoke-test step 10 above is the first real check.
