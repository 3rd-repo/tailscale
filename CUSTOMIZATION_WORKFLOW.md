# Tailscale Fork 定制开发与维护工作流指南

本文档系统整理了本 Tailscale Fork 仓库的架构设计、分支策略、自定义特性开发流程、GitHub Actions 自动化同步/构建发布机制及日常维护规范。

---

## 一、分支架构与设计原则

为了兼顾**长期跟进上游（Upstream）更新**与**灵活定制新特性**，本项目采用严格的分支隔离策略：

```text
upstream (tailscale/tailscale)
       │
       ▼ (merge upstream/main)
   ┌─────────┐
   │  main   │ ─── 上游代码 + 仅 GitHub Actions workflow（严禁业务代码侵入）
   └─────────┘
       │
       ▼ (自动 rebase：rebase-feature.yml)
┌────────────────┐
│ feature/custom │ ─── 全部自定义功能代码 + 文档（不含任何 workflow）
└────────────────┘
```

### 1. `main` 分支（主分支）
- **定位**：对齐官方上游基线的集成主干，并承载本仓库的 CI/CD。
- **原则**：
  - **绝不合并任何自定义业务代码**（如 `force-derp`、`no-logs-no-support` 等）。
  - **只允许 GitHub Actions workflow 存在于此分支**（当前为 `release-custom.yml` 与 `rebase-feature.yml`）。
  - 与上游同步的方式是 **merge**（`git merge upstream/main`），而不是 fast-forward（因为本分支含有 workflow 提交）。这一点必须在 merge 后推送，从而触发 `rebase-feature.yml` 自动更新 `feature/custom`。

### 2. `feature/custom` 分支（定制特性分支）
- **定位**：所有自定义功能、新特性、定制文档的开发与维护分支。
- **原则**：
  - 所有新特性（Go 源代码改动）在此分支开发、测试、提交。
  - 新特性的使用方法与说明，同步更新到本分支的 [README.md](README.md) 中。
  - **禁止**在此分支修改 `.github/workflows/` 下的任何文件；workflow 一律在 `main` 分支直接维护。
  - 该分支始终保持在 `main` 之上（linear），由 `rebase-feature.yml` 自动 rebase；出现冲突时需在本地手动解决。

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

当需要向 Tailscale 增加新功能时，请按以下步骤执行：

### 步骤 1：检出并确保位于 `feature/custom` 分支
```bash
git checkout feature/custom
git pull origin feature/custom
```

### 步骤 2：编写功能代码与本地编译测试
- 编写或修改 Go 代码。
- 在本地运行单元测试：
  ```bash
  go test ./cmd/tailscaled/... ./cmd/tailscale/cli/...
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

### 步骤 4：更新文档
- 在 [README.md](README.md) 的 **Custom Features** 章节下增加对应新特性的使用说明、命令示例及配置项说明。
- 如有必要，同步更新本文件（`CUSTOMIZATION_WORKFLOW.md`）。

### 步骤 5：提交并推送到定制分支
```bash
git add <你修改的文件>          # 切勿使用 git add . 误提交 out/、mise.toml 等本地文件
git commit -m "feat: <简要描述新特性>"
git push origin feature/custom
```

> 注意：不要在这里修改 `.github/workflows/`。CI 改动请直接改在 `main` 分支（见第四节）。

---

## 四、GitHub Actions 工作流设计

本仓库的 CI/CD 全部位于 `main` 分支，共两个 workflow：

### 1. `rebase-feature.yml` —— 保持 feature 与上游同步

- **触发**：`push` 到 `main`（即每次与上游同步后），也可手动 `workflow_dispatch`。
- **动作**：把 `feature/custom` rebase 到最新的 `main`。
  - **无冲突**：运行 `go build ./cmd/{tailscale,tailscaled,derper}` 做构建检查 → 自动 `git push --force-with-lease` 更新 `feature/custom`。
  - **有冲突**：Job 失败（红叉），并在该次运行的 **Summary** 中列出：
    - 冲突文件清单；
    - 每个文件的冲突块数（`<<<<<<<` 数量）；
    - 卡住的提交（`REBASE_HEAD`）；
    - 本地处理命令。
    同时上传 `rebase-conflicts` artifact。你在本地解完冲突后 `git push --force-with-lease origin feature/custom` 即可。

### 2. `release-custom.yml` —— 构建并发布自定义版本

- **触发**：手动 `workflow_dispatch`。
- **输入**：只需一个 `tag` —— **上游的 Release tag**（例如 `v1.104.7`）。
- **动作**：构建 `<上游 tag> + 定制`：
  1. 从上游 `git fetch --force` 该 tag（强制覆盖本地同名 tag，避免 fork 里 tag 错位/冲突）；
  2. 从 `origin` 拉取 `main` 与 `feature/custom`，用 `git merge-base(main, feature/custom)..feature/custom` 取出**定制提交**；
  3. checkout 该上游 tag，把定制提交 `git cherry-pick -x` 到其上（即 `<tag> + 定制`）；
  4. 再让 `build` / `build-derper` 矩阵编译。
- **发布命名**：自动派生为 `<tag>-custom`（例如 `v1.104.7-custom`），版本戳与压缩包名都用它。
  - **`build` Job**：矩阵并行交叉编译 `linux/amd64`、`linux/arm64`、`darwin/arm64`、`windows/amd64` 的 `tailscale` 与 `tailscaled`。
  - **`build-derper` Job**：独立编译 `linux/amd64` 的 `cmd/derper`。
  - **`release` Job**：聚合所有平台的压缩包，生成 `sha256sums.txt`，通过 `gh release create` / `gh release upload --clobber` 发布至 GitHub Releases。
- **注意**：定制提交是基于 `main` 计算的；如果所选上游 tag 与自定义代码差异过大，`cherry-pick` 会失败（Job 报错）。此时需在本地验证/调整，或改选与 `feature/custom` 基线更接近的 tag。

### 3. workflow 文件的修改规范

> [!IMPORTANT]
> workflow 只在 `main` 分支维护，**不要在 `feature/custom` 上修改 workflow**。否则会与 `main` 上的版本反复冲突，破坏自动 rebase。

在 `main` 分支修改 CI 的标准流程：
```bash
# 1. 切到 main 并同步（merge 上游）
git checkout main
git fetch upstream
git merge upstream/main

