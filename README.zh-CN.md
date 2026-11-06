# ReleaseFloor

[下载版本](https://github.com/feiiiiii5/releasefloor/releases/tag/v0.1.0) · [English](README.md)

发布前检查：新构建的归档包是否提高了运行环境要求，或漏掉了原有架构。

ReleaseFloor 是一个实验性的本地命令行工具，面向发布维护者和安装工具维护者。它从单个二进制、ZIP、tar.gz 中读取 ELF、Mach-O 可执行文件及附带动态库，清点文件记录的运行要求，按归档成员、平台、架构比较两个版本，也能用显式 JSON 策略检查多个发布产物。

例如，重新构建后 CLI 开始要求更高的 GLIBC，附带动态库提高了最低 macOS 版本，或新归档少了一个架构。工具读取这些信息，**不运行输入文件**。

**目前是小型开源实验。** 报告只描述文件中实际记录的元数据，不能证明程序在目标系统上可用。实际目标系统的冒烟测试及现有发布检查仍然需要保留。

[English](README.md) · [精确行为约定](docs/CONTRACT.md) · [立项依据与替代方案](docs/PRODUCT.md)

## 安装与可复现示例

从版本页面下载与你的系统和架构对应的安装包，按 `SHA256SUMS` 校验后解压。在解压目录中，Linux/macOS 运行 `./releasefloor version`，Windows PowerShell 运行 `.\releasefloor.exe version`。需要在其他目录调用时，把该目录加入 PATH。也可使用 Go 安装：`go install github.com/feiiiiii5/releasefloor/cmd/releasefloor@v0.1.0`。Windows 支持运行本工具，0.1 版不支持检查 PE 输入；预编译 macOS 命令记录的最低系统为 13.0。

无需编译即可试用：从同一版本下载并解压 `releasefloor-examples.zip`，在示例目录中用工具的实际路径运行 `/path/to/releasefloor compare old.zip new.zip`。它应返回 1；`check --policy policy.json old.zip` 应通过，换成 `new.zip` 应阻断。`releasefloor-demo.html` 是已生成的离线示例报告。

要从源码重新生成示例，先检出并进入仓库根目录：

```sh
git clone https://github.com/feiiiiii5/releasefloor.git
cd releasefloor
go run ./examples/generate --out output/demo
go build -o releasefloor ./cmd/releasefloor
./releasefloor compare --format html --output output/comparison.html output/demo/old.zip output/demo/new.zip
```

生成器只创建原创的格式元数据，不是可执行程序。候选包提高 GLIBC、GLIBCXX 和 macOS 最低版本，并丢失一个 arm64 slice，因此最后一条命令应返回 1。重复生成或导出时选择新路径。

## 构建并试用

在已检出的仓库根目录构建，需要 Go 1.25 或更新版本。实现只使用 Go 标准库；构建后的工具运行时不需要 Go 或其他外部命令。

```sh
go build -o releasefloor ./cmd/releasefloor
./releasefloor scan app-linux-amd64.tar.gz
./releasefloor compare previous.zip candidate.zip
```

上述文件名是使用示例，不代表已经对这些文件完成测试。建议先扫描自己的真实发布产物，再与上一版比较。

```sh
./releasefloor scan --format json app-linux-amd64.tar.gz
./releasefloor compare --format html --output comparison.html previous.zip candidate.zip
```

默认输出到标准输出。`--output` 只能指定**尚不存在的新文件**，不会覆盖输入或已有文件。HTML 报告自包含，不使用远程资源或脚本。

## 检查多个发布产物

把类似下面的策略保存为 `policy.json`。阈值应按你实际承诺的运行要求填写，示例值不构成兼容性建议。

```json
{
  "schema": "releasefloor.policy.v1",
  "rules": [
    {"platform": "elf", "arch": "amd64", "max_floors": {"GLIBC": "2.28"}},
    {"platform": "elf", "arch": "arm64", "max_floors": {"GLIBC": "2.28"}},
    {"platform": "macos", "arch": "amd64", "max_os": "12.0"},
    {"platform": "macos", "arch": "arm64", "max_os": "12.0"}
  ]
}
```

```sh
./releasefloor check --policy policy.json --format json --output matrix.json app-linux-amd64.tar.gz app-linux-arm64.tar.gz app-macos.zip
```

每个二进制都必须匹配至少一条策略规则，所有匹配规则的上限均生效。平台和架构均为明确值的规则，还要求整组产物中至少有一个二进制匹配。因此示例中的四条规则要求四个目标维度全部出现。`*` 通配规则只限制上限，不保证某个具体目标存在。

平台选择器支持 `elf`、`macos` 和 `*`；架构使用实际报告的准确值或 `*`。版本要求类别支持 `GLIBC`、`GLIBCXX`、`CXXABI`。未知策略字段及不支持的类别名称会报错。如果完整检查中没有记录某类别要求，它不会超出该类别上限，工具也不会捏造零版本。元数据未知或检查不完整仍然阻止通过。

策略限制要求上限，并检查明确指定的目标是否存在，但不是完整归档成员清单。要发现某个成员或该成员的某个架构被删掉，仍应与上一版产物比较。

`elf` 表示文件格式，不能仅凭它证明目标系统是 Linux。Mach-O 平台取自文件记录的构建信息。编写策略时，应使用 `scan` 实际报告的平台和架构值。

## 两版比较的含义

比较按归档内的准确成员路径、平台和架构对应。删除原有二进制、提高或新引入数值版本要求及最低系统版本，以及覆盖不完整、必需元数据未知，都会阻止检查通过。新增二进制会列出，但不声称已与旧版比较其要求；新增直接依赖会提示审阅。

Mach-O 同一动态库提供者的记录兼容版本提高，或弱依赖变为强依赖，也会阻断比较并要求复核；弱与强依赖分别比较。这不代表已证明实际 ABI 不兼容。

归档内路径属于身份的一部分。如果两个版本分别使用 `app-1.0/bin/app` 和 `app-1.1/bin/app` 这样的外层目录，可明确指定去掉一层：

```sh
./releasefloor compare --strip-components 1 previous.tar.gz candidate.tar.gz
```

工具不猜测重命名关系。直接输入单个二进制时，内部使用稳定的 `binary` 成员名，因此外部文件改名不会被误判成成员删除。

## 范围与边界

- 支持读取 ELF、Mach-O 及通用 Mach-O 的多个架构切片，输入为原始文件、ZIP、tar.gz 或 `.tgz`。
- 按文件内容识别归档中的普通成员，不依赖扩展名或可执行权限；也扫描随包分发的动态库。
- 不解压到磁盘、不跟随符号链接、不递归扫描嵌套归档、不运行输入、不获取依赖、不联网。
- 对不支持的可执行格式、嵌套归档、未知必需元数据和检查不完整显式报告。解析失败不会变成“没有问题”。
- 拒绝损坏归档、重复身份、不安全成员路径和超过资源限制的输入。默认限制为输入 128 MiB、单成员 64 MiB、普通成员合计 256 MiB、4,096 个成员。
- 保留产物与成员哈希、身份及元数据证据。哈希只能标识字节，不能证明来源真实。

ELF 数值版本上限和漂移检查仅覆盖 GLIBC、GLIBCXX、CXXABI。其他版本节点保留为证据，不代表已检查其数值变化或完整 ABI。FAT64、无 section/扩展编号 ELF、压缩的运行元数据及不支持的机器格式会明确失败。

动态加载、间接依赖、CPU 指令、内核要求和实际运行行为不在检查范围内。静态链接也不意味着能在所有系统运行。工具面向发布者自己的构建产物，采用有界解析，不提供恶意样本分析所需的隔离环境。

## 报告与 CI

三个命令均支持 `--format text|json|html`，JSON 报告格式为 `releasefloor.report.v1`。报告使用产物基本文件名和归档成员名，不写入本地输入的绝对路径；成员路径、依赖名称等元数据仍可能包含不适合公开的信息，分享前请检查。

| 退出码 | 含义 |
| --- | --- |
| `0` | 已实现的检查完成，未发现阻止通过的问题。 |
| `1` | 报告包含阻止通过的变更、策略问题或未知／不完整覆盖。 |
| `2` | 调用、输入、策略无效，或发生读写错误。 |

在 CI 已完成构建并收集产物后，可加入：

```sh
./releasefloor check --policy policy.json --format json --output releasefloor-matrix.json dist/app-linux-amd64.tar.gz dist/app-linux-arm64.tar.gz dist/app-macos.zip
```

让退出码影响任务结果，每轮使用新的报告文件名。需要检测删除时，再运行与上一版的 `compare`。上一版产物由你现有的可信流程获取、校验，ReleaseFloor 不代为下载。

## 替代方案与项目状态

如果现有发布脚本已经可靠回答这些问题，继续使用它即可。abicheck、BLINT、Packslip、cargo-dist 已覆盖相邻或重叠需求；详见[替代方案对比](docs/PRODUCT.md#alternatives)。本项目只实验“直接检查混合 CLI 归档及显式产物矩阵”，并保留缺失架构、不可比较和覆盖不足的结果。它不能代替 ABI 分析器或发布打包系统。

目前不声称有外部采用、商业验证或完整兼容性。立项依据、未知项和后续停止／扩大投入条件见 [PRODUCT.md](docs/PRODUCT.md)。变更记录见 [CHANGELOG.md](CHANGELOG.md)，贡献方式见 [CONTRIBUTING.md](CONTRIBUTING.md)，安全边界见 [SECURITY.md](SECURITY.md)。原创项目代码采用 [MIT 许可](LICENSE)。
