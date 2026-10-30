# Tailscale Fork 定制开发与维护工作流指南

本文档系统整理了本 Tailscale Fork 仓库的架构设计、分支策略、自定义特性开发流程、GitHub Actions 构建发布机制及日常维护规范。

---

## 一、分支架构与设计原则

为了兼顾**长期跟进上游（Upstream）更新**与**灵活定制新特性**，本项目采用严格的分支隔离策略：

```text
upstream (tailscale/tailscale)
       │
       ▼ (Sync Fork / Git Fetch)
   ┌─────────┐
   │  main   │ ─── 纯净上游代码 + 唯一的 Release Action 提交（严禁业务代码侵入）
   └─────────┘
       │
       ▼ (Feature Branch)
┌────────────────┐
│ feature/custom │ ─── 所有自定义功能代码 + 分支 README 文档 + Release Action
└────────────────┘
```

### 1. `main` 分支（主分支）
- **定位**：对齐官方上游最新基线的纯净主干。
- **原则**：
  - **绝不合并任何自定义业务代码**（如 `force-derp`、`no-logs-no-support` 等）。
  - **有且仅有一个提交**用于添加/更新 `.github/workflows/release-custom.yml`。
  - 确保随时可以使用 GitHub 的 **Sync fork** 按钮与官方上游主干同步，绝不产生代码合并冲突。