# 2. 直接编辑 .github/workflows/*.yml
#    （例如新增/调整 job、修改输入参数等）

# 3. 提交并推送，推送后会自动触发 rebase-feature.yml
git add .github/workflows/
git commit -m "ci: <描述对 CI 的改动>"
git push origin main
```

---

## 五、发布自定义新版本的操作步骤

1. **确认 `feature/custom` 已是最新**
   - 查看最近一次 `Rebase feature/custom onto main` 是否为绿色；
   - 若有冲突，先按第四节说明在本地解决并推送。
2. **触发构建**：
   - 打开 GitHub 仓库 **Actions** 标签页；
   - 左侧选择 **Release Custom Tailscale**；
   - 点击 **Run workflow** 并在 `tag` 填入 **上游 Release tag**（例如 `v1.104.7`）；
   - 点击 **Run workflow** 启动构建。
3. **等待流水线执行**：
   - `build (4 矩阵)` 与 `build-derper (1 独立任务)` 完全并行；
   - `release` 任务汇合打包并生成 SHA256 哈希。
4. **下载安装包**：
   - 访问 GitHub 仓库 **Releases** 页面（Release 名为 `<tag>-custom`）：
     - `tailscale_<tag>-custom_linux_amd64.tar.gz`
     - `tailscale_<tag>-custom_linux_arm64.tar.gz`
     - `tailscale_<tag>-custom_darwin_arm64.tar.gz`
     - `tailscale_<tag>-custom_windows_amd64.zip`
     - `derper_<tag>-custom_linux_amd64.tar.gz`
     - `sha256sums.txt`

---

## 六、日常维护常见问题速查

### Q1：`Rebase feature/custom onto main` 报冲突怎么办？
- **现象**：该次运行失败，Summary 中列出 `⚠️ Rebase 冲突`、冲突文件与块数。
- **解决**：在本地处理一次即可：
  ```bash
  git fetch origin
  git checkout feature/custom
  git rebase origin/main
  # 手动解决冲突后
  git add <冲突文件>
  git rebase --continue
  go test ./wgengine/magicsock/...   # 按需跑相关测试
  git push --force-with-lease origin feature/custom
  ```
- 之后除非上游再次改动到相同代码，否则 rebase 会自动通过并自动推送。

### Q2：如何保持 `main` 分支与官方 Upstream 同步？
`main` 含有 workflow 提交，因此**不能用 Sync fork 的 fast-forward**，要使用 merge：
```bash
git remote add upstream https://github.com/tailscale/tailscale.git || true
git fetch upstream
git checkout main
git merge upstream/main          # 有冲突则解决
git push origin main             # 推送后自动触发 rebase-feature.yml
```

### Q3：为什么 `feature/custom` 不应包含 workflow 改动？
`main` 已经有最终的 workflow 文件；如果 `feature/custom` 也修改同一文件，每次 rebase 都会在该文件上反复冲突。约定「workflow 归 `main`、代码归 `feature/custom`」后，rebase 只会剩下真正的代码冲突。
