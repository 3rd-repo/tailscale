# Tailscale

https://tailscale.com

Private WireGuard® networks made easy

## Overview

This repository contains the majority of Tailscale's open source code.
Notably, it includes the `tailscaled` daemon and
the `tailscale` CLI tool. The `tailscaled` daemon runs on Linux, Windows,
[macOS](https://tailscale.com/kb/1065/macos-variants/), and to varying degrees
on FreeBSD and OpenBSD. The Tailscale iOS and Android apps use this repo's
code, but this repo doesn't contain the mobile GUI code.

Other [Tailscale repos](https://github.com/orgs/tailscale/repositories) of note:

* the Android app is at https://github.com/tailscale/tailscale-android
* the Synology package is at https://github.com/tailscale/tailscale-synology
* the QNAP package is at https://github.com/tailscale/tailscale-qpkg
* the Chocolatey packaging is at https://github.com/tailscale/tailscale-chocolatey

For background on which parts of Tailscale are open source and why,
see [https://tailscale.com/opensource/](https://tailscale.com/opensource/).

## Custom Features

This fork includes custom enhancements built on top of upstream Tailscale:

### Force DERP Relay (`force-derp`)

Dynamically configure whether the current node is forced to relay all peer traffic exclusively via DERP (disabling direct peer-to-peer UDP connections and NAT hole punching):

```bash
# Force DERP relay mode (disable direct UDP and route all peer traffic through DERP)
tailscale debug force-derp true

# Restore default behavior (allow NAT hole punching and direct P2P connections)
tailscale debug force-derp false

# Query whether force-derp mode is currently enabled
tailscale debug force-derp
```

**How it works**:
- When enabled, immediately clears existing direct peer path caches (`bestAddr`).
- Suppresses advertising UDP endpoints to the coordination server (Control Plane), preventing peers from attempting direct connections.
- Overrides the WireGuard routing layer to strictly return the DERP relay address for outbound packets.
- Drops inbound direct UDP traffic and suppresses outgoing/incoming disco hole punching probes.

### Automated Release Workflow

This fork provides an automated GitHub Action workflow (`Release Custom Tailscale`) to build custom binaries on top of official releases:

1. **Sync with Upstream**: Click **Sync fork** on GitHub to pull the latest upstream releases and tags into your fork.
2. **Trigger the Workflow**:
   - Go to **Actions** -> **Release Custom Tailscale** -> **Run workflow**.
   - Input the desired upstream release tag (e.g., `v1.104.1`).
3. **Automated Pipeline**:
   - The workflow checks out the clean upstream release tag.
   - Automatically cherry-picks custom feature commits from `feature/custom` onto the target tag.
   - Cross-compiles `tailscale` and `tailscaled` for Linux (`amd64`, `arm64`, `armv7`) and Windows (`amd64`).
   - Generates SHA256 checksums (`sha256sums.txt`).
   - Uploads compiled archives directly to GitHub Releases (safely updating existing release assets via `--clobber`).
4. **Download Binaries**: Navigate to the repository's **Releases** page on GitHub to download pre-built packages.

## Using

We serve packages for a variety of distros and platforms at
[https://pkgs.tailscale.com](https://pkgs.tailscale.com/).

## Other clients

The [macOS, iOS, and Windows clients](https://tailscale.com/download)
use the code in this repository but additionally include small GUI
wrappers. The GUI wrappers on non-open source platforms are themselves
not open source.

## Building

We always require the latest Go release, currently Go 1.27. (While we build
releases with our [Go fork](https://github.com/tailscale/go/), its use is not
required.)

```
go install tailscale.com/cmd/tailscale{,d}
```

If you're packaging Tailscale for distribution, use `build_dist.sh`
instead, to burn commit IDs and version info into the binaries:

```
./build_dist.sh tailscale.com/cmd/tailscale
./build_dist.sh tailscale.com/cmd/tailscaled
```

If your distro has conventions that preclude the use of
`build_dist.sh`, please do the equivalent of what it does in your
distro's way, so that bug reports contain useful version information.

## Bugs

Please file any issues about this code or the hosted service on
[the issue tracker](https://github.com/tailscale/tailscale/issues).

## Contributing

PRs welcome! But please file bugs. Commit messages should [reference
bugs](https://docs.github.com/en/github/writing-on-github/autolinked-references-and-urls).

We require [Developer Certificate of
Origin](https://en.wikipedia.org/wiki/Developer_Certificate_of_Origin)
`Signed-off-by` lines in commits.

See [commit-messages.md](docs/commit-messages.md) (or skim `git log`) for our commit message style.

## About Us

[Tailscale](https://tailscale.com/) is primarily developed by the
people at https://github.com/orgs/tailscale/people. For other contributors,
see:

* https://github.com/tailscale/tailscale/graphs/contributors
* https://github.com/tailscale/tailscale-android/graphs/contributors

## Legal

WireGuard is a registered trademark of Jason A. Donenfeld.
