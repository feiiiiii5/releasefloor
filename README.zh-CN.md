# ReleaseFloor

[![CI](https://github.com/feiiiiii5/releasefloor/actions/workflows/ci.yml/badge.svg)](https://github.com/feiiiiii5/releasefloor/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/feiiiiii5/releasefloor?display_name=tag)](https://github.com/feiiiiii5/releasefloor/releases/latest)
[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)](#)

[English](README.md) · [精确行为约定 (Contract)](docs/CONTRACT.md) · [AI Agent 指南](docs/AGENT.md) · [产品决策与竞品分析](docs/PRODUCT.md) · [验证记录](docs/VALIDATION.md)

**零执行开销的原生二进制（ELF / Mach-O）发布前门禁工具。**  
在正式发布前，自动拦截编译归档包中意外发生的 **GLIBC 版本爬升**、**macOS 最低系统版本上浮**、**意外引入动态依赖** 以及 **多架构切片漏打** 等兼容性事故。

---

## 🎯 适用人群（谁需要 ReleaseFloor？）

ReleaseFloor 为所有从事原生编译型软件发布、分发或自动化运维的开发者与系统设计：

| 用户画像 | 典型痛点 | ReleaseFloor 如何解决 |
| :--- | :--- | :--- |
| **CLI / 基础工具作者**<br>*(Go, Rust, C/C++, Zig, Swift)* | 在较新的 CI 构建机编译出的二进制，分发到旧版 Linux（如 CentOS 7, Debian 10, Ubuntu 20.04）上直接报错无法运行。 | 发布前精准提取记录的 GLIBC/GLIBCXX 及 macOS 最低系统要求，守住最低兼容底线。 |
| **DevOps 与 Release 工程师**<br>*(CI/CD 流水线维护者)* | GitHub Actions 镜像（如 `ubuntu-latest`）或 Xcode 悄悄升级，导致构建环境静默漂移，破坏整个发布矩阵。 | 提供 3 行 YAML 的轻量级 CI 快速失败门禁，Exit Code 语义清晰（0 成功，1 阻断）。 |
| **包管理器与 Wheel 维护者**<br>*(Python Wheels & Java JNI)* | 打包带 C 扩展的 Python `.whl`（如 manylinux）或内嵌 JNI 原生动态库的 `.jar` 时，底层依赖意外超标。 | 直接解析 `.whl` 与 `.jar` 压缩包内部的 `.so` / `.dylib` 元数据，无需解压至磁盘。 |
| **自主式 AI 编码 Agent**<br>*(Claude Code, Antigravity, Cursor)* | Agent 自动执行交叉编译或工具链升级时，无法感知旧内核与系统版本的兼容性破坏。 | 原生内置零依赖 MCP（Model Context Protocol）服务，输出确定性 JSON 驱动自动自愈。 |

---

## 💡 使用场景（什么时候用 ReleaseFloor？）

### 场景 1：CI 构建机升级引发的“静默 GLIBC 爬升”
* **事故发生**：GitHub Actions 将 `ubuntu-latest` 从 Ubuntu 22.04 升级到 Ubuntu 24.04。
* **灾难后果**：你的 Go/Rust/C 程序编译成功（Exit Code 0），但底层自动链接了构建机上的 `libc.so.6 (GLIBC_2.38)`。当用户在 Debian 11 或 RHEL 8 上执行时，终端立刻报错崩溃：  
  `./mytool: /lib/x86_64-linux-gnu/libc.so.6: version 'GLIBC_2.38' not found`
* **ReleaseFloor 解决**：在 CI 发布前执行 `releasefloor compare old-v1.0.tar.gz new-v1.1.tar.gz`，毫秒级发现 GLIBC 底线从 `2.28` 异常爬升至 `2.38`，立即阻断发布（Exit Code 1）。

### 场景 2：macOS Deployment Target 悄悄上浮
* **事故发生**：本地开发机或 CI macOS Runner 升级了 Xcode，导致 `-mmacosx-version-min` 或 SDK 默认值从 macOS 11.0 (Big Sur) 变成 14.0 (Sonoma)。
* **灾难后果**：运行 macOS Monterey 或 Ventura 的旧版 Mac 用户双击或在终端启动程序时，系统直接弹窗拒绝运行。
* **ReleaseFloor 解决**：通过 `releasefloor check --policy policy.json dist/*.zip`，精准命中 `policy_os_exceeded` 并拦截发布。

### 场景 3：通用二进制（Universal Binary）遗漏架构切片
* **事故发生**：构建脚本异常或打包配置错误，导致打出的 macOS 通用二进制意外漏掉了 `arm64` 切片，仅包含 `x86_64`。
* **灾难后果**：所有使用 Apple Silicon（M1/M2/M3/M4）芯片的用户只能靠 Rosetta 2 转译低效运行，甚至直接报错崩溃。
* **ReleaseFloor 解决**：`check` 严格校验规则约定的目标架构矩阵；`compare` 明确将缺失切片标记为 `removed_binary` 阻断发布。

### 场景 4：意外引入第三方动态库依赖
* **事故发生**：代码引入了一个新库，或构建参数漏传了 `-static` / `CGO_ENABLED=0`，导致原本宣称纯静态的程序悄悄动态链接了 `libssl.so.3` 或 `libstdc++.so.6`。
* **灾难后果**：目标生产环境没有安装特定版本的动态链接库，软件无法启动。
* **ReleaseFloor 解决**：静态提取可执行文件全部动态导入项（DT_NEEDED / LC_LOAD_DYLIB），直接发现并报告新增动态依赖。

### 场景 5：AI Agent 闭环发布自愈
* **事故发生**：AI Coding Agent 负责编写代码、跨平台构建并打 Tag 发布，缺乏真机矩阵验证兼容性。
* **ReleaseFloor 解决**：Agent 通过内置 MCP Server 调用 `releasefloor_check`，拿到结构化 JSON 诊断（如 `GLIBC: 2.38 exceeds 2.28`），自动定位并修正编译参数（如切换 musl 目标或增加编译器约束），再度校验通过后自动完成发布。

---

## ⚡ 3 分钟极速上手教程

### 第一步：安装 ReleaseFloor（30 秒）

**方式一：使用 Go 一键安装 (Go 1.25+):**
```sh
go install github.com/feiiiiii5/releasefloor/cmd/releasefloor@v0.1.0
```

**方式二：下载开箱即用的预编译二进制:**  
前往 [GitHub Releases 页面](https://github.com/feiiiiii5/releasefloor/releases/latest) 下载适合你系统（Linux / macOS / Windows）的压缩包，解压后将 `releasefloor` 放置于系统 `PATH` 即可。

```sh
# 验证安装是否成功
releasefloor version
```

---

### 第二步：审查发布产物 (`scan`)（30 秒）

直接扫描任意编译生成的归档包（`.tar.gz`、`.zip`）、Python Wheel（`.whl`）、Java JAR（`.jar`）或可执行文件：

```sh
releasefloor scan dist/myapp-linux-amd64.tar.gz
```

**终端输出示例：**
```
releasefloor 0.1.0 · pass

artifact: myapp-linux-amd64.tar.gz
sha256:   e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
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

### 第三步：比对两版发布产物 (`compare`)（30 秒）

比对新构建的候选包与上一个已知稳定的历史版本，防止兼容性底线上浮倒退：

```sh
releasefloor compare releases/v1.0.0.tar.gz dist/v1.1.0.tar.gz
```

* **通过 (Exit Code 0)**：运行时版本要求一致或下降，或正常新增了二进制程序。
* **阻断 (Exit Code 1)**：任何 GLIBC 版本上浮、macOS 最低系统上调、架构切片遗漏或新增未预料的动态库。

---

### 第四步：自动生成门禁策略并集成到 CI (`init-policy` + `check`)（1 分钟）

无需手写繁琐的 JSON 规则配置，ReleaseFloor 可根据现有稳定版本自动逆向推导标准契约：

```sh
# 1. 一键基于当前稳定发布包推导出版本基线策略文件
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
- `releasefloor_scan`：查看任何二进制归档包的底层依赖元数据。
- `releasefloor_compare`：比对新旧两版发布包的兼容性差异。
- `releasefloor_check`：根据发布策略判定矩阵是否合规。
- `releasefloor_init_policy`：一键自动生成策略文件。

---

## 🚀 GitHub Actions 持续集成接入

在你的 release 工作流中仅需添加 3 行 YAML：

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

      # 1. 编译各平台跨平台产物至 dist/ 目录
      # ... (例如 goreleaser, cargo-dist, cmake 或 wheels 构建)

      # 2. 在上传 GitHub Releases 资产前执行门禁拦截
      - name: Gate release runtime requirements
        uses: feiiiiii5/releasefloor@v0.1.0
        with:
          command: check
          args: --policy .github/releasefloor-policy.json dist/*.tar.gz dist/*.zip
```

---

## 🌐 语言与生态覆盖

ReleaseFloor **完全语言无关**。它审查归档包内真实的底层机器码格式元数据（ELF 与 Mach-O），覆盖几乎所有主流软件分发场景：

* **C & C++**：独立二进制、共享库（`.so`, `.dylib`）与插件系统。专门为 `GLIBC`、`GLIBCXX` 与 `CXXABI` 版本门禁提供原生级精准提取。
* **Go & Rust**：单二进制 CLI、纯静态构建（`musl` / `CGO_ENABLED=0`）、交叉编译产物以及附带的系统动态依赖。
* **Python（原生二进制 Wheel）**：原生检查二进制 `.whl`（如 `manylinux` 与 `macosx` 轮子）内部打包的编译扩展（`.so`, `.dylib`），无需在磁盘解压。
* **Java / JVM（JNI 库与原生镜像）**：原生检查 `.jar` 归档中随包分发的 JNI 动态库，或 GraalVM Native Image 提前编译（AOT）生成的本地原生二进制。
* **Zig & Swift**：Linux 与 macOS 上的跨平台原生可执行文件与动态 Framework。

支持的输入格式：`.tar.gz`、`.tgz`、`.zip`、Python `.whl`、Java `.jar` 以及裸二进制文件。

---

## 📊 产品特性与竞品横向对比

| 核心特性 | ReleaseFloor | abicheck | BLINT | cargo-dist | 手写 Bash 脚本 |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **混合压缩归档支持** (`.tar.gz`, `.zip`, `.whl`, `.jar`) | **原生支持** (内存流式，免解压) | 仅支持单 `.so` | 仅支持单二进制 | 仅构建期间 | 需手动 `tar`/`unzip` |
| **多架构 Universal Mach-O 切片检查** | **完整支持** (检查所有 Slice) | 部分 | 不支持 | 仅限打包过程 | 需依赖 `otool`/`lipo` |
| **零执行 / 无需沙箱** | **纯静态解析** (绝对安全) | 是 | 是 | 是 | 视脚本而定 |
| **全矩阵声明式策略门禁** | **支持** (声明式 JSON 策略) | 不支持 | 仅做 Diff | 静态硬编码 | 难以维护 |
| **自动推导策略 (`init-policy`)** | **支持** (一键逆向基线) | 不支持 | 不支持 | 不支持 | 不支持 |
| **面向 AI Agent 的原生 MCP 协议** | **原生内置** (stdio 零依赖) | 不支持 | 不支持 | 不支持 | 不支持 |
| **编程语言无关** | **任何原生二进制** | C/C++ 专属 | 任何二进制 | Rust 生态专属 | 跨操作系统极其脆弱 |
| **外部环境依赖** | **零依赖** (纯 Go 标准库) | 依赖 Python / 动态库 | 依赖 Python / 动态库 | 依赖 Rust 工具链 | 依赖 `readelf` / `otool` |
| **离线自包含 HTML 审计报告** | **支持** (严格 CSP 安全标头) | 不支持 | 支持 HTML | 不支持 | 不支持 |

---

## 🔒 安全性与设计边界

* **绝不执行目标二进制**：ReleaseFloor 纯粹进行静态元数据解析，采用带上限的流式读取器防御恶意构造文件。
* **纯 Go 标准库实现**：无第三方依赖、无 CGo。交叉编译极简，攻击面极小。
* **从不静默覆盖已有文件**：`--output` 严格要求目标文件不存在（`O_CREATE | O_EXCL`），坚决不覆盖输入包或已有报告。
* **防解压炸弹与目录穿越**：内置对 Zip-Slip（`../` 路径穿越）、解压放大炸弹以及畸形格式头的全面边界防御。

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
