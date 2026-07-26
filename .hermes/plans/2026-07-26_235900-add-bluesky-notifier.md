# Add BlueSky Notifier Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Add a BlueSky notifier that posts DNS root zone diff notifications, parallel to the existing Slack and X notifiers.

**Architecture:** Introduce a new `internal/notify/bluesky.go` (and tests) implementing the existing `Notifier` interface. It will use the AT Protocol's `com.atproto.repo.createRecord` (`app.bsky.feed.post`) via HTTPS JSON, authenticated with App Password + handle. We will also extend `Config` with `BlueskyConfig`, wire loader/env vars, and route it from `cmd/dns-root-diff/main.go`.

**Tech Stack:** Go 1.26+, standard `net/http` + JSON, `gopkg.in/yaml.v3` for config. No external AT Protocol SDK.

---
## Current context / assumptions
- Notifier interface is defined in `internal/notify/notify.go` with `Notify(ctx, changes)` and `Name()`.
- X notifier lives in `internal/notify/twitter.go`; Slack notifier in `internal/notify/slack.go`. Both are good templates for formatting + config wiring.
- Config is loaded in `internal/config/config.go` and environment-overridden in `applyEnv`.
- Authentication for X is OAuth 2.0 user access token + refresh. BlueSky will use App Password.

## Decisions
- **Auth mode:** App Password (simpler, well-supported by AT Protocol).
- **Handle:** `dnsrootzonediff.bsky.social` (already created).

## Proposed approach
1. Add `BlueskyConfig` to `Config`.
2. Add env var overrides similar to other notifiers.
3. Implement `BlueskyNotifier` using AT Protocol JSON over HTTPS.
4. Add formatting helper for BlueSky post text.
5. Wire it into `main.go` alongside Slack/Twitter.
6. Add tests with HTTP replay/mocking.
7. Update README files with BlueSky config + env vars.

## Step-by-step plan

### Task 1: Add BlueskyConfig to Config

**Objective:** Add a `Bluesky` config block with App Password fields and env var support.

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Add struct + defaults to config.go**

Add:
```go
type BlueskyConfig struct {
    Enabled      bool   `yaml:"enabled"`
    Handle       string `yaml:"handle"`
    AppPassword  string `yaml:"app_password"`
    APIURL       string `yaml:"api_url"`
    MaxPostChars int    `yaml:"max_post_chars"`
}
```
Add `Bluesky BlueskyConfig` to `Config`.
Add defaults in `Default()`: `Enabled: false`, `APIURL: "https://bsky.social/xrpc"`, `MaxPostChars: 300`.
Add env overrides in `applyEnv()`: `BLUESKY_ENABLED`, `BLUESKY_HANDLE`, `BLUESKY_APP_PASSWORD`, `BLUESKY_API_URL`, `BLUESKY_MAX_POST_CHARS`.
Auto-enable when handle+app_password are set.

**Step 2: Run tests**

Run: `go test ./internal/config/ -v`
Expected: PASS (existing tests still green).

**Step 3: Add tests for BlueskyConfig**

In `internal/config/config_test.go`, add:
- `TestBlueskyConfigDefaults`
- `TestBlueskyEnvOverride`

**Step 4: Run tests again**

Expected: PASS including new Bluesky tests.

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add BlueskyConfig and env overrides"
```

---

### Task 2: Implement BlueSky AT Protocol client

**Objective:** Implement the minimal HTTPS client for Bluesky: createSession + createRecord using App Password.

**Files:**
- Create: `internal/notify/bluesky.go`
- Create/update: `internal/notify/bluesky_test.go`

**Step 1: Write failing tests**

Use `httptest.NewServer` to fake `com.atproto.server.createSession` and `com.atproto.repo.createRecord`; verify client exchanges credentials, reads JWT, and submits post text.

**Step 2: Run to verify failure**

Run: `go test ./internal/notify/ -v`
Expected: FAIL — package/file not implemented.

**Step 3: Minimal implementation**

```go
type BlueskyNotifier struct {
    cfg        BlueskyConfig
    httpClient *http.Client
    baseURL    string
}

func NewBlueskyNotifier(cfg BlueskyConfig) (*BlueskyNotifier, error) { ... }

func (n *BlueskyNotifier) Notify(ctx context.Context, changes []diff.Change) error { ... }

func (n *BlueskyNotifier) Name() string { return "bluesky" }
```

Endpoints:
- `POST /xrpc/com.atproto.server.createSession` (identifier=handle, password=app_password)
- `POST /xrpc/com.atproto.repo.createRecord` with repo=did (from session), collection=`app.bsky.feed.post`, record text + createdAt.

**Step 4: Run tests**

Run: `go test ./internal/notify/ -v`
Expected: PASS for Bluesky; existing Slack/Twitter tests remain green.

**Step 5: Commit**

```bash
git add internal/notify/bluesky.go internal/notify/bluesky_test.go
git commit -m "feat: add BlueSky notifier client"
```

---

### Task 3: Post formatter for BlueSky

**Objective:** Create concise human-readable diff summary suitable for Bluesky's ~300-char post limit.

**Files:**
- Modify: `internal/notify/format.go`
- Modify: `internal/notify/format_test.go`

**Step 1: Write failing test**

```go
func TestFormatBlueskyPost(t *testing.T) {
    ch := []diff.Change{...}
    text := FormatBlueSkyPost(ch, 300)
    assert.LessOrEqual(t, utf8.RuneCountInString(text), 300)
}
```

**Step 2: Run to verify failure**
**Step 3: Minimal format function**
**Step 4: Run + commit.**

Commit: `feat: add BlueSky post formatter`

---

### Task 4: Wire BlueSky into main and docs

**Objective:** Instantiate `BlueskyNotifier` in `cmd/dns-root-diff/main.go` when enabled; update README config examples.

**Files:**
- Modify: `cmd/dns-root-diff/main.go`
- Modify: `README.md`
- Modify: `README-ja.md`

**Step 1: Add notifier initiation wiring**
**Step 2: Run `go build ./...`**
**Step 3: Update docs with config.yaml sample + env vars**
**Step 4: Commit**

```bash
git add cmd/dns-root-diff/main.go README.md README-ja.md
git commit -m "feat: integrate BlueSky notifier"
```

---

### Task 5: Integration validation

**Objective:** Prove behavior starts correctly, failures are safe, and existing notifiers are unaffected.

**Tests/validation:**
- `go test ./...`
- `go vet ./...`
- `golangci-lint run`

**Step 1: Run all tests + lint**
**Step 2: Fix issues**
**Step 3: Commit**

```bash
git add go.sum go.mod .github/workflows/ci.yml
git commit -m "chore: verify Bluesky integration"
```

---

## Files likely to change
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/notify/bluesky.go` (new)
- `internal/notify/bluesky_test.go` (new)
- `internal/notify/format.go`
- `internal/notify/format_test.go`
- `cmd/dns-root-diff/main.go`
- `README.md`
- `README-ja.md`

## Risks / tradeoffs
- Auth secret handling: `bluesky_app_password` must be stored securely; config.yaml permissions should remain restrictive (0600).
- BlueSky post length limit (~300 chars visible): MVP supports single truncated post.
- AT Protocol schema evolves; pin minimal fields to avoid breakage (`text`, `createdAt`, `$type`).
