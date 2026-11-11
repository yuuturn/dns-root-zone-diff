![DNS Root Zone Diff](assets/logo@1024.png)

# DNS Root Zone Diff

A tool that mechanically detects changes to the DNS root zone file and sends notifications.

## Features

- Fetches the root zone from https://www.internic.net/domain/root.zone every 2 hours
- Fetches the DNSSEC trust anchors (root anchors) from https://data.iana.org/root-anchors/root-anchors.xml at the same interval
- Fetches the IANA TLD list (https://data.iana.org/TLD/tlds-alpha-by-domain.txt) at the same interval
- Detects differences from the previous snapshot
- Categorizes changes (delegation / DNSSEC / glue / signature / zone / TLD / other)
- Summarizes substantive changes (excluding re-signing noise) and notifies via Slack Webhook
- Posts the same summary to X (Twitter) API v2, split into chunks of 280 characters
- Posts the same summary to BlueSky via AT Protocol, split into 300-character posts
- Browses diff history through a Web UI (built with Cloudflare [kumo](https://github.com/cloudflare/kumo)), separated into Root Zone, Root Anchors, TLDs, and TLD History tabs. TLD History shows the root zone diff as a per-TLD timeline. The header carries a dark / light switch and a link to the GitHub repository

## Live

- X (Twitter): [@dnsrootzonediff](https://x.com/dnsrootzonediff) — live change notifications
- BlueSky: [@dnsrootzonediff.bsky.social](https://bsky.app/profile/dnsrootzonediff.bsky.social) — live change notifications
- Diff summary (Web UI): <https://dns-root-zone-diff.yfujii.net/>

## Development Requirements

| Tool | Purpose | Install |
|---|---|---|
| [mise](https://mise.jdx.dev/) | Pins Go / golangci-lint / gitleaks versions | `curl https://mise.run \| sh` (`brew install mise` on macOS) |
| Go 1.26.5 | Build and test | `mise install` (pinned in [`mise.toml`](mise.toml)) |
| golangci-lint 2.12.2 / gitleaks 8.30.1 | Lint and secret scanning | `mise install` (same file) |
| C compiler | Required by `go test -race` (CGO) | macOS: `xcode-select --install` / Ubuntu: `sudo apt install build-essential` |
| [pre-commit](https://pre-commit.com/) | Commit hooks (gofmt / go vet / go test / golangci-lint / gitleaks) | macOS: `brew install pre-commit` / Ubuntu: `pipx install pre-commit` |
| Node.js + npm | Only when changing `web/frontend/` | Install any suitable version with mise (CI uses Node 22) |
| [gh](https://cli.github.com/) (optional) | Creating pull requests | `brew install gh` etc. |

Setup:

```bash
mise install        # installs Go / golangci-lint / gitleaks
pre-commit install  # enables the commit hooks
# To make go available outside the repository as well
mise use -g go@1.26.5
```

`pre-commit` is Python-based and not managed by mise, so install it with the method for your OS.

`make test` runs `go test -race -cover ./...`, so a working C compiler with CGO enabled is
required (Xcode Command Line Tools on macOS, build-essential on Ubuntu).

If you change the frontend, install Node.js and run:

```bash
make frontend-install  # first time only
make frontend-build    # writes to internal/web/static
```

`make ci` runs the same checks as CI locally (gitleaks / vet / lint / test / build).

## Configuration

Copy [`config.yaml.example`](config.yaml.example) to `config.yaml` and fill in secrets.
An example:

```yaml
zone_url: "https://www.internic.net/domain/root.zone"
# Source of the DNSSEC trust anchors (root anchors). Leave empty to disable root anchors monitoring (defaults to IANA)
anchor_url: "https://data.iana.org/root-anchors/root-anchors.xml"
# Source of the IANA TLD list. Leave empty to disable TLD list monitoring (defaults to IANA)
tld_url: "https://data.iana.org/TLD/tlds-alpha-by-domain.txt"
fetch_interval: "2h"
# VPS data directory. Use "./data" for local runs.
data_dir: "/var/lib/dns-root-diff"
slack:
  enabled: false
  webhook_url: ""
twitter:
  enabled: false
  # OAuth 1.0a (legacy). Prefer OAuth 2.0 for production.
  api_key: ""
  api_secret: ""
  access_token: ""
  access_secret: ""
  # OAuth 2.0 user access token (recommended). Tokens expire in ~2h and are
  # refreshed automatically; updated tokens are written back to this file.
  oauth2_access_token: ""
  oauth2_refresh_token: ""
  oauth2_client_id: ""
  oauth2_client_secret: ""
  # Maximum number of tweets posted in a single detection (defaults to 6)
  max_posts: 6
web:
  enabled: false
  listen: "127.0.0.1:8080"
bluesky:
  enabled: false
  handle: ""
  app_password: ""
  api_url: "https://bsky.social/xrpc"
  max_post_chars: 300
```

Twitter and BlueSky credentials in `config.yaml` (or env) auto-enable their notifier even when `enabled: false` is set. Slack is auto-enabled only by the `SLACK_WEBHOOK_URL` environment variable; when the webhook comes from `config.yaml`, set `slack.enabled: true`. To disable a notifier, clear its credentials.

OAuth 2.0 tokens supplied through environment variables are re-applied on every reload, so tokens refreshed by the service cannot be persisted in that setup. Put `oauth2_access_token` / `oauth2_refresh_token` in `config.yaml` when relying on automatic refresh.

Overridable via environment variables:

- `DNS_ROOT_DIFF_ZONE_URL`
- `DNS_ROOT_DIFF_ANCHOR_URL`
- `DNS_ROOT_DIFF_TLD_URL`
- `DNS_ROOT_DIFF_INTERVAL`
- `DNS_ROOT_DIFF_DATA_DIR`
- `SLACK_WEBHOOK_URL`
- `TWITTER_API_KEY`
- `TWITTER_API_SECRET`
- `TWITTER_ACCESS_TOKEN`
- `TWITTER_ACCESS_SECRET`
- `TWITTER_OAUTH2_ACCESS_TOKEN`
- `TWITTER_OAUTH2_REFRESH_TOKEN`
- `TWITTER_OAUTH2_CLIENT_ID`
- `TWITTER_OAUTH2_CLIENT_SECRET`
- `TWITTER_MAX_POSTS`
- `DNS_ROOT_DIFF_WEB_ENABLED`
- `DNS_ROOT_DIFF_WEB_LISTEN`
- `BLUESKY_HANDLE`
- `BLUESKY_APP_PASSWORD`
- `BLUESKY_API_URL`
- `BLUESKY_MAX_POST_CHARS`

`BLUESKY_ENABLED` is also read, but it has no practical effect: credentials auto-enable the notifier, and no notifier is created without credentials. Disable BlueSky by clearing `handle` / `app_password`.

Credentials (including OAuth tokens), `zone_url` / `anchor_url` / `tld_url`, and `data_dir` are reloaded on every tick; source URL changes apply to the next fetch. Restart is required for `fetch_interval` (the ticker is created at startup) and `web.enabled` / `web.listen` (the listener is bound at startup). A changed `data_dir` applies to new snapshots and history, but the Web UI keeps using the history directory captured at startup.

## Change Categories and Notification Content

Detected changes are categorized by RR type.

| Category | RR types |
| --- | --- |
| `delegation` | NS |
| `DNSSEC` | DS, DNSKEY, NSEC, NSEC3, NSEC3PARAM, CDS, CDNSKEY |
| `glue` | A, AAAA |
| `signature` | RRSIG |
| `zone` | SOA, ZONEMD |
| `TLD` | TLD (pseudo RR type used by TLD list monitoring) |
| `other` | anything else |

The root zone is re-signed every 12 hours, at which point about 2,800 RRSIG records are replaced and the SOA serial and ZONEMD serial/digest are also updated. Runs that consist **only** of this **mechanical change** (paired RRSIG / SOA / ZONEMD updates with no unpaired substantive diffs) are **not notified to Slack / X / BlueSky**. The full history is retained in the Web UI.

The first run (no previous snapshot) only saves a baseline snapshot; no notification or history is recorded, since every record would otherwise be reported as added.

Whether a change is mechanical is determined not by RR type but by **pairing old and new records**. Because an RRset with multiple records is decomposed into removed + added (not folded into a "modified" diff), re-signing pairs records by their unchanged fields and TTL, and only paired records are treated as mechanical changes. Unpaired additions/removals (missing signatures, algorithm rollover, etc.) remain substantive.

| RR type | Fields used for pairing | Fields allowed to change |
| --- | --- | --- |
| RRSIG | type covered, algorithm, labels, original TTL, signer | expiration, inception, key tag, signature |
| SOA | MNAME, RNAME, refresh, retry, expire, minimum | serial |
| ZONEMD | scheme, hash algorithm | serial, digest |

Therefore the following are notified as substantive changes:

- TTL changes (RRSIG / SOA / ZONEMD, any of them)
- Additions/removals that did not pair (missing signatures, digest algorithm rollover, etc.)
- Changes to SOA MNAME/RNAME/refresh/retry/expire/minimum
- Changes to ZONEMD scheme/hash algorithm
- Changes to RRSIG algorithm/signer

The key tag is not included in the pairing fields. A ZSK rollover changes the key tag of all RRSIGs at once, but the key change itself is already notified as an apex DNSKEY change. Conversely, a signature algorithm rollover leaves all RRSIGs unpaired and is notified as a large batch of changes (an event expected only once every few decades, so it is intentionally left as-is).

When a run has substantive changes, it is notified in the following format. X splits at 280 weighted characters, Slack at 3,500 characters, and BlueSky at `max_post_chars` (default 300); when more than one message is needed, a `(1/3)`-style number is added to the title line. The example below is the Slack/BlueSky form, which includes the `re-signing` line; X omits that line (see the X example later).

The title line names the SOA serials the delta is between, so which serial-to-serial change a message reports is clear from the first line alone (the serial transition is not repeated in the body). When the diff contains no SOA change (such as a root anchors or TLD list notification), the title falls back to `DNS Root Zone changes` / `DNS Root Anchors changes` / `DNS TLD List changes`.

```
2026072501 -> 2026072502 changes (1/2)
delegation 2 / DNSSEC 1 / glue 2
re-signing: 5600 RRSIG (omitted)

[delegation]
  - gone. NS ns1.dns.nic.gone.
  + newgtld. NS ns1.dns.nic.newgtld.
[DNSSEC]
  + newgtld. DS 12345 8 2 A1B2C3D4E5F60718293A4B5C6D7E8F...
```

The `re-signing` count is the number of RRSIG changes (removed + added), roughly twice the number of replaced records. The breakdown is listed per record (RData capped at 40 characters before a trailing `...`). When it does not fit within the limit (`max_posts`), it switches to per-record lines with the RData shortened to its leading fields; for DS records this keeps the key tag / algorithm / digest type (`  + newgtld. DS 12345 8 2 ...`). If it still does not fit, it switches to per-TLD aggregation (`  example. NS +1 -1`), and if it still does not fit, the dropped count is shown explicitly at the end as `... +N more changes`. Slack sends at most 3 messages and X at most `max_posts`; BlueSky has no message-count cap, only `max_post_chars` per post.

DNSKEY / CDNSKEY detail lines carry an annotation with the key tag, role, and key size so the key can be identified. The annotation is appended after the (truncated) RData for both the full RData (40 characters) and the shortened form. The key tag is computed per RFC 4034 Appendix B, the key size from the public key per RFC 3110 (RSA), and the role (KSK / ZSK) from the SEP bit. The size is shown as `2048b` for RSA and as `p256` / `ed25519` for ECDSA / EdDSA; it is omitted when it cannot be computed:

```
[DNSSEC]
  - . DNSKEY 256 3 8 AwEAAcorNxNc9t0ue1ZenPjVMRujlQ2Y... (key tag = 8763 (zsk), size = 2048b)
  + . DNSKEY 257 3 8 AwEAAaz/tAm8yTn4Mfeh5eyI96WSVexT... (key tag = 20326 (ksk), size = 2048b)
```

X is constrained to 280 characters, so it omits the `re-signing` line from the overview to keep room for per-record detail lines (default `max_posts: 6`):

```
2026072501 -> 2026072502 changes (1/2)
delegation 2 / DNSSEC 1 / glue 2

[delegation]
  - gone. NS ns1.dns.nic.gone.
  + newgtld. NS ns1.dns.nic.newgtld.
[DNSSEC]
  + newgtld. DS 12345 8 2 A1B2C3D4E5F60718293A4B5C6D7E8F...
```

## Failure handling

The snapshot is saved before notification and history. If the process stops between the snapshot save and notification, that change is not notified again (the next run sees no diff) and its history entry is lost. If saving the snapshot fails, the run aborts before notifying and the next tick retries the same diff. Notification failures are logged and not retried; history is still written (unless that write also fails, in which case it is only logged).

## Root Anchors (DNSSEC Trust Anchors) Monitoring

Like the root zone, the IANA root anchors file (https://data.iana.org/root-anchors/root-anchors.xml) is fetched at the same interval (`fetch_interval`) and compared against the previous snapshot.

- Each `<KeyDigest>` is treated as the equivalent of a DS record: **key additions** (`+`), **removals** (`-`), and **retirements** (`~`, when a `validUntil` attribute is added) are detected. The retirement date is shown as `YYYY-MM-DD` at the end of the RData on the detail line
- The first run only saves a baseline snapshot; no notification or history is recorded
- Diff history is stored separately from the zone in `data_dir/anchor-diffs/` and is browsable in the Root Anchors tab of the Web UI
- Notifications use the title `DNS Root Anchors changes` and widen the RDATA limit on detail lines so the 64-character DS digests are not truncated

```text
DNS Root Anchors changes
DNSSEC 2

[DNSSEC]
  + Kmyv6jo DS 38696 8 2 683D2D0ACB8C9B712A1948B27F741219298D0A450D612C483AF444A4C0FB2B16
  ~ Kjqmt7v DS 19036 8 2 49AAC11D7B6F6446702E54A1607371607A1A41855200FD2CE1CDDE32F24E8FB5
    -> 19036 8 2 49AAC11D7B6F6446702E54A1607371607A1A41855200FD2CE1CDDE32F24E8FB5 2019-01-11
```

## TLD List Monitoring

Like the root zone and root anchors, the IANA TLD list (https://data.iana.org/TLD/tlds-alpha-by-domain.txt) is fetched at the same interval (`fetch_interval`) and compared against the previous snapshot.

- The numeric version from the `# Version ...` header line is used as the history entry's serial. A run where only the version advances (the TLD set is unchanged) produces no diff
- Each TLD is treated as one record of the pseudo RR type `TLD`, so **TLD additions** (`+`) and **removals** (`-`) are detected. Labels are normalized to lowercase so case-only differences are ignored
- The first run only saves a baseline snapshot; no notification or history is recorded
- Diff history is stored separately from the zone and root anchors in `data_dir/tld-diffs/` and is browsable in the TLDs tab of the Web UI
- Notifications use the title `DNS TLD List changes`
- On X / BlueSky, the notification is followed by a reply containing minimal IANA WHOIS information (`whois.iana.org`) for each added or removed TLD: domain / status / remarks / created / changed / source. Slack does not receive the WHOIS reply, and TLDs whose WHOIS lookup fails are skipped

```text
DNS TLD List changes
TLD 2

[TLD]
  + newgtld. TLD
  - oldgtld. TLD
```

Example X / BlueSky reply:

```text
% whois helsinki.
...
domain:       HELSINKI
status:       FORMER
remarks:      Registration information:
created:      2015-12-10
changed:      2026-10-28
source:       IANA
```

## TLD History

The Web UI's **TLD History** tab aggregates the root zone diff history by TLD and
shows each TLD's changes as a timeline. This is separate from IANA TLD list
monitoring (the TLDs tab).

- Lists the TLDs that have change history, newest change first. The search box
  opens any TLD, whether or not it has history
- A TLD's history includes records at its apex (`NS` / `DS` / `DNSKEY` for
  `example.`) as well as in-bailiwick glue (`A` / `AAAA` for `ns1.example.`)
- RRSIG re-signing (category=signature) is excluded because it is not a
  substantive TLD change
- On first use, existing files under `data_dir/diffs/` are backfilled and cached
  in `data_dir/tld-history/index.json`. Later runs only ingest newly added diffs,
  so the view stays fast as history grows
- Each event links to the corresponding root zone diff detail (`/diffs/<id>`)

## Usage

```bash
# Run once
make build
./bin/dns-root-diff -config config.yaml -once

# Run on a schedule
./bin/dns-root-diff -config config.yaml
```

## Web UI

Setting `web.enabled: true` starts a web server that lets you browse diff history in scheduled mode (`-once` does not start it). History is saved as JSON on each detection (zone in `data_dir/diffs/`, root anchors in `data_dir/anchor-diffs/`, TLD list in `data_dir/tld-diffs/`). TLD History is built lazily from `data_dir/diffs/` and cached in `data_dir/tld-history/index.json`.

- `GET /` : list and detail views (React + [@cloudflare/kumo](https://github.com/cloudflare/kumo)), switchable via the Root Zone / Root Anchors / TLDs / TLD History tabs. The header's moon / sun switch toggles dark / light (the choice is stored in localStorage and follows the OS setting until one is made), and "View on GitHub" links to the repository. Category tabs are deep-linkable through the URL fragment, so `/diffs/<id>#DNSSEC` opens the DNSSEC filter directly. On narrow screens the diff tables scroll horizontally instead of clipping columns
- `GET /api/diffs?page=1&per_page=20` : root zone diff history list
- `GET /api/diffs/{id}` : root zone diff detail. Optional query params: `category=<name>` filters changes by category, and `page` / `per_page` (default 100, max 100) paginate them — when paging params are present the response also carries `changes_total` and `total_pages`. DNSKEY / CDNSKEY changes also carry `old_key` / `new_key` (`tag` / `role` / `size`), which the UI renders with the same annotation as the notifications
- `GET /api/anchors/diffs?page=1&per_page=20` : root anchors diff history list
- `GET /api/anchors/diffs/{id}` : root anchors diff detail (same optional params as above)
- `GET /api/tlds/diffs?page=1&per_page=20` : TLD list diff history list
- `GET /api/tlds/diffs/{id}` : TLD list diff detail (same optional params as above)
- `GET /api/tld-history?page=1&per_page=20` : per-TLD history list (TLDs with change history, newest change first)
- `GET /api/tld-history/{tld}?page=1&per_page=20` : history for one TLD. Detection events are returned newest first and each carries the changes relevant to that TLD. `stats` aggregates all events regardless of the page. RRSIG re-signing is excluded. A TLD without history returns an empty `events` list
- `GET /api/health` : health check

List and detail responses carry `ETag` headers; requests with a matching `If-None-Match` are answered with `304 Not Modified`.

The frontend build artifacts are committed under `internal/web/static/` and embedded into the binary via go:embed, so ordinary builds and deploys do not require Node.js. If you modify the frontend (`web/frontend/`), rebuild and commit the artifacts together:

```bash
make frontend-install  # first time only
make frontend-build    # outputs to internal/web/static
```

If you expose it to the internet, a reverse proxy with TLS termination via nginx is recommended. See [deploy/nginx.conf.example](deploy/nginx.conf.example) for an example configuration. Its SELinux / firewalld notes apply to RHEL-family hosts; Ubuntu uses AppArmor/ufw. The example keeps the `listen ... http2` form because Ubuntu 24.04 ships nginx 1.24; on nginx 1.25.1+ switch to `http2 on;` (a comment in the file shows where).

## Tracing (OpenTelemetry)

Traces can be exported over OTLP. Tracing is **disabled by default**: it is enabled only when
`OTEL_EXPORTER_OTLP_ENDPOINT` or `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` is set (otherwise the
instrumentation is a no-op).

Instrumented spans:

| Span | Description |
|---|---|
| `GET /api/diffs`, ... | Web UI / API requests (low-cardinality route names such as `GET /api/diffs/{id}`) |
| `run` / `zone.run` / `anchor.run` / `tld.run` | One scheduled run (also `-once`), with `zone.records` / `zone.changes` / `anchor.digests` / `tld.count` / `tld.changes` attributes |
| `HTTP GET` | Root zone / root anchors / TLD list fetches (child spans of `zone.run` / `anchor.run` / `tld.run`; trace context is propagated via HTTP headers) |

Enable it with environment variables:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.example.com:4317
export OTEL_EXPORTER_OTLP_INSECURE=true   # when using OTLP/gRPC without TLS
export OTEL_SERVICE_NAME=dns-root-diff    # optional (defaults to this value)
export OTEL_RESOURCE_ATTRIBUTES=deployment.environment=production
./bin/dns-root-diff -config config.yaml
```

When running under systemd, pass the variables with a drop-in:

```ini
# /etc/systemd/system/dns-root-diff.service.d/telemetry.conf
[Service]
Environment=OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.example.com:4317
Environment=OTEL_EXPORTER_OTLP_INSECURE=true
Environment=OTEL_SERVICE_NAME=dns-root-diff
Environment=OTEL_RESOURCE_ATTRIBUTES=deployment.environment=production
```

```bash
sudo systemctl daemon-reload
sudo systemctl restart dns-root-diff
```

The exporter is OTLP/gRPC. Settings such as headers, TLS, compression, and timeouts
follow the standard `OTEL_EXPORTER_OTLP_*` environment variables. `OTEL_EXPORTER_OTLP_PROTOCOL`
is not used (HTTP/protobuf is not supported). Any OTLP-compatible backend
(Zabbix 8.0 APM, Grafana Tempo, Jaeger, ...) can be used.

## Testing

```bash
make test
make lint
```

## Deploying to a VPS

Before the first deploy, on the VPS:

1. Create the `dns-root-diff` user/group and install [`deploy/dns-root-diff.service`](deploy/dns-root-diff.service) under `/etc/systemd/system/`.
2. Place `config.yaml` at `/etc/dns-root-diff/config.yaml` (readable/writable by that user; OAuth 2.0 token refresh writes back to this file).
3. Set `data_dir: "/var/lib/dns-root-diff"` so it matches systemd `StateDirectory=dns-root-diff`.
4. Optionally configure nginx using [`deploy/nginx.conf.example`](deploy/nginx.conf.example).

Then from a development machine (macOS arm64 cross-compiles to linux/amd64):

```bash
make deploy
```

`make deploy` runs [`deploy/deploy.sh`](deploy/deploy.sh) (cross-compile, scp, `restorecon` when available, `daemon-reload`, restart, status).
