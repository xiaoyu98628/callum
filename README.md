# Callum

Callum 是面向通用任务 Agent 的 Go 后端，通过模型推理与工具调用完成任务，提供 stdio JSON-RPC 接口。
Electron 桌面客户端位于独立的 `callum-desktop` 仓库。

## 功能概览

- Chat Completions 文本及本地图片输入，支持流式输出、取消、超时和错误处理。
- 厂商、模型、工作区、会话与执行持久化，多会话有界并发、编辑重发和并行标题。
- 模型自主工具调用，包含文件读取、补丁、非交互命令、审批及用户补充输入。
- 上下文统计、手动与自动压缩、摘要来源追溯和中断状态恢复；恢复不自动重试操作。

图片输出、音视频输入输出、MCP 与插件尚未接入；平台支持和验证边界见对应专题文档。

## 快速启动

Go 版本以 [go.mod](go.mod) 为准，当前为 **Go 1.27.3**。从仓库根目录执行：

```sh
go run ./cmd/callum
```

启动后 stdout 输出 `system.ready`，随后逐行接收 JSON 请求；诊断日志写入 stderr。
开发默认数据目录为项目根目录下的 `.callum/dev1`，可通过 `--data-dir` 指定独立目录。
请求示例和测试数据隔离方法见 [JSON-RPC 联调](docs/jsonrpc.md#1-启动及输入约定)。

```json
{"jsonrpc":"2.0","id":"info","method":"system.info","params":{}}
```

运行模型前需配置厂商、启用模型，并在启用自动压缩时填写已核实的模型窗口或显式输入预算。
模型调用异步受理，联调时保持 stdin 打开直到执行终态。

## 文档导航

| 文档 | 内容 |
| --- | --- |
| [运行与开发](docs/development.md) | 当前状态、开发规划、启动参数、检查、构建与验证边界 |
| [存储与模型配置](docs/storage.md) | 数据路径、权限、模型元数据、表结构、迁移与并发 |
| [后端架构](docs/architecture.md) | 模块职责、依赖方向、装配与运行时资源所有权 |
| [JSON-RPC 协议与联调](docs/jsonrpc.md) | 接口、响应、事件、错误码、补充输入及请求示例 |
| [上下文统计与压缩](docs/context-compaction.md) | Token 估算、预算、压缩设置、摘要版本与恢复 |
| [Agent 工具与审批](docs/tools-and-approvals.md) | 工具循环、目录授权、审批设置、文件补丁与命令执行 |
| [对话、执行与上下文设计](docs/design/conversations.md) | 数据一致性、事务、轨迹恢复及平台沙箱设计 |

## 协作

开发与修改前阅读 [AGENTS.md](AGENTS.md)。文档命令从仓库根目录执行，验证使用独立临时资源。
