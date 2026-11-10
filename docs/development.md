# 运行与开发

本文包含 Go 后端的实现现状、开发规划、运行参数、构建及验证命令。所有命令均从仓库根目录执行；数据路径和存储兼容性见 [存储与模型配置](storage.md)，接口及联调见 [JSON-RPC 协议](jsonrpc.md)。

## 当前状态

已有统一模型接口，以及基于 OpenAI Go SDK v3 的 Chat Completions 文本和图片输入适配器，
支持非流式请求、流式事件、上下文取消和错误处理，SDK 版本以 `go.mod` 为准。
入口 `cmd/callum/main.go` 已装配数据库、模型适配器与标准输入输出通信服务。
Go 版本以 `go.mod` 为准，当前为 Go 1.27.3。

已新增 GORM + SQLite 的厂商与模型配置持久化模块，提供路径计算、版本迁移、配置增删改查、
模型启用控制和内部运行配置解析。启动程序后可以逐行输入 JSON 请求，管理配置、
根据本地模型 ID 调用模型、接收生成事件并取消独立执行。支持并发调用，超出上限时拒绝新调用。
客户端与 Go 之间采用 stdio，Go 调用模型厂商使用 HTTP；Electron 桌面客户端位于独立的 `callum-desktop` 仓库。
普通对话通过 `conversation.send` 发送当前输入，服务保存历史并组装上下文，支持工作区、并行标题、编辑重发、步骤查询和原始 usage。
兼容的 `model.invoke` 继续接收完整文本消息列表，不保存历史。
普通对话按输入 Token 预算规划摘要压缩，并在请求触及消息数量或实际编码字节边界前尝试压缩；预算估算与厂商实际 usage 分开，请求、响应和内存安全上限继续生效。远端仍负责最终容量校验。

适配器通过运行时配置接收 API 根地址和凭据，并由调用方传入 HTTP 客户端，
请求中的模型 ID 使用远端模型 ID。连接池由调用方管理，每次请求的超时与取消通过 `context.Context` 传递。
适配器不会自行读取 `OPENAI_*` 环境变量，也不会继承其中的组织、项目或自定义请求头。
普通对话支持文本和本地图片输入；已接通支持可选工作区的文件读取与补丁工具、跨平台非交互命令执行、模型自主工具调用与审批；已接通手动与自动上下文压缩；图片输出尚未接入。非流式请求失败时可能返回部分文本，
流式事件的接收方应以 `Stream` 返回 `nil` 作为整个调用成功的依据。
非流式响应和流式分片都会校验消息结构及所消费字段的 JSON 类型，合法空文本仍可正常返回。
响应或分片包含非 NULL 的 `audio` 生成字段时，明确返回不支持的输出错误；`audio:null` 和未知厂商元数据不影响正常文本处理。
非流式响应体关闭失败会作为调用错误返回，并保留同时发生的读取、解析或供应商错误。
已收到结束原因后，兼容 `usage` 对象附带单个空 `delta` choice 的用量分片，
但不接受其中新的生成内容或结束原因。

## 开发规划

以下为分阶段开发目标：

1. **模型接入**：接入一个模型供应商，支持文本请求、流式输出、上下文取消和错误处理。
2. **普通对话**：管理多轮消息历史、组装模型上下文，支持多个对话异步执行和独立取消。
3. **Agent 与工具**：实现工具注册、参数与权限校验、工具执行，以及结果回传模型的执行循环。
4. **上下文压缩与恢复**：已实现手动与自动压缩、来源追溯及摘要版本切换；中断状态已持久化，不自动重试执行。

已确认的表结构、事件和后续功能约定见 [对话设计](design/conversations.md)。当前已完成模型接入、普通对话、文件读取、补丁与命令工具循环及审批；上下文压缩已接通；用户补充输入已通过问题工具与 stdio 接入，更多工具随对应功能引入。

模块清单、业务与存储边界、工具组织及装配依赖见 [后端架构](architecture.md)。

## 运行

从仓库根目录执行：

```sh
go run ./cmd/callum
```

就绪后 stdout 输出 `system.ready` JSON 通知，随后持续接收 stdin 请求。
诊断日志使用 `log/slog` 输出 JSON 到 stderr，stdout 不输出欢迎语或命令提示符。
`--help` 将帮助文本写入 stderr 并退出，不打开数据库。

