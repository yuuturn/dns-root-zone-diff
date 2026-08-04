![DNS Root Zone Diff](assets/logo@1024.png)

# DNS Root Zone Diff

A tool that mechanically detects changes to the DNS root zone file and sends notifications.

## Features

- Fetches the root zone from https://www.internic.net/domain/root.zone every 2 hours
- Fetches the DNSSEC trust anchors (root anchors) from https://data.iana.org/root-anchors/root-anchors.xml at the same interval
- Detects differences from the previous snapshot
- Categorizes changes (delegation / DNSSEC / glue / signature / zone / other)
- Summarizes substantive changes (excluding re-signing noise) and notifies via Slack Webhook
- Posts the same summary to X (Twitter) API v2, split into chunks of 280 characters
- Posts the same summary to BlueSky via AT Protocol, split into 300-character posts
- Browses diff history through a Web UI (built with Cloudflare [kumo](https://github.com/cloudflare/kumo)), separated into Root Zone and Root Anchors tabs

## Live

- X (Twitter): [@dnsrootzonediff](https://x.com/dnsrootzonediff) — live change notifications
- BlueSky: [@dnsrootzonediff.bsky.social](https://bsky.app/profile/dnsrootzonediff.bsky.social) — live change notifications
- Diff summary (Web UI): <https://dns-root-zone-diff.yfujii.net/>

## Local Installation

```bash
brew install go golangci-lint pre-commit gh
pre-commit install
```

## Configuration

Create a `config.yaml` file:

```yaml
zone_url: "https://www.internic.net/domain/root.zone"
# Source of the DNSSEC trust anchors (root anchors). Leave empty to disable root anchors monitoring (defaults to IANA)
anchor_url: "https://data.iana.org/root-anchors/root-anchors.xml"
fetch_interval: "2h"
data_dir: "/var/lib/dns-root-diff"
slack:
  enabled: false
  webhook_url: ""
twitter:
  enabled: false
  api_key: ""
  api_secret: ""
  access_token: ""
  access_secret: ""
  # Maximum number of tweets posted in a single detection (defaults to 4)
  max_posts: 4
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

Overridable via environment variables:

- `DNS_ROOT_DIFF_ZONE_URL`
- `DNS_ROOT_DIFF_ANCHOR_URL`
- `DNS_ROOT_DIFF_INTERVAL`
- `DNS_ROOT_DIFF_DATA_DIR`
- `SLACK_WEBHOOK_URL`
- `TWITTER_API_KEY`
- `TWITTER_API_SECRET`
- `TWITTER_ACCESS_TOKEN`
- `TWITTER_ACCESS_SECRET`
- `TWITTER_MAX_POSTS`
- `DNS_ROOT_DIFF_WEB_ENABLED`
- `DNS_ROOT_DIFF_WEB_LISTEN`
- `BLUESKY_HANDLE`
- `BLUESKY_APP_PASSWORD`
- `BLUESKY_API_URL`
- `BLUESKY_MAX_POST_CHARS`

## Change Categories and Notification Content

Detected changes are categorized by RR type.

| Category | RR types |
| --- | --- |
| `delegation` | NS |
| `DNSSEC` | DS, DNSKEY, NSEC, NSEC3PARAM |
| `glue` | A, AAAA |
| `signature` | RRSIG |
| `zone` | SOA, ZONEMD |
| `other` | anything else |

The root zone is re-signed every 12 hours, at which point about 2,800 RRSIG records are replaced and the SOA serial and ZONEMD serial/digest are also updated. Runs that consist **only** of this **mechanical change** (over 5,000 diffs) are **not notified to Slack/X**. The full history is retained in the Web UI.

Whether a change is mechanical is determined not by RR type but by **pairing old and new records**. Because an RRset with multiple records is decomposed into removed + added (not folded into a "modified" diff), re-signing pairs records by their unchanged fields and TTL, and only unpaired records are treated as mechanical changes.

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

When a run has substantive changes, it is notified in the following format. X splits at 280 characters and Slack at 3,500 characters; when more than one message is needed, a `(1/3)`-style number is added to the title line.

```
DNS Root Zone changes (1/2)
serial 2026072501 -> 2026072502
delegation 2 / DNSSEC 1 / glue 2
re-signing: 2800 RRSIG (omitted)

[delegation]
  - gone. NS ns1.dns.nic.gone.
  + newgtld. NS ns1.dns.nic.newgtld.
[DNSSEC]
  + newgtld. DS 12345 8 2 A1B2C3D4E5F60718293A4B5C6D7E8F...
```

The breakdown is listed per record. When it does not fit within the limit (`max_posts`), it switches to per-TLD aggregation (`  example. NS +1 -1`), and if it still does not fit, the dropped count is shown explicitly at the end as `... +N more changes`.

X is constrained to 280 characters, so it omits the `serial` and `re-signing` lines from the overview to keep room for per-record detail lines:

```
DNS Root Zone changes (1/2)
delegation 2 / DNSSEC 1 / glue 2

[delegation]
  - gone. NS ns1.dns.nic.gone.
  + newgtld. NS ns1.dns.nic.newgtld.
[DNSSEC]
  + newgtld. DS 12345 8 2 A1B2C3D4E5F60718293A4B5C6D7E8F...
```

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

## Usage

```bash
# Run once
make build
./bin/dns-root-diff -config config.yaml -once

# Run on a schedule
./bin/dns-root-diff -config config.yaml
```

## Web UI

Setting `web.enabled: true` starts a web server that lets you browse diff history in scheduled mode (`-once` does not start it). History is saved as JSON on each detection (zone in `data_dir/diffs/`, root anchors in `data_dir/anchor-diffs/`).

- `GET /` : list and detail views (React + [@cloudflare/kumo](https://github.com/cloudflare/kumo)), switchable via the Root Zone / Root Anchors tabs
- `GET /api/diffs?page=1&per_page=20` : root zone diff history list
- `GET /api/diffs/{id}` : root zone diff detail
- `GET /api/anchors/diffs?page=1&per_page=20` : root anchors diff history list
- `GET /api/anchors/diffs/{id}` : root anchors diff detail
- `GET /api/health` : health check

The frontend build artifacts are committed under `internal/web/static/` and embedded into the binary via go:embed, so ordinary builds and deploys do not require Node.js. If you modify the frontend (`web/frontend/`), rebuild and commit the artifacts together:

```bash
make frontend-install  # first time only
make frontend-build    # outputs to internal/web/static
```

If you expose it to the internet, a reverse proxy with TLS termination via nginx is recommended. See [deploy/nginx.conf.example](deploy/nginx.conf.example) for an example configuration and SELinux / firewalld steps.

## Testing

```bash
make test
make lint
```

## Deploying to a VPS

```bash
make deploy
```

Before deploying, create a dedicated user and data directory on the VPS, and place `config.yaml` there.
