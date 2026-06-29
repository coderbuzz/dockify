# TODO

## Webhook Secret

- [ ] **Settings page**: Add toggle to disable/clear the webhook secret.
  - Currently the secret is auto-generated at startup (`internal/settings/settings.go:31` `ensureWebhookSecret()`) and cannot be disabled — only show/hide, copy, roll.
  - Affected: `internal/http/templates/settings.html`, `internal/settings/settings.go`, `internal/settings/handler.go`
  - After adding disable, update README Option A note to mention this feature.

- [ ] **Settings template**: Replace hardcoded `dockify.amg.id` with a generic example domain in the GitHub Actions workflow snippet.
  - File: `internal/http/templates/settings.html:33`

## Backup

- [ ] **Fix misleading YAML header comment**: `internal/backup/backup.go:95` says "Secrets (SSH keys, auth passwords if present) are included as saved" but SSH keys are **not** included in the export (`ExportServer` struct has no SSH key field). Either fix the comment or add SSH key export support.
  - File: `internal/backup/backup.go:94-96`