| 启动参数 | 默认值 | 说明 |
| --- | --- | --- |
| `--data-dir` | 见[数据路径约定](storage.md#数据路径与访问权限) | 显式数据目录优先，相对路径按工作目录计算 |
| `--max-concurrent-runs` | `4` | 同时执行的调用上限，范围 `1` 至 `64`，超出时立即返回忙碌错误 |
| `--call-timeout` | `5m` | 普通对话每次模型请求超时；兼容 `model.invoke` 仍按配置解析及调用整体计时，必须大于零 |
| `--tool-timeout` | `30s` | 单次工具执行超时，必须大于零 |

运行数量和实际模型请求数量分别受同一个配置上限约束；标题使用空闲模型请求容量，不排队争用。
活动任务占满运行容量时立即返回忙碌；已确定终态的任务正在清理时，允许在受理请求的短期超时内等待其释放。

例如：

```sh
go run ./cmd/callum --max-concurrent-runs 2 --call-timeout 3m
```

初始化限时 10 秒，配置接口及调用前的配置查询限时 5 秒；短期初始化、配置请求上下文
不会成为异步模型执行的运行上下文。HTTP 响应头等待上限为 30 秒，不自动跟随重定向或重试。

## 检查与构建

```sh
gofmt -l cmd internal
go vet -mod=readonly ./...
go fix -mod=readonly -diff ./...
CALLUM_LIVE_TEST=0 go test -mod=readonly -race -timeout 60s ./...
CGO_ENABLED=0 go build -mod=readonly -o bin/callum ./cmd/callum
git diff --check
```

发布程序使用固定构建标签：

```sh
CGO_ENABLED=0 go build -mod=readonly -tags release -o bin/callum ./cmd/callum
```

构建产物写入 `bin/`，该目录已加入 `.gitignore`。
`go fix -diff` 只预览当前 Go 版本可用的现代化建议，不写入源文件。
有建议时命令返回非零退出码，CI 会将其作为检查失败；建议应逐项审查并落实，再确认预览无差异。`.gitattributes` 固定 Go 文件检出为 LF，保持 Windows、macOS 和 Linux 的 gofmt 检查一致。
代码优先使用当前 Go 版本支持且能提高清晰度的写法，例如 `errors.AsType`、`slices` 和 `t.Context()`。

适配器测试使用可控的 HTTP transport 模拟响应，不访问真实模型服务。
测试覆盖消息转换、异常响应、流式分片与中断、取消和超时、回调失败、错误链及响应资源释放。
这些模拟测试不能替代具体供应商端点的真实 API 联调。

持久化测试仅使用临时数据库，覆盖配置读写与重新打开、凭据保留和清空、JSON 空值、
GORM 零值更新、UUIDv7 主键约束、模型禁用、关联删除限制、上下文取消、初始化回滚、
同版本整数主键结构及不支持版本的拒绝与数据保留、并发初始化和访问权限。配置业务测试使用存储替身验证业务规则与上下文传递；
协议测试通过业务服务和内存存储替身验证请求映射，不依赖 SQLite。
通信与执行测试覆盖响应顺序、协议错误、独立取消、并发限制、超时、输出阻塞和清理错误。
应用集成测试通过真实临时 SQLite 与本机模拟 HTTP 服务验证
“保存配置 → 查询 → 生成 → 并发与取消 → 重启读取”的调用链，不调用真实模型 API。
运行该测试需要允许监听本机临时回环端口。
可单独执行：

```sh
go test -mod=readonly -race -timeout 60s ./internal/config/... ./internal/model/provider/... ./internal/storage/sqlite/... ./internal/stdio/... ./internal/inference/... ./internal/workspace/... ./internal/conversation/... ./internal/agent/...
go test -mod=readonly -tags release -race -timeout 60s ./internal/config/... ./internal/model/provider/... ./internal/storage/sqlite/... ./internal/stdio/... ./internal/inference/... ./internal/workspace/... ./internal/conversation/... ./internal/agent/...
CGO_ENABLED=0 go test -mod=readonly -timeout 60s ./internal/config/... ./internal/model/provider/... ./internal/storage/sqlite/... ./internal/stdio/... ./internal/inference/... ./internal/workspace/... ./internal/conversation/... ./internal/agent/...
```
