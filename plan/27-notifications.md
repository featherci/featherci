# Step 27: Build Notifications

## Context
Steps 1-20 built the full CI pipeline with secrets. Users need to be notified when builds succeed or fail. This step adds a pluggable notification system with email (SMTP/Sendgrid/Mailgun), Slack, Discord, and Pushover integrations.

## Pre-existing Infrastructure
- **DB table**: Needs new `notification_channels` table (migration)
- **Config**: Will need new env vars for global defaults
- **Worker**: `advanceBuild` in `worker.go` already detects terminal build states — ideal hook point
- **Models**: `Build` has `Status`, `Project` has owner info

## Design

### Architecture
```
Build completes → Worker detects terminal state → NotificationService.Notify(build)
                                                        ↓
                                              Load project's channels
                                                        ↓
                                              For each channel: dispatch
                                                   ↓         ↓        ↓        ↓
                                                 Email     Slack   Discord  Pushover
```

Notifications are **fire-and-forget** from the worker's perspective — dispatched in a goroutine so they never block build processing. Failures are logged but don't affect build status.

### Notification Triggers
- **On failure**: Always notify (default)
- **On success**: Notify (configurable per channel)
- **On recovery**: Build succeeds after a previous failure (nice-to-have, derive from build history)

### Channel Types
| Type | Transport | Config Fields |
|------|-----------|---------------|
| `email_smtp` | SMTP | host, port, username, password, from, to (comma-sep) |
| `email_sendgrid` | Sendgrid API | api_key, from, to |
| `email_mailgun` | Mailgun API | api_key, domain, from, to |
| `slack` | Slack Incoming Webhook | webhook_url, channel (optional) |
| `discord` | Discord Webhook | webhook_url |
| `pushover` | Pushover API | app_token, user_key |

## Database

### Migration: `notification_channels` table
```sql
CREATE TABLE notification_channels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,         -- email_smtp, email_sendgrid, email_mailgun, slack, discord, pushover
    config_encrypted BLOB NOT NULL,  -- JSON config, encrypted with SecretKey
    on_success BOOLEAN NOT NULL DEFAULT 0,
    on_failure BOOLEAN NOT NULL DEFAULT 1,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    created_by INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id)
);
CREATE INDEX idx_notification_channels_project_id ON notification_channels(project_id);
```

Config is stored as encrypted JSON (reuse `crypto.Encryptor`) since it contains API keys/passwords.

## Implementation

### 1. Database migration (`internal/database/migration_005_notifications.go`)
- Add `notification_channels` table

### 2. Notification model (`internal/models/notification_channel.go`)
- `NotificationChannel` struct: ID, ProjectID, Name, Type, ConfigEncrypted, OnSuccess, OnFailure, Enabled, CreatedBy, CreatedAt, UpdatedAt
- `NotificationChannelRepository` interface: Create, GetByID, ListByProject, Update, Delete
- `SQLiteNotificationChannelRepository`

### 3. Notifier interface + implementations (`internal/notify/`)
- `internal/notify/notifier.go` — `Notifier` interface: `Send(ctx, BuildEvent) error`
- `internal/notify/event.go` — `BuildEvent` struct: project name, build number, status, branch, commit, URL, duration
- `internal/notify/email_smtp.go` — SMTP sender using `net/smtp` with HTML template
- `internal/notify/email_sendgrid.go` — Sendgrid v3 API (HTTP, no SDK dependency)
- `internal/notify/email_mailgun.go` — Mailgun API (HTTP, no SDK dependency)
- `internal/notify/slack.go` — Slack incoming webhook with rich message blocks
- `internal/notify/discord.go` — Discord webhook with embed
- `internal/notify/pushover.go` — Pushover API notification
- `internal/notify/email_template.go` — Shared HTML email template (clean, branded)

### 4. Notification service (`internal/services/notifications.go`)
- `NotificationService` with `NotificationChannelRepository` + `*crypto.Encryptor` + `baseURL string`
- `CreateChannel(ctx, projectID, name, channelType, configJSON, onSuccess, onFailure, userID)` — validate type, encrypt config, store
- `UpdateChannel(ctx, id, configJSON, onSuccess, onFailure)` — re-encrypt, update
- `DeleteChannel(ctx, id)`
- `ListChannels(ctx, projectID)` — metadata only
- `TestChannel(ctx, id)` — decrypt config, send test notification
- `NotifyBuild(ctx, build, project)` — load enabled channels, decrypt configs, dispatch each in goroutine

