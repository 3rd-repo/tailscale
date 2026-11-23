# Tailscale Fork 定制开发与维护工作流指南

本文档系统整理了本 Tailscale Fork 仓库的分支策略、自定义特性开发流程、GitHub Actions 同步/构建发布机制及日常维护规范。

本项目采用 **模型 A：`feature/custom` 直接基于某个上游 release tag**，因此发布产物天然就是「上游 tag + 定制」，构建时不需要 cherry-pick。

---

## 一、分支架构与设计原则

```text
upstream (tailscale/tailscale)
       │  release tag（如 v1.104.7）
       ▼
┌────────────────┐
│ feature/custom │ ─── 上游 release tag + 全部自定义功能代码 + 文档（不含任何 workflow）
└────────────────┘
       ▲
       │ 其当前基线记录在 git tag `custom-base` 中

   ┌─────────┐
   │  main   │ ─── 上游 main + 仅 GitHub Actions workflow（严禁业务代码侵入）
   └─────────┘
```

### 1. `feature/custom` 分支（定制特性分支）
- **定位**：唯一的功能分支。
- **基线**：始终基于某个**上游 release tag**（例如 `v1.104.7`），即 `feature/custom = <tag> + 定制`。
- **原则**：
  - 所有新特性（Go 源代码改动）在此分支开发、测试、提交。
  - **禁止**修改 `.github/workflows/` 下的任何文件；workflow 一律在 `main` 分支维护。
  - 当前基线由 git tag **`custom-base`** 记录（它指向所用的上游 tag 的 commit）。
  - 切换到新的上游 release 时，用 `rebase-feature.yml`（或本地 `git rebase --onto`）迁移基线。

### 2. `main` 分支（主分支）
- **定位**：只承载 GitHub Actions workflow，并可与上游 `main` 保持同步（用于参考/后续 rebase）。
- **原则**：
  - **绝不合并任何自定义业务代码**。
  - **只允许 GitHub Actions workflow 存在于此分支**（当前为 `release-custom.yml` 与 `rebase-feature.yml`）。
  - 与上游同步用 **merge**（因本分支含 workflow 提交），此同步**不会**影响 `feature/custom` 的基线。

### 3. `custom-base` git tag（基线标记）
- 指向 `feature/custom` 当前所基于的上游 tag commit。
- `rebase-feature.yml` 用它作为旧基线来做 `git rebase --onto <新tag> custom-base`，成功后再把它更新为新 tag。
- 发布时 `release-custom.yml` 用它校验「你输入的 tag 是否就是当前基线」。

---

## 二、当前已集成的自定义特性

| 特性名称 | 作用说明 | 涉及主要文件 |
| :--- | :--- | :--- |
| **强制指定 DERP 中继** | 增加 `tailscale debug force-derp` 子命令（布尔开关），强制所有流量只走 DERP，禁用直连 UDP / NAT 打洞 | `cmd/tailscale/cli/debug_forcederp.go`、`wgengine/magicsock/forcederp.go` |
| **默认免日志与免遥测** | `tailscaled` 默认开启 `--no-logs-no-support=true`，无需每次手动加参即可避免上传遥测日志 | `cmd/tailscaled/tailscaled.go` |
| **显示隐藏守护进程子命令** | `tailscaled --help` 显式列出并说明 `install-system-daemon`、`uninstall-system-daemon` 和 `debug` | `cmd/tailscaled/tailscaled.go` |
| **服务安装支持自定义启动参数** | `install-system-daemon [flags]` 允许透传 `-statedir`、`--port` 等参数并持久化注册到系统服务 | `cmd/tailscaled/install_*.go` |
| **Linux 原生 systemd 支持** | 为 Linux 平台增加 `install-system-daemon` 与 `uninstall-system-daemon`，自动注册、配置并启动 systemd 服务 | `cmd/tailscaled/install_linux.go` |
| **独立 DERP 构建流水线** | Release Workflow 独立并行编译并打包 Linux x86 (`linux/amd64`) 的 `derper` 中继服务端 | `.github/workflows/release-custom.yml`（在 main 分支） |

---

## 三、开发新特性的标准流程

### 步骤 1：检出并确保位于 `feature/custom` 分支
```bash
git checkout feature/custom
git pull origin feature/custom
```

### 步骤 2：编写功能代码与本地编译测试
```bash
go test ./wgengine/magicsock/... ./cmd/tailscaled/... ./cmd/tailscale/cli/...
```
跨平台矩阵编译验证：
```bash
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o /dev/null ./cmd/tailscaled
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -o /dev/null ./cmd/tailscaled
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o /dev/null ./cmd/tailscaled
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/tailscaled
```

### 步骤 3：同步编写单元测试
- 为新功能编写自动化测试（例如新增 `xxx_test.go`）。

### 步骤 4：更新文档
- 在 [README.md](README.md) 的 **Custom Features** 章节补充使用说明。
- 如有必要同步更新本文件。

### 步骤 5：提交并推送
```bash
git add <你修改的文件>          # 切勿使用 git add . 误提交 out/、mise.toml 等本地文件
git commit -m "feat: <简要描述新特性>"
git push origin feature/custom
```

> 注意：不要在这里修改 `.github/workflows/`。CI 改动请直接改在 `main` 分支（见第四节）。

---

