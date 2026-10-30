# Callum

Callum 面向通用任务 Agent 开发，目标是通过模型推理与工具调用完成任务，
支持多轮对话、多对话异步执行、上下文压缩和执行取消。

## 当前状态

项目处于重新构建阶段，当前聚焦模型接入。
已有统一模型接口，以及基于 OpenAI Go SDK v3 的 Chat Completions 文本适配器，
支持非流式请求、流式事件、上下文取消和错误处理，SDK 版本以 `go.mod` 为准。
入口 `cmd/callum/main.go` 已装配数据库、模型适配器与标准输入输出通信服务。
Go 版本以 `go.mod` 为准，当前为 Go 1.27.3。

已新增 GORM + SQLite 的厂商与模型配置持久化模块，提供路径计算、版本迁移、配置增删改查、
模型启用控制和内部运行配置解析。启动程序后可以逐行输入 JSON 请求，管理配置、
根据本地模型 ID 调用模型、接收生成事件并取消独立执行。支持并发调用，超出上限时拒绝新调用。
客户端与 Go 之间采用 stdio，Go 调用模型厂商使用 HTTP；本仓库尚未包含 Electron 客户端。
调用请求必须携带完整文本消息列表，当前不保存会话历史，不自动管理多轮上下文或 Token 预算。

适配器通过运行时配置接收 API 根地址和凭据，并由调用方传入 HTTP 客户端，
请求中的模型 ID 使用远端模型 ID。连接池由调用方管理，每次请求的超时与取消通过 `context.Context` 传递。
适配器不会自行读取 `OPENAI_*` 环境变量，也不会继承其中的组织、项目或自定义请求头。
目前支持文本消息；工具调用与多模态尚未接入。非流式请求失败时可能返回部分文本，
流式事件的接收方应以 `Stream` 返回 `nil` 作为整个调用成功的依据。
非流式响应和流式分片都会校验消息结构及所消费字段的 JSON 类型，合法空文本仍可正常返回。
非流式响应体关闭失败会作为调用错误返回，并保留同时发生的读取、解析或供应商错误。
已收到结束原因后，兼容 `usage` 对象附带单个空 `delta` choice 的用量分片，
但不接受其中新的生成内容或结束原因。

## 开发规划

以下为分阶段开发目标：

1. **模型接入**：接入一个模型供应商，支持文本请求、流式输出、上下文取消和错误处理。
2. **普通对话**：管理多轮消息历史、组装模型上下文，支持多个对话异步执行和独立取消。
3. **Agent 与工具**：实现工具注册、参数与权限校验、工具执行，以及结果回传模型的执行循环。
4. **上下文压缩与恢复**：管理 Token 预算、历史摘要和压缩版本，逐步完善执行状态持久化与中断恢复。

当前模块与后续规划按职责组织如下：

| 目录 | 职责 | 状态 |
| --- | --- | --- |
| `internal/model` | 业务使用的模型接口、请求、响应、流式事件和错误类型 | 已实现文本接口 |
| `internal/model/adapter/chatcompletions` | Chat Completions 请求、认证、响应转换和流式解析 | 已实现，含模拟测试 |
| `internal/model/provider` | 厂商连接、模型规格、启用设置、GORM 映射及运行配置解析 | 已实现配置持久化 |
| `internal/config` | 应用数据目录与数据库路径计算 | 已实现模块路径配置 |
| `internal/database` | SQLite 连接池、私有文件检查和结构迁移 | 已实现 |
| `internal/app` | 启动装配、运行配置解析与适配器创建、资源清理 | 已实现 |
| `internal/stdio` | 逐行 JSON 协议、接口参数转换、响应与事件输出 | 已实现 |
| `internal/inference` | 模型调用并发限制、执行标识、超时与独立取消 | 已实现 |

普通对话、Agent 调度、工具和上下文管理模块随对应功能引入。
其他协议和供应商兼容差异按实际需求接入。

## 厂商与模型配置持久化

