# age-imap-decryptor

[![CI](https://github.com/dhcgn/age-imap-decryptor/actions/workflows/ci.yml/badge.svg)](https://github.com/dhcgn/age-imap-decryptor/actions/workflows/ci.yml)
[![Release](https://github.com/dhcgn/age-imap-decryptor/actions/workflows/release.yml/badge.svg)](https://github.com/dhcgn/age-imap-decryptor/actions/workflows/release.yml)
[![Docker](https://github.com/dhcgn/age-imap-decryptor/actions/workflows/docker.yml/badge.svg)](https://github.com/dhcgn/age-imap-decryptor/actions/workflows/docker.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/dhcgn/age-imap-decryptor)](https://goreportcard.com/report/github.com/dhcgn/age-imap-decryptor)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/dhcgn/age-imap-decryptor/main)
[![GitHub Release](https://img.shields.io/github/v/release/dhcgn/age-imap-decryptor)](https://github.com/dhcgn/age-imap-decryptor/releases)
[![GHCR](https://img.shields.io/badge/GHCR-ghcr.io%2Fdhcgn%2Fage--imap--decryptor-2ea44f?logo=github)](https://github.com/dhcgn/age-imap-decryptor/pkgs/container/age-imap-decryptor)
![GitHub License](https://img.shields.io/github/license/dhcgn/age-imap-decryptor)

Receiver-side companion to [age-web-gateway](https://github.com/dhcgn/age-web-gateway). Watches your IMAP inbox, decrypts `.age`-encrypted attachments with your private age key, and saves the plaintext files — with their original filenames restored — to a local directory.

## How it fits together

[age-web-gateway](https://github.com/dhcgn/age-web-gateway) lets anyone send you encrypted messages and files from a browser at [age.hdev.io](https://age.hdev.io/). All encryption happens client-side before anything leaves the sender's device; only ciphertext reaches your inbox. **age-imap-decryptor** is the other half: it sits next to your mailbox and automatically decrypts everything that arrives.

```
Sender (browser)
      │  client-side age encryption
      ▼
age-web-gateway ──────────────────► Your IMAP inbox
                  encrypted email         │
                                  age-imap-decryptor
                                          │
                              decrypted/
                              └── 2026-05-25_18.13.45Z_7/
                                  ├── message.age.plain
                                  └── screenshot-age-web.png
```

## Features

- **Initial scan** — processes all existing messages in the mailbox on startup
- **IMAP IDLE** — reacts to new mail within seconds, no polling required
- **Sidecar-aware rename** — `attachment-001.payload.age` is renamed to the original filename declared in the accompanying `attachment-001.meta.age` sidecar
- **Stateless deduplication** — uses output directory names (`<timestamp>_<uid>`) to skip already-processed messages; no database or state file needed
- **Read-only mailbox** — no flags are set, no messages are moved or deleted
- **Minimal footprint** — single static binary; Docker image built on `scratch`

### About IMAP IDLE

IMAP IDLE ([RFC 2177](https://datatracker.ietf.org/doc/html/rfc2177)) allows the server to push a notification to the client the instant a new message arrives, rather than requiring the client to poll every N seconds. The underlying [go-imap](https://github.com/BrianLeishman/go-imap) library handles the protocol details — IDLE sessions are renewed automatically every ~5 minutes and the connection is re-established after any interruption. The result: decryption happens within seconds of delivery with zero polling overhead.

## Prerequisites

1. **age identity file** — generate one with [`age-keygen`](https://github.com/FiloSottile/age):
   ```bash
   age-keygen -o ~/.age/id.txt
   # Publish the public key so senders can find it:
   #   DNS TXT  _age.<yourdomain.com>  "age1..."
   #   or HTTPS https://<yourdomain.com>/.well-known/age
   ```
2. **IMAP mailbox** — port 993 (TLS), any provider works
3. **Sender filter** *(optional)* — set `filter.sender: "no-reply@example.tld"` to process only gateway-originated mail and ignore everything else

## Configuration

Copy `config.example.yaml` to `config.yaml` and fill in your values:

```yaml
imap:
  server: "imap.example.com"
  port: 993                          # default: 993
  username: "user@example.com"
  password: "supersecret"            # or leave empty and set WATCHER_IMAP_PASSWORD
  mailbox: "INBOX"                   # default: INBOX

filter:
  attachment_extension: ".age"       # default: .age
  sender: "no-reply@example.tld"     # optional; server-side FROM filter

keys:
  path: "~/.age"                     # directory containing age identity files

output:
  base_dir: "/var/lib/age-imap-decryptor/decrypted"

runtime:
  initial_scan: true   # process existing messages on startup
  idle: true           # keep running and react to new messages via IMAP IDLE
```

The password can be supplied via the `WATCHER_IMAP_PASSWORD` environment variable instead of the config file. Protect `config.yaml` with `chmod 600` if you store the password there.

| `initial_scan` | `idle` | Behaviour |
|:-:|:-:|---|
| `true` | `true` | Scan existing messages, then enter IDLE (default) |
| `true` | `false` | One-shot scan, then exit |
| `false` | `true` | IDLE only — skip the initial scan |
| `false` | `false` | Invalid — service exits with an error |

## How to run

### Docker (recommended for always-on use)

```bash
docker run -d \
  --name age-imap-decryptor \
  --restart unless-stopped \
  -v ~/.age:/keys:ro \
  -v /var/lib/age-imap-decryptor/decrypted:/output \
  -v /path/to/config.yaml:/config.yaml:ro \
  ghcr.io/dhcgn/age-imap-decryptor --config /config.yaml
```

Supplying the password via environment variable:

```bash
docker run -d \
  -e WATCHER_IMAP_PASSWORD=mysecret \
  -v ~/.age:/keys:ro \
  -v /var/lib/age-imap-decryptor/decrypted:/output \
  -v /path/to/config.yaml:/config.yaml:ro \
  ghcr.io/dhcgn/age-imap-decryptor --config /config.yaml
```

### Binary

Download a pre-built binary from the [Releases](https://github.com/dhcgn/age-imap-decryptor/releases) page, or install with Go:

```bash
go install github.com/dhcgn/age-imap-decryptor/cmd/age-imap-decryptor@latest
```

```bash
# Default: scan existing messages, then enter IDLE
./age-imap-decryptor --config config.yaml

# One-shot scan only (no IDLE)
./age-imap-decryptor --config config.yaml --idle=false

# IDLE only (skip initial scan)
./age-imap-decryptor --config config.yaml --scan=false
```

The service handles `SIGINT` / `SIGTERM` for clean shutdown.

### Build from source

```bash
git clone https://github.com/dhcgn/age-imap-decryptor
cd age-imap-decryptor
go build ./cmd/age-imap-decryptor
```

Requires Go 1.25 or later.

## Output layout

Each processed message gets its own directory named `<UTC-timestamp>_<IMAP-UID>`:

```
decrypted/
└── 2026-05-25_18.13.45Z_7/
    ├── message.age.plain              ← decrypted message body
    ├── attachment-001.meta.age.plain  ← sidecar JSON (kept for reference)
    └── screenshot-age-web.png        ← payload renamed from original filename
```

The sidecar file contains the original filename, MIME type, size, and SHA-256 of the payload — everything needed to verify the file after decryption.

If a message has already been processed (directory with matching `_<uid>` suffix exists), it is skipped on restart — no duplicate output, no state file required.

## Related

- [age-web-gateway](https://github.com/dhcgn/age-web-gateway) — sender-side browser tool; live instance at [age.hdev.io](https://age.hdev.io/)
- [age](https://github.com/FiloSottile/age) — the underlying encryption format and Go library