### 5. Notification handler (`internal/handlers/notification.go`)
- `NotificationHandler` with NotificationService, projects, projectUsers, templates, logger
- `List` — GET `/projects/{ns}/{name}/notifications` — render list page
- `New` — GET `/projects/{ns}/{name}/notifications/new` — render add form (type selector, dynamic config fields)
- `Create` — POST `/projects/{ns}/{name}/notifications` — form submit, redirect
- `Edit` — GET `/projects/{ns}/{name}/notifications/{id}/edit` — render edit form
- `Update` — POST `/projects/{ns}/{name}/notifications/{id}` — update, redirect
- `Delete` — POST `/projects/{ns}/{name}/notifications/{id}/delete` — delete, redirect
- `Test` — POST `/projects/{ns}/{name}/notifications/{id}/test` — send test, redirect with result
- All require `CanUserManage` permission

### 6. Notification UI
- `web/templates/pages/notifications/list.html` — channel list with enable/disable, test button
- `web/templates/pages/notifications/form.html` — add/edit form with dynamic fields per type (JS shows/hides fields based on type dropdown)

### 7. Route + server wiring
- `routes.go`: Register notification routes behind RequireAuth
- `server.go`: Create NotificationChannelRepository, NotificationService, NotificationHandler
- Settings page: Add "Notifications" link (with count, like secrets)

### 8. Worker integration
- `worker.go`: Add `notifier` interface `{ NotifyBuild(ctx, *Build, *Project) }`
- In `recalcBuildStatus()` after status change to terminal: call `notifier.NotifyBuild()`
- `cmd/featherci/main.go`: Create NotificationService, pass to Worker

## Email Template
Nice HTML email with:
- Color-coded header (green for success, red for failure)
- Project name, build number, branch
- Commit SHA (short) + message
- Duration
- Link to build page
- FeatherCI branding footer

## Files to Create
| File | Description |
|------|-------------|
| `internal/database/migration_005_notifications.go` | notification_channels table |
| `internal/models/notification_channel.go` | Model + SQLite repository |
| `internal/notify/notifier.go` | Notifier interface |
| `internal/notify/event.go` | BuildEvent struct |
| `internal/notify/email_smtp.go` | SMTP email sender |
| `internal/notify/email_sendgrid.go` | Sendgrid API sender |
| `internal/notify/email_mailgun.go` | Mailgun API sender |
| `internal/notify/email_template.go` | Shared HTML email template |
| `internal/notify/slack.go` | Slack webhook sender |
| `internal/notify/discord.go` | Discord webhook sender |
| `internal/notify/pushover.go` | Pushover API sender |
| `internal/services/notifications.go` | Notification service |
| `internal/handlers/notification.go` | HTTP handlers |
| `web/templates/pages/notifications/list.html` | Channel list page |
| `web/templates/pages/notifications/form.html` | Add/edit form page |

## Files to Modify
| File | Change |
|------|--------|
| `internal/server/server.go` | Wire NotificationChannelRepo, NotificationService, NotificationHandler |
| `internal/server/routes.go` | Register notification routes |
| `internal/worker/worker.go` | Add notifier interface, call on terminal build |
| `cmd/featherci/main.go` | Create NotificationService, pass to Worker |
| `web/templates/pages/projects/settings.html` | Add "Notifications" link with count |

## Implementation Order
1. Database migration
2. Notification channel model + repository
3. Notifier interface + BuildEvent
4. Email template
5. SMTP, Sendgrid, Mailgun senders
6. Slack, Discord, Pushover senders
7. Notification service
8. Notification handler
9. UI templates
10. Route + server wiring
11. Worker integration
12. Settings page link

## Verification
1. `go vet ./...` — clean
2. `go test ./...` — all pass
3. `go build ./cmd/featherci/` — compiles
4. Visual: settings page links to notifications, list/add/edit/test all work
5. Test notification sends successfully for each channel type