持久化使用 GORM v1.31.2、官方 SQLite 方言 v1.6.0 和 `modernc.org/sqlite` v1.61.0，
具体依赖以 `go.mod` 为准。标准库和原有模型 SDK 不提供所需的 ORM 与 SQLite 引擎，
因此新增这些依赖。官方方言接收显式创建的 modernc 连接池，支持 `CGO_ENABLED=0`；
`mattn/go-sqlite3` 是方言的间接依赖，本模块不使用它建立连接。

模块调用顺序为 `config.LoadArgs(args)` / `config.Load(dataDir)` → `database.Open(ctx, cfg.DatabasePath)` →
`provider.NewStore(db.DB)`。`database.Open` 成功后，创建者负责调用 `db.Close()`，
`Store` 只使用传入的 GORM 实例，不关闭数据库。`Close` 可以重复调用。
初始化失败会关闭此前创建的连接池；初始化上下文不绑定到运行时实例，
每次配置操作都接收自己的 `context.Context`。

### 数据路径与访问权限

| 使用场景 | 模块配置约定 |
| --- | --- |
| 开发构建、GoLand 默认调试及 `go run` | `config.Load("")` 自动解析为 Callum 项目根目录下 `.callum/dev/callum.db` |
| 带 `-tags release` 的发布构建 | `config.Load("")` 自动解析为用户家目录下 `.callum/callum.db` |
| 自动化测试 | 每个测试使用 `t.TempDir()` 下独立的数据子目录 |

构建选择由 `internal/config/mode_dev.go`（`!release`）和 `mode_release.go`（`release`）控制，
调用方继续使用 `config.Load(dataDir)`，无需传递运行模式或修改源代码。
显式指定的数据目录始终优先，相对路径按当前工作目录解析。
开发默认路径从当前工作目录向上查找模块标识为 `github.com/xiaoyu98628/callum` 的 `go.mod`，
支持从项目子目录启动；定位失败时返回错误，不使用其他项目目录或用户家目录。
发布构建不依赖项目源码，家目录缺失或不是绝对路径时返回错误。
路径配置只计算路径，不创建文件；数据库模块只接收绝对路径，不自行推断运行模式。
`.callum/` 已被 Git 忽略，测试与开发不得使用真实用户数据目录。
程序入口已使用上述配置，启动时通过 `--data-dir` 可显式指定数据目录。

新建数据目录和数据库文件分别使用 `0700`、`0600` 权限。
Unix 系统下，已有数据目录或文件允许其他用户访问时会拒绝打开，不自动修改已有权限；
数据目录末级和数据库文件不接受符号链接。Windows 的访问控制依赖系统 ACL，
Unix 权限位不能作为其私有访问保证。

API Key 按已确认的方案保存在 SQLite 中，数据库没有额外加密。
厂商查询仅返回 `HasAPIKey`，不查询明文凭据；`Resolve` 在内部解析运行配置时才读取凭据，
运行配置不得整体写入日志或作为普通配置快照导出。GORM 的默认 SQL 日志已关闭，
存储错误由调用边界统一处理。数据库文件及可能出现的 SQLite 辅助文件都应作为敏感数据管理。

### 表结构与校验

| 表 | 字段 |
| --- | --- |
| `model_providers` | `id`、`name`、`protocol`、`base_url`、`api_key`、`created_at`、`updated_at` |
| `models` | `id`、`provider_id`、`display_name`、`remote_model_id`、`enabled`、`context_window`、`max_output_tokens`、`capabilities`、`model_type`、`reasoning_levels`、`created_at`、`updated_at` |

厂商和模型主键由服务端生成 UUIDv7，以标准小写、带连字符的 36 字符 TEXT 保存，
`provider_id` 使用同一格式；客户端按字符串传递本地 ID，不能自行指定新记录的 ID。
UUID 生成复用 `github.com/google/uuid v1.6.0`，已从间接依赖改为直接依赖，版本不变。
时间戳继续使用 UTC 毫秒。SQLite 没有原生表、字段 COMMENT 元数据，
`records.go` 中的中文注释用于源码说明，不会作为数据库注释保存。
协议、根地址和凭据属于厂商配置；当前只允许 `chatcompletions` 协议。
同一厂商可以配置多个连接，厂商名称不强制唯一；同一厂商配置下的远端模型标识必须唯一。
模型通过外键关联厂商，仍有模型引用时不能删除厂商。

