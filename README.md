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

## Custom Features (自定义特性)

本仓库基于官方 Tailscale 进行定制开发，增加了以下功能特性：

### 强制 DERP 中转 (`force-derp`)

支持通过 debug 命令动态配置当前节点是否强制仅使用 DERP 进行中转（禁用 P2P 直连打洞与 UDP 数据传输）：

```bash
# 强制开启 DERP 中转（禁止所有 UDP 直连，所有节点间流量均走 DERP 中转）
tailscale debug force-derp true

# 恢复默认模式（允许自动 NAT 打洞与直连）
tailscale debug force-derp false

# 查询当前节点是否处于强制 DERP 模式
tailscale debug force-derp
```

**实现机制**：
- 开启时立即清除已建立的直连路径缓存 (`bestAddr`)；
- 停止向协调服务器（Control Plane）上报本机 UDP 直连端点，对端节点不会尝试直连；
- 在路由层拦截直连发包，确保 WireGuard 报文只返回 DERP 地址；
- 阻断入站/出站的 UDP 直连报文与 disco 打洞探测。

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
