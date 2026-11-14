# ReleaseFloor

[![CI](https://github.com/feiiiiii5/releasefloor/actions/workflows/ci.yml/badge.svg)](https://github.com/feiiiiii5/releasefloor/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/feiiiiii5/releasefloor?include_prereleases&display_name=tag)](https://github.com/feiiiiii5/releasefloor/releases/tag/v0.1.0)
[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)](#)

[English](README.md) · [精确行为约定 (Contract)](docs/CONTRACT.md) · [AI Agent 指南](docs/AGENT.md) · [产品决策与竞品分析](docs/PRODUCT.md) · [验证记录](docs/VALIDATION.md)

**静态检查 ELF / Mach-O 发布产物记录的运行时要求，不执行被检查的程序。**  
在发布前检查 **GLIBC 版本上升**、**macOS 最低系统版本上调**和**架构切片遗漏**，并报告新增动态依赖供审查。

---

## 🎯 适用人群（谁需要 ReleaseFloor？）

ReleaseFloor 为所有从事原生编译型软件发布、分发或自动化运维的开发者与系统设计：

| 用户画像 | 典型痛点 | ReleaseFloor 如何解决 |
| :--- | :--- | :--- |
| **CLI / 基础工具作者**<br>*(Go, Rust, C/C++, Zig, Swift)* | 在较新的 CI 构建机编译出的二进制，分发到旧版 Linux（如 CentOS 7, Debian 10, Ubuntu 20.04）上直接报错无法运行。 | 发布前精准提取记录的 GLIBC/GLIBCXX 及 macOS 最低系统要求，守住最低兼容底线。 |
| **DevOps 与 Release 工程师**<br>*(CI/CD 流水线维护者)* | GitHub Actions 镜像（如 `ubuntu-latest`）或 Xcode 悄悄升级，导致构建环境静默漂移，破坏整个发布矩阵。 | 提供 CI 检查与明确的退出码：0 通过、1 阻断、2 命令或输入错误。 |
| **包管理器与 Wheel 维护者**<br>*(Python Wheels & Java JNI)* | 打包带 C 扩展的 Python `.whl`（如 manylinux）或内嵌 JNI 原生动态库的 `.jar` 时，底层依赖意外超标。 | 直接解析 `.whl` 与 `.jar` 压缩包内部的 `.so` / `.dylib` 元数据，无需解压至磁盘。 |
| **自主式 AI 编码 Agent**<br>*(Claude Code, Antigravity, Cursor)* | 工具链调整可能提高产物记录的库版本或 macOS 最低版本要求。 | 通过 MCP 工具和 JSON 报告，让 Agent 的构建流程读取运行时要求。 |

---

## 💡 使用场景

以下为示意流程；实际检查过的产物与命令见[验证记录](docs/VALIDATION.md)。

### 场景 1：CI 构建机升级引发的“静默 GLIBC 爬升”
* **事故发生**：CI 镜像或工具链升级，改变了链接时使用的库。
* **灾难后果**：你的 Go/Rust/C 程序编译成功（Exit Code 0），但底层自动链接了构建机上的 `libc.so.6 (GLIBC_2.38)`。当用户在 Debian 11 或 RHEL 8 上执行时，终端立刻报错崩溃：  
  `./mytool: /lib/x86_64-linux-gnu/libc.so.6: version 'GLIBC_2.38' not found`
* **ReleaseFloor 解决**：在 CI 发布前执行 `releasefloor compare old-v1.0.tar.gz new-v1.1.tar.gz`，发现 GLIBC 底线从 `2.28` 异常爬升至 `2.38`，立即阻断发布（Exit Code 1）。

### 场景 2：macOS Deployment Target 悄悄上浮
* **事故发生**：本地开发机或 CI macOS Runner 升级了 Xcode，导致 `-mmacosx-version-min` 或 SDK 默认值从 macOS 11.0 (Big Sur) 变成 14.0 (Sonoma)。
* **灾难后果**：运行 macOS Monterey 或 Ventura 的旧版 Mac 用户双击或在终端启动程序时，系统直接弹窗拒绝运行。
* **ReleaseFloor 解决**：通过 `releasefloor check --policy policy.json dist/*.zip`，精准命中 `policy_os_exceeded` 并拦截发布。

### 场景 3：通用二进制（Universal Binary）遗漏架构切片
* **事故发生**：构建脚本异常或打包配置错误，导致打出的 macOS 通用二进制意外漏掉了 `arm64` 切片，仅包含 `x86_64`。
* **灾难后果**：Apple Silicon 用户失去原生 arm64 版本；能否经 Rosetta 运行取决于环境和程序。
* **ReleaseFloor 解决**：`check` 严格校验规则约定的目标架构矩阵；`compare` 明确将缺失切片标记为 `removed_binary` 阻断发布。

### 场景 4：意外引入第三方动态库依赖
* **事故发生**：代码引入了一个新库，或构建参数漏传了 `-static` / `CGO_ENABLED=0`，导致原本宣称纯静态的程序悄悄动态链接了 `libssl.so.3` 或 `libstdc++.so.6`。
* **灾难后果**：目标生产环境没有安装特定版本的动态链接库，软件无法启动。
* **ReleaseFloor 解决**：静态提取可执行文件全部动态导入项（DT_NEEDED / LC_LOAD_DYLIB），直接发现并报告新增动态依赖。

### 场景 5：AI Agent 闭环发布自愈
* **事故发生**：AI Coding Agent 负责编写代码、跨平台构建并打 Tag 发布，缺乏真机矩阵验证兼容性。
* **ReleaseFloor 解决**：Agent 通过 MCP 调用 `releasefloor_check`，读取版本要求，调整相应构建配置并检查重新生成的产物。检查通过表示记录的元数据满足策略；目标系统运行仍需单独验证。

---

## ⚡ 快速上手

### 第一步：安装本文对应的源码版本

下文命令对应 2026 年 11 月 11 日的源码提交 `bf469b35a0fe4a1809b8073c9499bc464ec61419`。该版本在 v0.1.0 之后加入 MCP、`init-policy`、wheel/JAR 输入支持及组合式 GitHub Action。

使用 Go 1.25 或更新版本安装：
```sh
go install github.com/feiiiiii5/releasefloor/cmd/releasefloor@bf469b35a0fe4a1809b8073c9499bc464ec61419
releasefloor help
```

[v0.1.0 预发布页](https://github.com/feiiiiii5/releasefloor/releases/tag/v0.1.0)提供 Linux、macOS、Windows 预编译命令。该发布版有 `scan`、`compare`、`check` 和 `version`，不含上述后来加入的功能。新源码的 `version` 仍输出 `0.1.0`，因此用源码提交识别本构建。Windows 是扫描器运行平台；尚不支持检查 PE 输入。

---

### 第二步：审查发布产物 (`scan`)

扫描包含 ELF / Mach-O 的归档包（`.tar.gz`、`.zip`）、Python Wheel（`.whl`）、Java JAR（`.jar`）或可执行文件：

```sh
releasefloor scan dist/myapp-linux-amd64.tar.gz
```

**示意输出（数值简化）：**
```
releasefloor 0.1.0 · pass

artifact: myapp-linux-amd64.tar.gz
sha256:   <artifact SHA-256>
coverage: complete (4 regular entries, 1 runtime binary, 3 ignored files)

  bin/myapp (elf, amd64, executable)
    floors:       GLIBC 2.28, GLIBCXX 3.4.28
    libraries:    libc.so.6, libstdc++.so.6
```

需要输出为 JSON 或完全离线的自包含 HTML 审计报告？
```sh
# 输出机器可读 JSON，供自动化脚本或 Agent 处理
releasefloor scan dist/myapp-linux-amd64.tar.gz --format json

# 输出严格安全策略（Strict CSP）的离线 HTML 报告
releasefloor scan dist/myapp-linux-amd64.tar.gz --format html --output report.html
```

---

### 第三步：比对两版发布产物 (`compare`)

比对新构建的候选包与上一个已知稳定的历史版本，防止兼容性底线上浮倒退：

```sh
releasefloor compare releases/v1.0.0.tar.gz dist/v1.1.0.tar.gz
```

* **通过 (Exit Code 0)**：没有 error 级别的问题。新增二进制为信息项；新增直接依赖库为 warning，供审查。
* **阻断 (Exit Code 1)**：受检查的版本要求或 macOS 最低版本提高、二进制或切片缺失，以及覆盖不完整或未知要求会产生 error。完整规则见[行为约定](docs/CONTRACT.md)。

---

### 第四步：生成初始策略并集成到 CI (`init-policy` + `check`)

根据基线产物生成初始策略，再审阅版本上限和必需目标，随后用于 CI：

```sh
# 1. 根据基线产物生成初始策略
mkdir -p .github
releasefloor init-policy --output .github/releasefloor-policy.json dist/*.tar.gz dist/*.zip

# 2. 在 CI 中使用策略文件对所有候选包进行门禁裁决
releasefloor check --policy .github/releasefloor-policy.json dist/*.tar.gz dist/*.zip
```

**生成的 `.github/releasefloor-policy.json` 示例：**
```json
{
  "schema": "releasefloor.policy.v1",
  "rules": [
    {"platform": "elf", "arch": "amd64", "max_floors": {"GLIBC": "2.28"}},
    {"platform": "elf", "arch": "arm64", "max_floors": {"GLIBC": "2.28"}},
    {"platform": "macos", "arch": "amd64", "max_os": "11.0"},
    {"platform": "macos", "arch": "arm64", "max_os": "11.0"}
  ]
}
```

---

### 第五步：接入你的 AI 编程助手（MCP Server 模式）

ReleaseFloor 针对 Agent 时代设计，内置零依赖的 Model Context Protocol（MCP）标准协议服务。

在 Claude Desktop（`claude_desktop_config.json`）或 Cursor（`mcp.json`）中直接添加配置：

```json
{
  "mcpServers": {
    "releasefloor": {
      "command": "releasefloor",
      "args": ["mcp"]
    }
  }
}
```

你的 AI Agent 即可直接调用以下 4 大工具：
- `releasefloor_scan`：查看受支持的 ELF / Mach-O 产物及归档包的依赖元数据。
- `releasefloor_compare`：比对新旧两版发布包的兼容性差异。
- `releasefloor_check`：根据发布策略判定矩阵是否合规。
- `releasefloor_init_policy`：一键自动生成策略文件。

---

## 🚀 GitHub Actions 持续集成接入

配置 Go 环境，在构建完成后加入检查：

```yaml
name: Release
on:
  push:
    tags: ['v*']

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25.x'
          cache: false

      # 1. 编译各平台跨平台产物至 dist/ 目录
      # ... (例如 goreleaser, cargo-dist, cmake 或 wheels 构建)

      # 2. 在上传 GitHub Releases 资产前执行门禁拦截
      - name: Gate release runtime requirements
        uses: feiiiiii5/releasefloor@bf469b35a0fe4a1809b8073c9499bc464ec61419
        with:
          version: bf469b35a0fe4a1809b8073c9499bc464ec61419
          command: check
          args: --policy .github/releasefloor-policy.json dist/*.tar.gz dist/*.zip
```

---

## 🌐 语言与生态覆盖

ReleaseFloor 直接读取 ELF 与 Mach-O 元数据，不依赖源代码使用的编程语言：

* **C & C++**：独立二进制、共享库（`.so`, `.dylib`）与插件系统。专门为 `GLIBC`、`GLIBCXX` 与 `CXXABI` 版本门禁提供原生级精准提取。
* **Go & Rust**：单二进制 CLI、纯静态构建（`musl` / `CGO_ENABLED=0`）、交叉编译产物以及附带的系统动态依赖。
* **Python（原生二进制 Wheel）**：原生检查二进制 `.whl`（如 `manylinux` 与 `macosx` 轮子）内部打包的编译扩展（`.so`, `.dylib`），无需在磁盘解压。
* **Java / JVM（JNI 库与原生镜像）**：原生检查 `.jar` 归档中随包分发的 JNI 动态库，或 GraalVM Native Image 提前编译（AOT）生成的本地原生二进制。
* **Zig & Swift**：Linux 与 macOS 上的跨平台原生可执行文件与动态 Framework。

支持的输入格式：`.tar.gz`、`.tgz`、`.zip`、Python `.whl`、Java `.jar` 以及裸二进制文件。

---

## 📊 当前实现的功能

| 功能 | 本文源码版本的范围 |
| --- | --- |
| 归档扫描 | ELF/Mach-O 原文件、`.tar.gz`、`.tgz`、`.zip`、`.whl` 和 `.jar`；不向磁盘解压 |
| 架构记录 | 分别记录受检查的 Universal Mach-O 各切片 |
| 版本比较 | 按成员路径、格式、平台和架构匹配，报告运行时要求变化 |
| 策略检查 | GLIBC、GLIBCXX、CXXABI、macOS 最低版本上限及明确要求的目标 |
| 初始策略 | `init-policy` 从给定产物生成待审阅的版本上限 |
| Agent 接口 | JSON 报告及 stdio MCP 服务 |
| 报告 | 文本、JSON 和自包含 HTML |

替代工具与本项目面向的工作流程见[产品决策](docs/PRODUCT.md)。

---

## 🔒 安全性与设计边界

* **不执行目标程序**：静态解析被检查产物的元数据。
* **Go 标准库实现**：扫描器无第三方 Go 依赖，不使用 CGo。
* **从不静默覆盖已有文件**：`--output` 严格要求目标文件不存在（`O_CREATE | O_EXCL`），坚决不覆盖输入包或已有报告。
* **归档读取边界**：输入大小、单成员大小、展开字节数和文件数均设有上限；拒绝不安全路径，不向磁盘解压归档内容。

---

## 📖 CLI 命令与帮助参考

```
Usage: releasefloor <command> [options] [arguments...]

Commands:
  scan         Inspect runtime requirements of an archive or binary
  compare      Compare runtime requirements between baseline and candidate builds
  check        Verify release artifacts against an explicit policy file
  init-policy  Synthesize a starter policy JSON from baseline artifacts
  mcp          Run as a Model Context Protocol (MCP) server over stdio
  version      Print version information
  help         Show help for ReleaseFloor or a specific command
```

查看各子命令的详细用法与参数示例：
```sh
releasefloor help scan
releasefloor help compare
releasefloor help check
releasefloor help init-policy
```

---

## 开源许可

ReleaseFloor 基于 [MIT License](LICENSE) 协议完全开源。