`capabilities` 是模型声明支持的功能，使用 JSON 对象，当前识别 `reasoning`、`image_input`、`tools`；
未出现的键表示未知，`false` 表示明确不支持。
`model_type` 是用户选中启用的功能，使用 JSON 数组，必须属于已明确支持的功能且不能重复。
它不表示单选的模型用途分类。保存这些元数据不意味着当前协议适配器已经实现相应能力，
调用装配会验证适配器支持情况，不支持的选中功能会拒绝调用。

`reasoning_levels` 保存展示名称和远端参数值，例如：

```json
[
  {"display_name": "低", "remote_value": "low"},
  {"display_name": "中", "remote_value": "medium"},
  {"display_name": "高", "remote_value": "high"}
]
```

这些值仅用于说明格式，实际取值以厂商协议为准。同一模型的远端推理值不能重复；
明确不支持推理的模型不能配置非空推理等级。
Token 容量未知时保存 SQL `NULL`，填写时必须大于零，不自动推断容量。
能力、推理等级为 `nil` 时保存 `NULL`，空 map 或空切片保存为空对象或空数组，保留“未知”和“已知无选项”的区别。
`model_type` 为 `nil` 时按空数组保存。

配置更新为完整替换；`ProviderInput.APIKey` 为 `nil` 时保留原凭据，指向空字符串时清空凭据。
创建模型时必须明确指定 `ModelInput.Enabled`，不会把显式 `false` 改为 `true`。
更新会写入 `false`、空数组及需要清空的 `NULL`，不会自动创建不存在的记录。
`ListModels(ctx, true)` 只返回启用记录，`Resolve` 对已禁用模型返回 `ErrDisabled`。

### 迁移与并发

数据库使用 STRICT 表、外键约束、联合唯一索引及字段检查约束。
结构版本保存在 SQLite `user_version` 中，当前版本为 `2`。
首次建表及版本更新在同一写事务中执行，失败时回滚；
已有配置表但没有结构版本，或数据库版本高于程序支持版本时会返回错误并保留原数据。
版本 `1` 的整数主键数据库会在事务中升级为 UUIDv7 主键，并重新映射厂商关联。
配置、密钥、JSON 原文、NULL 值和时间戳保留；失败时恢复旧表、索引、记录及版本。
迁移成功后旧整数 ID 不再有效，调用方需要重新查询模型列表。旧表存在自定义索引或触发器时，
程序拒绝自动迁移并保留数据，避免丢失额外结构。
启动不使用 `AutoMigrate` 自动改写已有业务表；后续结构变化需要增加版本迁移与兼容测试。

连接池上限为一个连接，外键检查和 5 秒锁等待对每个连接生效。
事务使用 `BEGIN IMMEDIATE`，保证并发初始化时串行检查和更新结构版本。
每次配置操作传递调用方上下文；查询结果读取完成后释放连接，
`Resolve` 通过一次关联查询读取厂商和模型，不在等待模型响应期间持有连接或事务。

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
| `--data-dir` | 按构建标签选择 | 显式数据目录优先，相对路径按工作目录计算 |
| `--max-concurrent-runs` | `4` | 同时执行的调用上限，范围 `1` 至 `64`，超出时立即返回忙碌错误 |
| `--call-timeout` | `5m` | 单次模型执行超时，包含配置解析，必须大于零 |

例如：

```sh
go run ./cmd/callum --max-concurrent-runs 2 --call-timeout 3m
```

初始化限时 10 秒，配置接口及调用前的配置查询限时 5 秒；短期初始化、配置请求上下文
不会成为异步模型执行的运行上下文。HTTP 响应头等待上限为 30 秒，不自动跟随重定向或重试。

## 标准输入输出接口

消息采用 JSON-RPC 2.0 的请求、响应与通知结构，以换行分隔，每行一个 UTF-8 JSON 对象。
支持单请求及通知消息，不接受批量请求数组；空行忽略，换行结束的消息会立即处理。
客户端建议使用非空字符串 `id`，响应保留原请求 ID；合法通知不返回接口响应。
接口参数使用具名对象，不识别的参数字段会返回参数错误。