## 四、GitHub Actions 工作流设计

CI/CD 全部位于 `main` 分支，共两个 workflow。

### 1. `rebase-feature.yml` —— 把 feature 迁移到新的上游 tag

- **触发**：手动 `workflow_dispatch`，输入参数 `tag` = **要迁移到的上游 release tag**。
- **动作**：把 `feature/custom` 从当前基线（`custom-base`）rebase 到该 tag：
  ```
  git rebase --onto <新tag> custom-base   # 在当前 feature/custom 分支上
  ```
  - **无冲突**：运行 `go build ./cmd/{tailscale,tailscaled,derper}` 构建检查 → 自动 `git push --force-with-lease` 更新 `feature/custom`，并把 `custom-base` 更新为新 tag。
  - **有冲突**：Job 失败（红叉），并在该次运行的 **Summary** 中列出：
    - 新/旧基线、卡住的提交（`REBASE_HEAD`）；
    - 冲突文件清单与每个文件的冲突块数（`<<<<<<<` 数量）；
    - 本地处理命令。
    同时上传 `rebase-conflicts` artifact。

### 2. `release-custom.yml` —— 构建并发布

- **触发**：手动 `workflow_dispatch`，输入参数 `tag` = **feature 所基于的上游 tag**。
- **动作**：
  1. `verify` Job：从上游 fetch 该 tag、从 `origin` fetch `custom-base`，**校验两者指向同一 commit**；不一致则报错提示先跑 `rebase-feature.yml`。
  2. `build` / `build-derper` Job：**直接编译 `feature/custom`**（它已经是 `<tag> + 定制`），不再 cherry-pick。
  3. `release` Job：聚合压缩包、生成 `sha256sums.txt`，发布为 `<tag>-custom`。
- **发布命名**：`<tag>-custom`（例如 `v1.104.7-custom`），版本戳与压缩包名都用它。

### 3. workflow 文件的修改规范

> [!IMPORTANT]
> workflow 只在 `main` 分支维护，**不要在 `feature/custom` 上修改 workflow**。否则会与 `main` 上的版本反复冲突，破坏 rebase。

```bash
git checkout main
git fetch upstream && git merge upstream/main   # 可选：同步上游
# 编辑 .github/workflows/*.yml
git add .github/workflows/
git commit -m "ci: <描述对 CI 的改动>"
git push origin main
```

---

## 五、发布自定义新版本的操作步骤

### A. 首次 / 需要换基线时
1. 打开 **Actions → Rebase feature branch onto upstream tag → Run workflow**；
2. 填入目标 **上游 release tag**（如 `v1.104.7`）；
3. 若为绿色，`feature/custom` 与 `custom-base` 已自动更新；
   若为红色（冲突），按第六节 Q1 在本地解决后再继续。

### B. 常规发版
1. 确认最近一次 `Rebase feature branch onto upstream tag` 为绿色；
2. 打开 **Actions → Release Custom Tailscale → Run workflow**，填入与基线一致的 **上游 release tag**；
3. 等待 `verify` → `build`(4 矩阵) + `build-derper` → `release`；
4. 在 **Releases** 页面下载（Release 名为 `<tag>-custom`）：
   - `tailscale_<tag>-custom_linux_amd64.tar.gz`
   - `tailscale_<tag>-custom_linux_arm64.tar.gz`
   - `tailscale_<tag>-custom_darwin_arm64.tar.gz`
   - `tailscale_<tag>-custom_windows_amd64.zip`
   - `derper_<tag>-custom_linux_amd64.tar.gz`
   - `sha256sums.txt`

---

## 六、日常维护常见问题速查

### Q1：`Rebase feature branch onto upstream tag` 报冲突怎么办？
- **现象**：该次运行失败，Summary 中列出 `⚠️ Rebase 冲突`、冲突文件与块数。
- **解决**（本地处理一次）：
  ```bash
  git fetch origin --tags
  git checkout feature/custom
  git rebase --onto <新tag> custom-base
  # 手动解决冲突后
  git add <冲突文件>
  git rebase --continue
  go test ./wgengine/magicsock/...   # 按需跑相关测试
  git push --force-with-lease origin feature/custom
  git tag -f custom-base <新tag>
  git push --force origin refs/tags/custom-base
  ```
- 冲突的根源通常是上游改动了自定义 hook 所在的函数签名，需要按新签名调整定制代码。

### Q2：如何切换到新的上游 release（例如从 v1.104.7 到 v1.105.0）？
直接跑 `rebase-feature.yml` 并填入 `v1.105.0` 即可；有冲突就按 Q1 本地解一次。成功后 `custom-base` 会指向 `v1.105.0`，随后发布 `v1.105.0-custom`。

### Q3：`main` 分支需要跟上游 main 同步吗？
可选。`main` 只用于承载 workflow 和对齐上游 main；它**不影响** `feature/custom` 的基线。同步方式：
```bash
git fetch upstream
git checkout main
git merge upstream/main
git push origin main
```

### Q4：为什么 `feature/custom` 不应包含 workflow 改动？
`main` 已经有最终的 workflow 文件；如果 `feature/custom` 也修改同一文件，每次 rebase 都会在该文件上反复冲突。约定「workflow 归 `main`、代码归 `feature/custom`」后，rebase 只会剩下真正的代码冲突。