### 2. `feature/custom` 分支（定制特性分支）
- **定位**：所有自定义功能、新特性、定制文档的开发与维护分支。
- **原则**：
  - 所有新特性（Go 源代码改动）在此分支开发、测试、提交。
  - 新特性的使用方法与说明，同步更新到本分支的 [README.md](file:///root/code/test/tailscale/README.md) 中。
  - GitHub Actions 构建工作流首先在此分支编写与验证，确认无误后再同步回 `main` 分支。

---

## 二、当前已集成的自定义特性

| 特性名称 | 作用说明 | 涉及主要文件 |
| :--- | :--- | :--- |
| **强制指定 DERP 中继** | 增加 `tailscale debug force-derp <region-id>` 子命令，强制锁定流量使用指定区域 DERP | `cmd/tailscale/cli/debug.go` |
| **默认免日志与免遥测** | `tailscaled` 默认开启 `--no-logs-no-support=true`，无需每次手动加参即可避免上传遥测日志 | `cmd/tailscaled/tailscaled.go` |
| **显示隐藏守护进程子命令** | `tailscaled --help` 显式列出并说明 `install-system-daemon`、`uninstall-system-daemon` 和 `debug` | `cmd/tailscaled/tailscaled.go` |
| **服务安装支持自定义启动参数** | `install-system-daemon [flags]` 允许透传 `-statedir`、`-state` 等参数并持久化注册到系统服务 | `cmd/tailscaled/install_*.go` |
| **Linux 原生 systemd 支持** | 为 Linux 平台增加 `install-system-daemon` 与 `uninstall-system-daemon`，自动注册、配置并启动 systemd 服务 | `cmd/tailscaled/install_linux.go` |
| **独立 DERP 构建流水线** | Release Workflow 独立并行编译并打包 Linux x86 (`linux/amd64`) 的 `derper` 中继服务端 | `.github/workflows/release-custom.yml` |

---

## 三、开发新特性的标准流程

当需要向 Tailscale 增加新功能时，请严格按以下 5 个步骤执行：

### 步骤 1：检出并确保位于 `feature/custom` 分支
```bash
git checkout feature/custom
git pull origin feature/custom
```

### 步骤 2：编写功能代码与本地编译测试
- 编写或修改 Go 代码。
- 在本地运行单元测试：
  ```bash
  go test -v ./cmd/tailscaled/...
  go test -v ./cmd/tailscale/...
  ```
- 进行跨平台矩阵编译验证（确保没有引入平台特定的构建破坏）：
  ```bash
  # Linux (amd64 / arm64)
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/tailscaled
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/tailscaled

  # macOS (arm64)
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /dev/null ./cmd/tailscaled

  # Windows (amd64)
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/tailscaled
  ```

### 步骤 3：同步编写单元测试
- 为新功能编写完备的自动化测试（例如新增 `xxx_test.go`），防止后续上游代码更新引发回归问题。

### 步骤 4：更新 `feature/custom` 分支的 README.md
- 在 [README.md](file:///root/code/test/tailscale/README.md) 的 **Custom Features** 章节下增加对应新特性的使用说明、命令示例及配置项说明。

### 步骤 5：提交并推送到定制分支
```bash
git add .
git commit -m "feat: <简要描述新特性>"
git push origin feature/custom
```

---

## 四、GitHub Actions 构建工作流设计与同步规范

本项目通过 [.github/workflows/release-custom.yml](file:///root/code/test/tailscale/.github/workflows/release-custom.yml) 实现全自动化的跨平台二进制构建与发布。

### 1. 构建流水线运行原理
1. **纯净基线切出**：工作流通过 `workflow_dispatch` 接收目标官方 Tag（如 `v1.104.4`）。
2. **强制精准抓取 Tag**：使用 `git fetch --force upstream "refs/tags/${BASE_TAG}:refs/tags/${BASE_TAG}"`，仅抓取目标 Release Tag，避免全量 fetch 触发历史标签冲突（`would clobber existing tag`）。
3. **动态应用定制特性**：
   - 自动计算 `feature/custom` 相对于上游基线的所有增量提交：
     ```bash
     BASE=$(git merge-base origin/main origin/feature/custom)
     COMMITS=$(git rev-list --reverse "${BASE}..origin/feature/custom")
     ```
   - 将定制特性的 commits 自动 Cherry-pick 到目标纯净 Tag 之上，即刻生成带有自定义功能的源码树。
4. **并行并行构建**：
   - **`build` Job**：矩阵并行交叉编译 `linux/amd64`、`linux/arm64`、`darwin/arm64`、`windows/amd64` 的 `tailscale` 与 `tailscaled`。
   - **`build-derper` Job**：独立并行编译 `linux/amd64` 的 `cmd/derper` 服务端。
5. **发布阶段 (`release` Job)**：
   - 下载并聚合所有平台的压缩包。
   - 自动生成 `sha256sums.txt` 校验和。
   - 通过 `gh release create` / `gh release upload --clobber` 自动发布至 GitHub Releases。

### 2. 工作流文件（Workflow）的修改与同步规范

> [!IMPORTANT]
> **黄金法则**：所有对 `.github/workflows/release-custom.yml` 的新增或修改，**必须先在 `feature/custom` 分支完成**，测试确认后，再**同步并压缩为单个 Commit 提交到 `main` 分支**。

#### 同步到 `main` 分支的操作指南：
```bash
# 1. 切换到主分支
git checkout main
git pull origin main

# 2. 从 feature/custom 检出最新的工作流文件
git checkout feature/custom -- .github/workflows/release-custom.yml

# 3. 确保 main 分支只包含 upstream 基线 + 该 Action 文件的 1 条干净提交
# (若已有旧的 action 提交，可使用 git reset --soft 对齐 upstream，随后统一提交)
git add .github/workflows/release-custom.yml
git commit -m "ci: add release-custom workflow"

# 4. 推送到远程主分支
git push --force origin main

# 5. 切回特性分支继续日常开发
git checkout feature/custom
```

---

## 五、发布自定义新版本的操作步骤

当官方发布了新的 Release（例如 `v1.104.5`）或者你在 `feature/custom` 分支中增加了新功能并希望重新打包时：

1. **触发构建**：
   - 打开 GitHub 仓库页面，点击 **Actions** 标签页。
   - 在左侧选择 **Release Custom Tailscale** 工作流。
   - 点击右侧 **Run workflow** 下拉框：
     - **Branch**：选择 `main`（推荐）或 `feature/custom`。
     - **Target Upstream Release Tag**：输入目标官方版本号，例如 `v1.104.4`。
   - 点击 **Run workflow** 按钮启动构建。
2. **等待流水线执行**：
   - `build (4 矩阵)` 与 `build-derper (1 独立任务)` 将完全并行执行。
   - 编译完成后由 `release` 任务汇合打包并生成 SHA256 哈希。
3. **下载安装包**：
   - 访问 GitHub 仓库的 **Releases** 页面即可下载预编译的包：
     - `tailscale_<tag>_linux_amd64.tar.gz`
     - `tailscale_<tag>_linux_arm64.tar.gz`
     - `tailscale_<tag>_darwin_arm64.tar.gz`
     - `tailscale_<tag>_windows_amd64.zip`
     - `derper_<tag>_linux_amd64.tar.gz`
     - `sha256sums.txt`

---

## 六、日常维护常见问题速查

### Q1：GitHub Actions 报错 `! [rejected] vX.X.X -> vX.X.X (would clobber existing tag)`
- **原因**：Fork 仓库本地的历史标签与官方 upstream 同名标签指向的 commit 哈希不一致。Git 在执行非强制全量 fetch 时会保护并报错。
- **解决**：当前工作流已升级为 `git fetch --force upstream refs/tags/${BASE_TAG}:refs/tags/${BASE_TAG}`，仅按需强制抓取目标 Tag，从根源杜绝了此问题。若需要校准本地所有标签，可执行：
  ```bash
  git fetch --force --tags https://github.com/tailscale/tailscale.git
  git push --force --tags origin
  ```

### Q2：如何保持 `main` 分支与官方 Upstream 同步？
1. 在 GitHub 网页点击 **Sync fork** -> **Update branch**。
2. 或在本地配置官方 upstream：
   ```bash
   git remote add upstream https://github.com/tailscale/tailscale.git || true
   git checkout main
   git fetch upstream main
   git reset --hard upstream/main
   git checkout feature/custom -- .github/workflows/release-custom.yml
   git add .github/workflows/release-custom.yml
   git commit -m "ci: add release-custom workflow"
   git push --force origin main
   ```