| 接口 | 参数与行为 |
| --- | --- |
| `system.info` | 空对象，返回协议版本与运行限制 |
| `system.shutdown` | 空对象，返回响应后开始关闭 |
| `provider.create` | `name`、`protocol`、`base_url`，可选 `api_key`；返回 `{ "id": "UUIDv7" }` |
| `provider.update` | `id` 及完整连接配置；省略 `api_key` 保留旧密钥，空字符串清空，`null` 无效 |
| `provider.get` / `provider.delete` | `{ "id": "UUIDv7" }`；删除仍被引用的厂商会失败 |
| `provider.list` | 空对象；结果数组只包含 `has_api_key`，不包含密钥 |
| `model.create` | `provider_id`、`display_name`、`remote_model_id`、`enabled` 以及可选元数据；返回 ID |
| `model.update` | `id` 及完整模型配置；省略的可选元数据清空为未知值，选中功能按空数组处理 |
| `model.get` / `model.delete` | `{ "id": "UUIDv7" }` |
| `model.list` | 可选 `selectable_only`，默认 `false`；`true` 只筛选启用记录，调用前仍会校验适配器支持情况 |
| `model.set_enabled` | `{ "id": "UUIDv7", "enabled": false }`，不影响已经接受的执行 |
| `model.invoke` | `model_id`、`messages`，可选 `stream`（默认 `true`）；接受后返回 `run_id` |
| `run.cancel` | `{ "run_id": "执行标识" }`；返回 `cancel_requested`，不存在或已确定终态时为 `false` |

厂商、模型查询结果均使用 snake_case 字段，并包含 `created_at`、`updated_at`。
模型元数据字段及空值语义见前文。创建、更新模型时 `enabled` 必须明确传入布尔值。
修改和删除接口成功返回记录 ID，不会将不存在的记录自动创建出来。

`messages` 是完整文本消息数组，包含 `role` 和 `content`，支持 `system`、`user`、`assistant`，
每次最多 256 条。服务不会在调用之间补充历史，不保存对话或执行记录。
当前适配器只提供文本调用，`model_type` 必须为空；保存了推理、图片、工具等能力元数据，
不代表适配器已支持这些功能，选中这些功能后发起调用会返回不支持错误。
Token 容量字段只是配置元数据，当前不做 Token 估算或设置输出 Token 参数。

每次调用重新查询厂商和模型配置，不在网络等待期间占用数据库连接。
接受后使用独立的配置快照、消息副本及取消上下文；后续配置修改只影响新的调用。
并发执行的事件可以交错，接收方必须按 `run_id` 分发；同一次执行的接口响应排在其事件之前。

| 事件 | 内容 |
| --- | --- |
| `system.ready` | 数据库与服务装配成功后通知协议版本和限制 |
| `run.text_delta` | `run_id`、`text`，仅流式调用输出 |
| `run.completed` | 正常完成；非流式调用包含完整 `text`，流式调用不重复累计文本 |
| `run.failed` | `run_id` 与结构化 `error`；非流式调用可能附带不完整文本 |
| `run.cancelled` | `run_id` 与取消错误 |

生成事件可以包含 `provider_request_id`，它与接口请求 ID、执行 ID 分开管理。
以 `run.completed` 作为成功依据，收到部分文本不能视为成功；每次被接受的执行只确定一次终态。
通信断开时无法保证终态事件送达，也没有断线重放或执行恢复机制。
执行标识仅在当前进程内有效，不作为数据库主键。

### 手动联调

运行程序后在同一个终端逐行输入以下消息，回车发送。示例中的密钥为占位符，
`<provider_id>`、`<model_id>`、`<run_id>` 必须替换为实际响应中的标识。
API 地址、远端模型标识及密钥应使用同一个厂商提供的配置。

```json
{"jsonrpc":"2.0","id":"1","method":"provider.create","params":{"name":"我的厂商","protocol":"chatcompletions","base_url":"https://api.example.com/v1","api_key":"YOUR_API_KEY"}}
{"jsonrpc":"2.0","id":"2","method":"model.create","params":{"provider_id":"<provider_id>","display_name":"我的模型","remote_model_id":"REMOTE_MODEL_ID","enabled":true,"model_type":[]}}
{"jsonrpc":"2.0","id":"3","method":"model.list","params":{"selectable_only":true}}
{"jsonrpc":"2.0","id":"4","method":"model.invoke","params":{"model_id":"<model_id>","stream":true,"messages":[{"role":"user","content":"你好"}]}}
{"jsonrpc":"2.0","id":"5","method":"run.cancel","params":{"run_id":"<run_id>"}}
{"jsonrpc":"2.0","id":"6","method":"system.shutdown","params":{}}
```

将 `stream` 改为 `false` 可验证非流式调用，仍会立即返回执行 ID，随后通过终态事件返回完整结果。
手动输入无需 Node.js。Electron 接入时由主进程启动打包的 Go 程序并持有管道，
窗口通过 preload 暴露的业务 API 与主进程通信，复用上述消息格式。

模型执行是异步的，stdin EOF、关闭请求、Ctrl+C 或 SIGTERM 会停止接收并取消、等待全部执行，
随后关闭 HTTP 空闲连接和数据库。EOF 与正常关闭会排空可发送的事件；外部取消或通信失败会直接关闭管道。
因此，单次 `echo ... | callum` 在输入结束后会取消尚未完成的调用；联调时保持输入打开直到取得终态。

请求每行最多 1 MiB（不含换行），响应最多 8 MiB，单次累计模型文本最多 1 MiB，
单次上游原始 HTTP 响应最多 16 MiB（包含流式协议开销），这些字节限制不等同于 Token 限制。
输出队列最多 64 条，单次写入或等待队列空间超过 5 秒会中断通信并清理执行。
输入超长、输出写入失败均会退出，不会无限缓存消息。

### 错误约定

请求格式错误、参数错误、接口不存在以及接受前的业务错误通过接口响应的 `error` 返回；
接受后的生成错误通过 `run.failed` / `run.cancelled` 返回。错误只包含受控描述及必要的供应商请求标识、
HTTP 状态，不暴露凭据、原始供应商响应体或底层错误链。

| 错误码 | 含义 |
| --- | --- |
| `-32700` / `-32600` | JSON 格式错误 / 请求对象无效 |
| `-32601` / `-32602` / `-32603` | 接口不存在 / 参数无效 / 内部错误 |
| `1001` / `1002` / `1003` | 配置不存在 / 模型标识重复 / 关联约束冲突 |
| `1004` / `1005` / `1006` | 模型禁用 / 功能或输出不支持 / 并发已满 |
| `1007` / `1008` | 取消 / 超时 |
| `1009` / `1010` / `1011` | 模型调用失败 / 文本超限 / 服务正在关闭 |

启动失败时不发送 `system.ready`，向 stderr 记录错误并以非零状态退出。

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
代码优先使用当前 Go 版本支持且能提高清晰度的写法，例如 `errors.AsType`、`slices` 和 `t.Context()`。

适配器测试使用可控的 HTTP transport 模拟响应，不访问真实模型服务。
测试覆盖消息转换、异常响应、流式分片与中断、取消和超时、回调失败、错误链及响应资源释放。
这些模拟测试不能替代具体供应商端点的真实 API 联调。

持久化测试仅使用临时数据库，覆盖配置读写与重新打开、凭据保留和清空、JSON 空值、
GORM 零值更新、UUIDv7 主键约束、模型禁用、关联删除限制、上下文取消、迁移回滚、
整数主键升级及并发初始化、访问权限。通信与执行测试覆盖响应顺序、协议错误、独立取消、
并发限制、超时、输出阻塞和清理错误。应用集成测试通过本机模拟 HTTP 服务验证
“保存配置 → 查询 → 生成 → 并发与取消 → 重启读取”的调用链，不调用真实模型 API。
运行该测试需要允许监听本机临时回环端口。
可单独执行：

```sh
go test -mod=readonly -race -timeout 60s ./internal/config ./internal/database ./internal/model/provider
go test -mod=readonly -tags release -race -timeout 60s ./internal/config ./internal/database ./internal/model/provider
CGO_ENABLED=0 go test -mod=readonly -timeout 60s ./internal/config ./internal/database ./internal/model/provider
```
