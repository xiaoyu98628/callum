# Callum

Callum 面向通用任务 Agent 开发，目标是通过模型推理与工具调用完成任务，
支持多轮对话、多对话异步执行、上下文压缩和执行取消。

## 当前状态

已有统一模型接口，以及基于 OpenAI Go SDK v3 的 Chat Completions 文本和图片输入适配器，
支持非流式请求、流式事件、上下文取消和错误处理，SDK 版本以 `go.mod` 为准。
入口 `cmd/callum/main.go` 已装配数据库、模型适配器与标准输入输出通信服务。
Go 版本以 `go.mod` 为准，当前为 Go 1.27.3。

已新增 GORM + SQLite 的厂商与模型配置持久化模块，提供路径计算、版本迁移、配置增删改查、
模型启用控制和内部运行配置解析。启动程序后可以逐行输入 JSON 请求，管理配置、
根据本地模型 ID 调用模型、接收生成事件并取消独立执行。支持并发调用，超出上限时拒绝新调用。
客户端与 Go 之间采用 stdio，Go 调用模型厂商使用 HTTP；本仓库尚未包含 Electron 客户端。
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

已确认的表结构、事件和后续功能约定见 [对话设计](docs/design/conversations.md)。当前已完成模型接入、普通对话、文件读取、补丁与命令工具循环及审批；上下文压缩已接通；用户补充输入已通过问题工具与 stdio 接入，更多工具随对应功能引入。

当前模块与后续规划按职责组织如下：

| 目录 | 职责 | 状态 |
| --- | --- | --- |
| `internal/model` | 模型接口、内容块、响应、usage 和流式结果契约 | 已实现 |
| `internal/model/adapter/chatcompletions` | Chat Completions 请求、认证、响应转换和流式解析 | 已实现，含模拟测试 |
| `internal/model/provider` | 厂商与模型配置类型、存储接口、错误与标识校验 | 已实现契约 |
| `internal/model/provider/service` | 配置业务校验、UUIDv7 分配及启用规则 | 已实现业务服务 |
| `internal/config` | 应用配置加载及启动参数 | 已实现 |
| `internal/settings`、`service` | 配置组契约、执行快照、值校验与版本更新 | 已实现 |
| `internal/storage/sqlite/settings` | 应用设置持久化和版本冲突校验 | 已实现 |
| `internal/stdio/settings` | 应用设置查询、更新接口 | 已实现 |
| `internal/config/paths` | 开发与发布构建的数据路径计算 | 已实现 |
| `internal/storage/sqlite` | SQLite 连接池、私有文件检查、服务实例文件锁 | 已实现 |
| `internal/storage/sqlite/schema` | 仓储与迁移共用的 GORM 表映射 | 已实现 |
| `internal/storage/sqlite/catalog` | 厂商与模型配置仓储、映射转换及存储错误处理 | 已实现 |
| `internal/storage/sqlite/migrations` | 事务建表、结构版本检查及迁移测试 | 已实现 |
| `internal/app` | 独立运行时装配、业务入口、stdio 连接接入及资源清理 | 已实现 |
| `internal/stdio` | 通信服务入口、就绪信息及请求路由 | 已实现 |
| `internal/stdio/protocol` | JSON-RPC 消息结构及请求、参数解析 | 已实现 |
| `internal/stdio/transport` | 标准流适配、连接、输出队列及通信关闭 | 已实现 |
| `internal/stdio/catalog` | 厂商与模型配置接口、参数和业务错误转换 | 已实现 |
| `internal/stdio/execution` | 模型调用、取消接口及执行事件转换 | 已实现 |
| `internal/inference` | 模型调用请求、执行事件及错误契约 | 已实现 |
| `internal/inference/executor` | 兼容无历史调用，复用统一运行调度 | 已实现 |
| `internal/workspace/service` | 工作区管理、软删除和恢复 | 已实现 |
| `internal/conversation/service`、`history` | 会话管理、发送与重发、标题和来源指纹 | 已实现 |
| `internal/agent/executor`、`service` | 统一调度、模型与工具循环、并行标题及终态保存 | 已实现 |
| `internal/agent/approval` | 审批快照、决定及等待协调 | 已实现 |
| `internal/agent/transcript` | 执行轨迹配对、失败回答过滤及中断工具输出恢复 | 已实现 |
| `internal/tool/registry`、`executor` | 工具注册、规范化、权限边界及执行 | 已实现 |
| `internal/tool/policy` | 根据可信操作事实、历史决定、沙箱边界及目录授权计算审批要求 | 已实现 |
| `internal/tool/builtin/filesystem` | 文件工具统一装配入口 | 已实现 |
| `internal/tool/builtin/filesystem/list`、`read`、`search`、`patch` | 独立的目录查看、按行读取、字面搜索及文件补丁工具 | 已实现 |
| `internal/tool/builtin/filesystem/internal/fileaccess` | 文件工具内部共享的固定目标、身份检查、句柄及有界文本读取 | 已实现 |
| `internal/tool/builtin/command` | 命令工具统一装配入口 | 已实现 |
| `internal/tool/builtin/command/run` | 命令审批快照、进程组/Job 生命周期及有界双流输出 | 已实现，平台验证结果分别说明 |
| `internal/tool/builtin/internal/arguments` | 文件与命令工具共用的严格 JSON 参数结构校验 | 已实现 |
| `internal/stdio/approval` | 审批查询和决定接口 | 已实现 |
| `internal/agent/events` | 有界事件订阅、过滤及慢订阅者隔离 | 已实现 |
| `internal/storage/sqlite/workspace`、`conversation`、`agent` | 普通对话仓储及跨表事务 | 已实现 |
| `internal/stdio/workspace`、`conversation` | 工作区和对话接口 | 已实现 |

`internal/conversation/compaction` 负责完整轮次的压缩规划、来源指纹与摘要校验，执行编排位于 `internal/agent/service`。用户补充输入使用现有步骤持久化，支持选项、文字、跳过和取消。工具和审批业务不依赖 stdio，后续入口复用同一 Agent 与审批服务。
会话更新通过 `conversation.Update` 接收模型 ID 及可更新字段，业务服务校验后生成仓储使用的 `conversation.Change`，模型快照由服务端解析；JSON-RPC 的请求、响应和通知字段保持兼容。
`policy.Operation` 只保存可信的判定事实，不持有设置快照或工具资源；Agent 校验执行快照并查询授权，policy 统一计算历史拒绝、未知结果、沙箱及目录授权的最终判定，执行认领仍在事务内复核授权。
`agent/transcript` 不依赖 ORM 或通信协议；SQLite 保留有界查询、分支筛选和事务写入，历史读取与压缩检查点共用轨迹恢复规则，`conversation/history` 负责将轨迹装配到对应用户消息后。
其他协议和供应商兼容差异按实际需求接入。

目录按业务与职责划分；契约保留在模块根包，具体业务、存储和通信实现进入职责明确的子包。
测试跟随对应实现，跨模块测试通过公开入口组装；不为规划能力创建空包或占位实现。
文件和命令工具分别由 `filesystem.Tools()`、`command.Tools(environment)` 汇总，根包只负责工具集合入口；
具体工具子包通过 `New()` 提供统一工具契约，各自负责定义、业务参数校验、准备及执行。
命令实现位于 `command/run`，工具定义、参数解析和调用生命周期分别放置；环境由装配层显式传入。
各工具通过内部 `arguments` 包统一校验 JSON 结构，共享文件访问仍由 `filesystem/internal/fileaccess` 提供。

`provider/service.Service` 通过 `provider.Repository` 接口访问配置，由 `sqlite/catalog.Repository` 实现该接口。
GORM 映射和驱动错误处理集中在存储包，业务服务不导入 ORM 或数据库驱动。
stdio 路由调用 catalog、execution 接口处理器，后者将协议参数转换为业务输入；
protocol 和 transport 包不依赖配置业务、模型执行或 stdio 根包。
执行管理器只依赖统一模型接口，具体适配器由 app 中的工厂装配。
app 创建连接、配置服务及执行管理器，并负责资源关闭顺序；程序入口处理参数、信号和退出码。

## 厂商与模型配置持久化

持久化使用 GORM v1.31.2、官方 SQLite 方言 v1.6.0 和 `modernc.org/sqlite` v1.61.0，
具体依赖以 `go.mod` 为准。标准库和原有模型 SDK 不提供所需的 ORM 与 SQLite 引擎，
因此新增这些依赖。官方方言接收显式创建的 modernc 连接池，支持 `CGO_ENABLED=0`；
`mattn/go-sqlite3` 是方言的间接依赖，本模块不使用它建立连接。

配置存储的装配顺序为 `config.LoadArgs(args)` / `config.Load(dataDir)` →
`sqlite.Open(ctx, cfg.DatabasePath)` → `catalog.NewRepository(db.DB)` → `providerservice.New(repository)`。
这里的 `catalog` 为 `internal/storage/sqlite/catalog`，`providerservice` 为 `internal/model/provider/service` 的导入别名。
数据库打开成功后由创建者调用 `db.Close()`；存储实现使用传入的 GORM 实例，
业务服务使用传入的存储接口，二者均不关闭数据库。数据库 `Close` 可以重复调用。
`sqlite.Open` 将厂商、模型、工作区、会话、消息、运行、步骤、审批和应用设置的 GORM 表映射传给 `migrations.Run`；
迁移逻辑及其测试集中在独立目录，迁移包不反向导入 SQLite 存储包，也不重复定义表映射。
`migrations/migrate.go` 负责版本判断、事务和回滚；
`migrations/000001_create_model_config.go` 负责创建厂商表、模型表及其约束和索引，以及已有库的主键类型检查。
`000002_create_conversation_core.go` 创建普通对话五张表。`000003_create_run_approvals.go` 增加审批表及步骤组合唯一索引。`000004_create_app_settings.go` 增加应用设置表、默认执行设置及运行配置快照。`000005_create_context_versions.go` 新增摘要版本表和独立压缩设置组，直接初始化当前 Token 配置，设置 revision 为 `1`。`000006_create_directory_grants.go` 新增会话目录授权表。新库顺序执行六个迁移，在同一事务中记录版本 `6`；已有版本 `1` 至 `5` 的库通过检查后保留配置、对话、审批及摘要并升级。旧执行快照保持 NULL，不推断其历史权限。
初始化失败会关闭此前创建的连接池；初始化上下文不绑定到运行时实例，
每次配置操作都接收自己的 `context.Context`。

`app.New(ctx, cfg, logger)` 创建独立运行时，不需要 stdin/stdout。运行时持有数据库、实例锁、HTTP 连接池、
共享调度器和事件发布器；初始化上下文仅用于启动，已接受的执行使用应用运行上下文。
`Providers()`、`Workspaces()`、`Conversations()`、`Runs()` 返回业务服务，`Invoke` 提供无历史调用，
`Approvals()` 返回审批查询与决定服务，`Settings()` 返回应用设置查询与更新服务，`Cancel` 请求独立取消。无历史调用和普通对话共享运行及实际模型请求的并发限制。
调用 `Send` 或 `Invoke` 时，`accepted` 回调获得执行标识；回调成功后才启动执行和发布执行事件。
回调失败时释放预留容量，已写入的普通对话执行保存为取消状态。

业务事件通过 `Runtime.Subscribe(events.Filter{...})` 订阅，可同时按会话、执行标识和事件类型过滤；
空过滤器接收全部事件。每个运行时最多 32 个订阅，每个订阅最多排队 64 个事件，调用者负责关闭订阅，事件内容只读。
订阅读取使用 `Next(ctx)`，正常关闭后排空队列并返回 `io.EOF`；读取上下文取消后仍需关闭订阅。
队列溢出立即断开该订阅并返回 `events.ErrSlowConsumer`，其他订阅和业务执行继续运行，不提供逐事件重放。
步骤通知在进入队列前投影为元数据，完整输出、usage 和定义仍通过步骤查询读取；无历史调用事件不保留原始错误链。

`Runtime.ServeStdio(ctx, in, out)` 接管一个 stdio 连接，通过订阅将业务事件转成 JSON-RPC，
先发送 `system.ready`，再转发事件并处理请求。EOF 或 `system.shutdown` 只结束该连接，
取消并等待该连接发起的执行，随后排空订阅与协议输出；其他调用方的执行和共享运行时继续运行。
外部取消或通信失败会关闭该连接的标准流以解除阻塞。关闭订阅本身不会取消任务；连接所属任务由接入层单独登记和清理。

现有 `app.Run` 仍是独立 stdio 进程入口：创建运行时、接入连接，并在连接退出后调用 `Runtime.Close()`。
`Close` 停止入口、取消并等待所有任务及连接，关闭发布器、HTTP 空闲连接和数据库，最后释放实例锁，可以重复调用。
HTTP、WebSocket 和 CLI 入口尚未实现；接入时复用同一运行时和业务事件，各自转换请求、错误、响应和推送格式。
macOS/Linux 标准流副本与非阻塞标志的恢复封装在 `stdio/transport`，入口不直接操作文件描述符。

### 数据路径与访问权限

| 使用场景 | 模块配置约定 |
| --- | --- |
| 开发构建、GoLand 默认调试及 `go run` | `config.Load("")` 自动解析为 Callum 项目根目录下 `.callum/dev/callum.db` |
| 带 `-tags release` 的发布构建 | `config.Load("")` 自动解析为用户家目录下 `.callum/callum.db` |
| 自动化测试 | 每个测试使用 `t.TempDir()` 下独立的数据子目录 |

构建选择由 `internal/config/paths/dev.go`（`!release`）和 `internal/config/paths/release.go`（`release`）控制，
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
`internal/storage/sqlite/schema` 中的中文注释用于源码说明，不会作为数据库注释保存。
协议、根地址和凭据属于厂商配置；当前只允许 `chatcompletions` 协议。
同一厂商可以配置多个连接，厂商名称不强制唯一；同一厂商配置下的远端模型标识必须唯一。
模型通过外键关联厂商，仍有模型引用时不能删除厂商。

`model_type` 是可多选的内容类型元数据，使用 JSON 数组，允许 `text`、`image`、`audio`、`video`，不能重复；空数组表示尚未标注，不阻止文本对话或工具调用。
`capabilities` 保存具体能力，使用 JSON 对象，当前识别 `reasoning`、`image_input`、`tools`。
`image_input:true` 声明支持图片输入；缺少该键或为 `false` 时拒绝图片输入。`reasoning` 及推理等级仅作元数据，不自动设置远端推理参数。
当前所有模型在普通对话与 Agent 执行中默认获得已注册工具，`capabilities.tools` 保留为兼容元数据，无论缺失、`true` 或 `false` 都不控制工具提供，也不要求在 `model_type` 中选择工具。
内容类型与具体能力相互独立：标注 `image` 不等于支持图片输入或图片输出，标注 `audio`、`video` 不会使适配器自动获得音视频能力。
当前适配器支持文本及明确声明的图片输入，图片输出、音视频输入输出尚未接入。

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
结构版本保存在 SQLite `user_version` 中，当前版本为 `5`，与建表文件编号一致。
新库在同一写事务中建立配置、普通对话、审批、应用设置、上下文版本和目录授权结构，记录版本 `6`，失败时回滚。
已有版本 `1` 的数据库会检查两张配置表均使用单列 `TEXT` 类型的 `id` 主键，检查通过后继续使用，
不会重新分配记录标识，随后增加普通对话表。曾使用相同版本号的整数主键结构会被拒绝，不提供自动升级。
版本 `2` 升级时增加审批表及组合唯一索引；版本 `3` 升级时增加应用设置、初始化默认值并为执行表增加可空快照。版本 `4` 增加上下文版本并直接初始化 Token 压缩设置，不提供旧条数、字节阈值及其执行快照的数据转换。压缩预算从已配置模型窗口解析或显式设置。
开发阶段不提供旧 `model_type` 功能标签的数据转换；新建和更新配置按内容类型规则校验。
当前版本检查设置组唯一索引、快照约束及既有配置、对话和审批结构；历史同版本号但缺少相应结构的库会被拒绝。
已有配置表但没有结构版本、结构检查失败，
或版本不受支持时会返回错误并保留原有数据、结构和版本号，不自动删除或重建数据库。
开发时可通过 `--data-dir` 指向独立的新数据目录。
启动不使用 `AutoMigrate` 自动改写已有业务表；后续结构变化需要增加版本迁移与兼容测试。

连接池上限为一个连接，外键检查和 5 秒锁等待对每个连接生效。
事务使用 `BEGIN IMMEDIATE`，保证并发初始化时串行检查和更新结构版本。
每次配置操作传递调用方上下文；查询结果读取完成后释放连接，
`Resolve` 通过一次关联查询读取厂商和模型，不在等待模型响应期间持有连接或事务。

应用在迁移和恢复之前取得数据库旁的 `callum.db.lock` 文件锁，并持有到资源关闭。同一数据库只允许一个运行中的服务实例，多个客户端窗口共享这个 Go 进程。
取得实例锁后，将遗留活动执行及步骤标记为 `interrupted`，将待审批记录标记为 `cancelled`，保留已保存内容及已经作出的决定；终止确认时间为本次恢复时间，不推测实际崩溃时刻，也不自动重试。新契约的未完成工具步骤同时保存独立的操作事实，已启动但无法确认的修改或外发标记为 `unknown`；旧记录不补造历史事实。
关闭时释放锁，锁文件可以保留。初始化失败和重复关闭均释放本进程拥有的资源。

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
每次最多 256 条。这是兼容的 `model.invoke` 接口约定，不补充历史、不保存执行记录，只处理文本，不提供工具循环；模型内容类型及能力元数据不会阻止它执行文本请求。
普通对话与 Agent 默认向所有模型提供已注册工具，具体是否调用、调用哪个工具和参数由模型决定。图片输入需 `capabilities.image_input:true`，推理参数尚未接入。
工具提供不要求会话绑定工作区。文件工具支持绝对路径，相对路径需要工作区作为基准；用户提问不需要携带工具名称或调用参数。
`context_window` 用于解析普通对话压缩预算，`max_output_tokens` 在已配置时限制预算中的输出预留；这些字段不自动设置供应商输出 Token 参数。自动压缩启用时，必须配置有效窗口或显式输入预算；完整替换模型配置时需保留所需容量字段。

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
通信断开时无法保证终态事件送达，不提供逐事件重放。
`model.invoke` 的执行标识仅在当前进程内有效；普通对话的 UUIDv7 执行标识和历史状态保存到数据库。

### 普通对话接口

首次发送不提供 `conversation_id`，必须提供 `model_id`；接受后原子创建会话、用户消息、回答消息、执行及初始步骤。
已有会话可以省略 `model_id`，服务从会话选定模型重新解析当前有效配置。不存在的会话 ID 不会被当作新会话创建。
客户端“新会话”按钮只建立输入界面，不提供 `conversation.create` 接口。

| 接口 | 参数与行为 |
| --- | --- |
| `workspace.create` | `name`、绝对 `root_path`，返回工作区；相同路径已软删除则恢复原记录 |
| `workspace.get` / `delete` / `restore` | `id`，删除和恢复保留关联会话的独立状态 |
| `workspace.list` | 可选 `archived`、`deleted`、`limit`、`offset` |
| `workspace.update` | `id` 及可选 `name`、`pinned`、`archived` |
| `conversation.send` | `content` 内容块数组，可选 `conversation_id`、`model_id`、`stream`；新会话还可提供 `workspace_id` |
| `conversation.resend` | `conversation_id`、最后一条用户 `message_id`、新 `content`，可选 `model_id`、`stream` |
| `conversation.get` / `delete` / `restore` | `id`，软删除保留历史 |
| `conversation.list` | 可选 `workspace_id` 或 `unbound`，以及 `archived`、`deleted`、`limit`、`offset` |
| `conversation.update` | `id` 及可选 `title`、`model_id`、`workspace_id`、`detach_workspace`、`pinned`、`archived`；设置标题后来源为 manual |
| `conversation.messages` | `conversation_id`，可选 `before_sequence`、`limit`，返回按序排列的一页消息 |
| `run.get` / `run.list` | 持久化运行的 `id`；或 `conversation_id`、可选 `limit`、`offset` |
| `step.get` / `step.list` | 步骤 `id`；或 `run_id`、可选 `after_sequence`、`limit`，返回元数据和数据大小 |
| `step.data` | `step_id`、`field`（output、usage、input、definition），可选字节 `offset`、`limit`；返回 JSON 原文片段 |

列表 limit 默认 100，最大 200，offset 从 0 开始。消息查询还有 7 MiB 的页大小保护；可用返回页最小 sequence 作为下一次 before_sequence，直到空数组。
step.data 默认、最大分段为 64 KiB，最小 4 字节，返回 fragment、next_offset、total_bytes、done、present。按 next_offset 继续读取，拼接 fragment 后解析 JSON；不会在 UTF-8 字符中间截断。

首次受理响应包含 conversation_id、run_id、user_message_id、回答 message_id 和 removed_message_ids。
重发保留用户消息 ID 及 sequence，删除旧回答并返回其 ID；run.started 通知同样携带 removed_message_ids，供其他窗口清理旧回答。
原执行必须结束才能重发；只允许编辑最后一条用户消息。不保留旧回答的模型结果副本，但保留执行身份及 usage 等审计信息。
会话内 sequence 从 1 开始，不使用 parent_message_id。跨表关系使用外键、唯一约束和事务校验。

默认标题为首条问题文字去首尾空白后前 30 个 Unicode 码点，英文按字母、空格、标点计数，仅图片默认“新对话”。
标题调用与首次回答并行，使用当前模型，最多 10 秒且受模型请求超时约束；容量不足时跳过，主回答结束即取消并等待尚未完成的标题任务。
标题成功通过 conversation.updated 更新；人工改名和问题内容变化会阻止旧标题覆盖。标题失败不导致回答失败。

正文与消息状态保存在 conversation_messages，模型原始结果和 usage 保存在具体 model_call 步骤。
非流式 output 包含协议标识、json_response 格式和原响应对象；流式为 stream_result，标准字段组装，未知 delta 扩展按字段保留片段。
未取得可用模型响应时，以 request_error 保存受控 HTTP 状态、调用阶段和供应商请求标识，不保存可能回显凭据的错误体。
usage 原样保留供应商字段，缺失为 NULL；不把零值或估算值当作已返回的用量。标题单独计入其模型步骤，不在父执行重复累计。
流式正文分批保存，终态前保存最终或部分结果；异常进程退出可能丢失尚未落库的最后增量。
消息正文及其更新通知只采用通过累计文本上限检查的增量。超限时执行失败，并保留已接收的部分正文；步骤中的原始响应和实际 usage 独立保存，不会把被拒收的片段重新写入消息。

content 示例：`[{"type":"text","text":"分析图片"},{"type":"image","path":"/absolute/path/image.png"}]`。
图片要求使用规范绝对路径、普通对话启用 image_input，并由具体端点支持；服务读取 PNG、JPEG、GIF、WebP 为内联数据发送，不把本地路径直接交给远端。
单图最多 4 MiB，实际请求 JSON 上限 12 MiB；图片读取失败、格式不支持、超限均明确失败，不静默丢弃内容块。
图片在打开后核对普通文件类型及身份，再通过固定句柄读取。Unix 使用非阻塞打开，检查后路径被替换为命名管道时会拒绝读取，不等待管道写端。
当前适配器不生成图片，附件管理未接入，编辑或删除消息不删除图片源文件。
实际模型请求的有效上下文最多 256 条、JSON 不超过 8 MiB；历史可通过摘要持续复用，不以会话累计消息数作为上限。历史消息按完整轮次分页读取，轨迹按完整模型与工具批次读取；压缩准备窗口最多 512 条、16 MiB。自动压缩在模型调用前及工具结果追加后统一检查 Token、消息数量、上下文 JSON 和适配器实际请求字节；超限先压缩可安全替换的来源，再复查全部边界，不能容纳的单个批次明确失败，不静默截断历史。Token 预算与资源安全边界独立计算。
未成功完成的历史回答不作为完整回答传入后续上下文。图片原文件不复制、不版本化，历史再次调用仍依赖该路径指向的可读文件。

启用自动压缩时，运行以下普通对话示例前，请为模型保存已核实的 context_window，或通过 context_compaction 显式设置 token_budget（见“上下文压缩”）。model.invoke 不应用会话压缩预算。

### Go 向客户端推送的对话事件

通知没有顶层 id，method 为事件类型；params 按需携带 conversation_id、run_id、step_id、message_id。
同一发送请求的受理响应先于其所有事件。普通对话的事件与旧 model.invoke 共用一条输出连接，以 conversation_id 区分对话事件。

| 事件 | 内容 |
| --- | --- |
| `conversation.created` / `conversation.updated` | conversation 快照，包括默认、生成或人工标题 |
| `run.started` | run 快照，重发时包含 removed_message_ids |
| `message.created` / `message.updated` | message 快照；非流式回答通过最终消息快照返回 |
| `message.delta` | block_index 和 delta，例如 `{"type":"text","text":"你好"}`，只追加到对应消息 |
| `step.created` / `step.updated` | step 元数据，output、usage、definition 通过 step.data 获取 |
| `tool.output` | 工具输出分片，含步骤内 sequence、stdout/stderr、base64 字节 |
| `run.completed` / `run.failed` / `run.cancelled` | run 终态快照；失败提供受控 error.kind、error.message |

状态落库后通知；若终态持久化失败，run.failed 明确附带 persisted=false，表示数据库终态未确认，不能按成功保存处理。
原始大结果不随每次步骤通知重复发送。重新打开会话通过消息、运行和步骤查询恢复展示。
工作区、会话有未结束执行时，禁止归档、删除和移动等影响执行的操作；标题及置顶可在执行中修改。
工具与审批使用 `step.created`、`step.updated`、`tool.output`、`run.updated`、`approval.requested`、`approval.updated`；压缩通过现有执行和步骤事件报告，摘要不进入回答正文；补充输入发送 `user_input.requested`、`user_input.updated`，回复通过 `user_input.submit` 提交。

联调示例（替换响应中的实际 ID，保持 stdin 打开）：

```json
{"jsonrpc":"2.0","id":"c1","method":"conversation.send","params":{"model_id":"<model_id>","content":[{"type":"text","text":"你好，请介绍一下你自己"}]}}
{"jsonrpc":"2.0","id":"c2","method":"conversation.messages","params":{"conversation_id":"<conversation_id>"}}
{"jsonrpc":"2.0","id":"c3","method":"conversation.send","params":{"conversation_id":"<conversation_id>","content":[{"type":"text","text":"继续解释"}],"stream":false}}
{"jsonrpc":"2.0","id":"c4","method":"step.list","params":{"run_id":"<run_id>"}}
{"jsonrpc":"2.0","id":"c5","method":"step.data","params":{"step_id":"<step_id>","field":"usage"}}
{"jsonrpc":"2.0","id":"c6","method":"conversation.resend","params":{"conversation_id":"<conversation_id>","message_id":"<user_message_id>","content":[{"type":"text","text":"修改后的问题"}]}}
```

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
stdio 输出队列最多 64 条，单次写入或等待队列空间超过 5 秒会中断该连接并清理它发起的执行。
事件订阅还有独立的 64 条队列；订阅溢出会立即断开对应连接，避免慢消费者占用共享执行资源。
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


## 上下文压缩

Go 服务支持 `conversation.compact` 手动压缩，并在每次回答模型请求前按设置自动检查有效上下文。手动压缩复用会话互斥、全局执行容量、`run.cancel`、执行与步骤查询，不创建用户消息或回答消息。原始消息、图片路径、模型结果和工具操作审计保留。

`context_compaction` 是独立应用设置组，使用 `settings.get` 和携带 revision 的 `settings.update` 管理：

| 字段 | 默认值 | 范围与含义 |
| --- | --- | --- |
| `enabled` | `true` | 是否自动压缩；关闭后仍可手动触发 |
| `token_budget` | `0` | 0 表示从模型窗口推导；正数为显式输入预算，最多 1000000000；不能扩大已知窗口推导出的预算 |
| `output_reserve` | `4096` | 输出预留；模型声明了 max_output_tokens 时取两者较小值 |
| `safety_margin` | `1024` | 已知窗口额外扣除的安全余量 |
| `image_token_estimate` | `8192` | 每个图片块的规划估算，不按路径或 Base64 大小计量 |
| `retain_turns` | `4` | 保留最近 1 至 32 个已结束用户轮次；当前问题始终保留原文，工具轨迹可按已完成批次生成步骤摘要 |

设置在受理事务中固定，模型容量在配置解析时复制；后续设置和模型更新不影响当前执行。output_reserve、safety_margin、image_token_estimate 均为 1 至 1000000000 的整数；更新时六个字段全部必填，旧阈值字段不再接受。

已知窗口时，输入预算为 `context_window - min(output_reserve, 已声明的 max_output_tokens) - safety_margin`；未声明输出上限则直接扣 output_reserve。token_budget 为正数时进一步取较小值；窗口未知则 token_budget 必须为正数，此值直接表示已扣除预留后的可用输入预算。预留量是规划策略，不是厂商容量声明，也不自动设置输出参数。

每轮对完整请求计算 Token 估算，包括有效文本、已有摘要、图片估算、工具定义、工具调用及结果、服务端操作事实。估算超过预算，或消息数量、上下文及实际请求字节超限时触发压缩，压缩后再次检查全部边界；必须保留的内容仍超过边界时明确失败。没有有效预算时异步执行失败，error_kind 为 token_budget_unconfigured；无法在完整轮次边界内满足预算时为 token_budget_exceeded。关闭自动压缩后，普通回答只检查资源安全边界；手动压缩的模型请求仍要求有效预算。

本地 unicode-v1 是可审查的启发式估算：ASCII 字母、数字和空白连续段每三字符估一 Token、标点单独计数，其他字符按每码点二 Token、四字节码点四 Token，加上消息和工具封装预留；不计算统一消息 JSON 的转义长度。它不等同于厂商 tokenizer，图片单图估算也需按实际模型调整。同一次执行中，上一轮实际 prompt_tokens 只可参考完全相同的消息前缀和工具定义，新内容仍估算；压缩、工具定义变化、图片前缀或 usage 缺失时不复用，不以较小的 usage 降低估算。实际 usage 不混入估算，不累加历次用量作为上下文占用。远端容量错误明确失败，不自动重发模型或工具操作。

```json
{"jsonrpc":"2.0","id":"compact-settings","method":"settings.get","params":{"key":"context_compaction"}}
{"jsonrpc":"2.0","id":"compact-save","method":"settings.update","params":{"key":"context_compaction","revision":1,"value":{"enabled":true,"token_budget":0,"output_reserve":4096,"safety_margin":1024,"image_token_estimate":8192,"retain_turns":4}}}
{"jsonrpc":"2.0","id":"compact-run","method":"conversation.compact","params":{"conversation_id":"<conversation_id>"}}
{"jsonrpc":"2.0","id":"compact-versions","method":"conversation.context_versions","params":{"conversation_id":"<conversation_id>","limit":50,"offset":0}}
{"jsonrpc":"2.0","id":"compact-detail","method":"conversation.context_version","params":{"conversation_id":"<conversation_id>","id":"<context_version_id>"}}
```

写设置时必须使用实际读取到的 revision。压缩受理只返回 `conversation_id`、`run_id`，保持 stdin 打开并等待执行终态。没有较早完整轮次时不调用模型，步骤结果为 `compressed:false`。手动压缩可指定 `model_id`，省略时使用会话当前模型；自动压缩使用本次回答模型。摘要调用不提供工具，原始响应和实际 usage 独立保存在 `context_compression` 下的 `model_call` 子步骤，父步骤不重复计量。

摘要版本保存本次新增覆盖的消息 ID、运行 ID、顺序、消息与工具轨迹指纹，通过 `base_version_id` 追溯较早摘要。激活时在一个事务中重查来源、切换唯一 active 版本并完成编排步骤。取消、失败或重启不会激活未完成产物；已经提交的有效版本保留。编辑重发在同一受理事务中使覆盖被编辑消息的摘要失效，并清除其摘要正文及生成该摘要的模型结果副本；未受影响的有效摘要及其模型结果继续保留。

后续回答使用“有效摘要＋未压缩历史”；摘要作为有来源的历史数据，不作为权限或新的用户指令。待核实的工具操作另从服务端审计记录读取：摘要错误地声称成功也不能解除重新确认要求；单次最多恢复 4096 个待核实固定操作，超限明确失败。摘要校验结构和大小，不能证明语义完全无损；原文可通过消息、步骤接口核对。图片来源指纹覆盖消息中的路径，不创建图片内容快照或删除图片。

摘要为包含 `goals`、`constraints`、`decisions`、`facts`、`open_questions` 五个字符串数组的 JSON 文本，最多 64 KiB。`size_stats` 记录压缩前后的消息数、JSON 字节数及 estimated_before_tokens、estimated_after_tokens、estimation_method；估算针对被摘要替换的历史部分，不是完整请求或 usage。摘要 Token 估算未减少、结构不合法、超时、来源变化或保存失败均使本次压缩失败，旧有效版本继续保留。摘要请求自身也按 Token 预算规划源批次，并保留每批最多 248 条模型消息、7 MiB 的安全边界；沿完整轮次边界持续分批，手动压缩直到没有可压缩的较早轮次，自动压缩直到请求回到预算内；不设固定批次上限。每批必须消费新的来源范围并缩小摘要 Token 估算，无进展时明确失败。当前问题、正在执行的工具批次、保留轮次或不可拆分的单个批次无法满足预算或安全边界时明确失败。

执行内步骤摘要保存在 `context_compression.output.checkpoint`，格式版本为 `1`，记录来源执行、父检查点、覆盖步骤范围、批次指纹、结构化摘要及大小统计。摘要请求每批最多 248 条来源消息、7 MiB；提交时在短事务中复查完整批次、来源指纹和父检查点，并原子完成压缩步骤。摘要必须同时减少 JSON 字节和估算 Token，失败或取消保留此前有效检查点。后续请求和重启恢复使用“步骤摘要＋未覆盖工具轨迹”；当前问题、未完成批次和独立操作事实不被摘要替代。原始步骤、模型结果和真实 usage 保留，无新增数据库表或迁移。旧库中的超长轨迹可在恢复阶段分批压缩，不自动重放工具操作。

压缩增加模型调用耗时与费用。自动压缩无需前端新入口；桌面客户端需更新随附 Go 程序才能使用该能力，本仓库不会替换另一个仓库中的二进制。

## Agent 工具与审批

目录授权由服务端根据已固定的实际目标与影响类别推导（真实绝对目录 `root_path`、`access: read/read_write`），不改变原始操作快照与指纹，历史待核实操作继续匹配原身份；仅对 on-request 下因工作区外普通修改而询问的有效操作提供；审批对象顶层 `directory_scope` 存在时客户端才可展示选项。新普通修改授权提供包含读取的 read_write；历史 read 授权继续保留、可查询和撤销，但默认纯读取不再依赖它；不隐式扩大为删除、外发、未知操作或命令执行权限。

`approval.decide` 的可选 `scope` 默认 `once`；选择 `conversation_directory` 时，审批决定与 `conversation_directory_grants` 授权在同一事务保存。授权绑定当前 conversation_id，跨提问及重启保留，其他会话不继承。历史一次性审批不会自动升级。授权范围是实际目录路径，目录改名不会自动迁移授权；判断使用路径层级而非字符串前缀，并覆盖多文件计划及辅助路径，符号链接按实际目标校验。

`approval.directory.list` 按 conversation_id 分页查询（limit 默认 100、最大 200，offset 默认 0），返回包含撤销记录的数组；记录包含 id、conversation_id、approval_id、scope、created_at、revoked_at。`approval.directory.revoke` 接收 conversation_id 和 id，重复撤销幂等，不改变来源审批。创建及撤销发布 `directory_grant.updated`，通知包含 conversation_id 与 directory_grant。工具步骤 input.directory_grant_ids 和 authorization.reason=conversation_directory_grant 记录继承来源，不伪造新的用户决定。工具认领事务再次核验撤销状态及范围；撤销不撤回已开始或已单独批准的操作，只使依赖该目录授权的后续操作重新判断。被拒绝或结果待核实的操作、always/never 策略继续执行各自的限制。


`internal/agent` 接替原 `internal/run` 的执行管理职责，并编排模型与工具循环；数据库表名、`run_id` 和现有事件名称保持兼容。
所有模型默认收到已注册工具定义并自主选择是否调用，不需要工具能力开关；流式参数全部组装并校验后，Go 才执行权限判断及操作。
所有内置文件和命令工具的参数必须为单个 UTF-8 JSON 对象，不超过 16 KiB；统一拒绝重复字段（含嵌套对象）、未知或大小写不匹配的字段、`null`、类型错误、过深嵌套及尾随内容。业务默认值、路径和数值范围由各工具校验，`Validate` 与 `Prepare` 采用同一解析与业务规则。三个读取工具此前接受的重复字段现在返回参数无效；合法请求的参数规则保持一致。三个读取工具的当前定义版本为 3，说明文字更新为只读策略；新准备操作使用新定义计算指纹，历史定义、操作指纹和授权记录不重写；目录选项仍从已固定的目标快照推导。
内置 `list_directory`、`read_file`、`search_text`、`apply_patch`，同轮工具按模型返回顺序串行执行，工具步骤以该轮 `model_call` 为父步骤。
模型步骤保存实际请求参数、可用工具定义和上下文指纹，不重复保存整套历史或凭据。流式工具调用的供应商未知扩展字段按属性保留。
工具定义种类数不设本地固定上限，由实际请求大小和远端协议校验。工具参数各不超过 16 KiB，响应中调用 ID、名称及参数的累计字段字节不超过 8 MiB；上游原始响应仍受 16 MiB 限制，流式与非流式采用相同调用字节边界。执行前检查整批调用及最小结果结构能否容纳，每次取得真实工具结果后再次检查累计上下文；超限时保留已发生操作的审计与结果，不自动重试。当前问题保留原文，正在执行的整批工具调用和结果不被拆开。较早已完成批次可生成步骤摘要；每轮模型请求前保留最近完整批次，容量不足时明确失败。
模型调用轮次、累计工具调用次数及单次响应提出的工具调用数量均不设固定上限。执行持续到模型完成回答、用户取消、连接或应用退出，或发生无法继续的错误；总模型正文仍受 1 MiB 上限约束，辅助标题独立记录。并发容量、单次模型及工具超时、审批和 Token/内容边界继续生效。
每轮模型请求单独取得并释放请求槽位。审批等待不设置过期时间，也不持有事务或模型请求槽位；由用户决定或执行取消结束等待，仍占用全局活动执行容量和会话互斥。

文件工具支持绝对路径；相对路径必须是工作区内的本地路径，不接受 `..` 越界，工作区内的绝对路径采用相同审批规则。
无工作区时不会使用服务进程当前目录解析相对路径，而是返回 `path_base_required`，要求绝对路径或绑定工作区。
符号链接按解析后的实际目标分类。纯读取不受工作区写入边界限制，`on-request` 和 `never` 下无需逐次审批，可访问当前系统用户可读的本地路径；`always` 仍逐次询问。普通修改在工作区外或无工作区时需要审批，已有匹配的会话目录写授权可复用；`never` 拒绝仍需审批的修改。
准备调用时只固定句柄和元数据，不读取正文或目录条目；单文件固定文件句柄，目录通过 `os.Root` 限制在此次目标目录内。
执行前重新检查路径、目标身份及工作区状态；目标被替换或链接被重定向时拒绝执行，默认批准单个文件不授权其父目录及相邻文件；目录及子目录授权必须由用户另行明确选择。
单个文本文件最多读取 1 MiB；`read_file` 默认读取 200 行、单次最多 1000 行；工具结果最多 64 KiB，参数最多 16 KiB。
`list_directory` 默认返回 100 项、最多 200 项。`search_text` 执行字面匹配，默认最多 50 条、可设至 100 条；
最多检查 1000 个路径、32 层深度和累计 8 MiB 文件内容，跳过符号链接与 `.git`，通过 `truncated` 表示省略。
搜索空目录正常返回空匹配；遍历中的空子目录不会中断其他文件的搜索。
实际读取和搜索仍可能因文件被删除、权限变化或内容不再可读而失败，工具失败以结构化结果回传模型。

主对话的每轮模型请求加入固定系统提示，要求在一组工具操作前用 1～2 句话说明目的，在完成阶段、发现问题或改变方向时概括已确认的发现和下一步；连续检查相关文件合并说明，普通问答直接回答，用户可要求减少过程说明。
提示位于 `internal/agent/service/prompt.go`，通过 `loop.go` 的固定上下文每轮发送，参与输入 Token 估算及预算校验；历史压缩后仍重新加入，不保存为用户消息，也不作为摘要来源。辅助标题、历史摘要及兼容的 `model.invoke` 不加入该提示。较小上下文预算可能更早触发压缩或容量错误。
流式正文保存若被同一调用上下文的取消或超时打断，保留取消或超时终态；实际写入错误及终态保存失败继续报告存储错误。消息保存使用单条原子 UPDATE，避免额外隐式事务的取消回滚覆盖原错误；显式传入的调用方事务仍有效。
说明由所选模型生成，实际遵循程度取决于模型；服务端不拼接虚构的进展、不因缺少说明额外调用模型或重试工具。说明不是审批或成功凭据，工具执行和权限校验仍依据实际结果与服务端规则。

工具调用轮中的文本和原始模型结果保存在模型步骤。回答消息在工具调用轮结束后清空占位内容，下一轮继续流式输出，
客户端应以 `message.updated` 替换当前消息内容；成功结束后消息表保存最终回答。
模型与工具步骤额外保存统一的 `context_message`，后续对话恢复完整调用和结果配对；摘要标题不进入工具轨迹。
上下文轨迹读取前检查总大小和数量，不加载全部原始模型响应；编辑重发后旧工具审计保留，但旧分支不再进入上下文。
每次真实模型请求分别保存原始 usage，不累计估算值。

工具返回 `Result`，将实际 JSON 输出和副作用事实分开报告；取消、超时、执行错误或资源关闭失败不会覆盖已经得到的有效输出。
新工具步骤的 `input.operation_version` 为 `1`，`output` 保存 `result`、`operation`、可选的受控 `error` 和统一的 `context_message`。
`operation` 包含 `version:1`、`outcome`、固定操作的 `request_hash`，恢复未完成步骤时另记 `recovered:true`。
`outcome` 支持 `none`（无副作用）、`not_applied`（确认未生效）、`applied`（确认生效）、`partially_applied`（部分生效）、`unknown`（无法确认）。它与步骤及执行状态独立，失败或取消不代表操作未生效。
执行工具前保存保守的操作边界；结果写入失败或进程中断时，已启动的只读工具保持 `none`，其他已启动操作保持 `unknown`，未启动的步骤保持 `not_applied`。已有的明确事实及实际输出不被恢复覆盖。
失败、取消或中断执行中存在已生效、部分生效或未知的操作时，后续上下文在对应用户消息后恢复成对的调用与结果；同轮尚未执行的调用补齐为未执行，不恢复失败回答正文。
同一固定操作出现未知、部分生效或已生效但报错的结果后，再次调用需用户确认，权限判定原因是 `previous_result_requires_confirmation`；`never` 直接拒绝，批准仍不替代执行前校验。确认后得到明确成功结果才解除该操作的待核实状态。
上述字段复用现有步骤 JSON，不改变数据库结构版本、设置或 stdio 方法；旧成功轨迹继续兼容，旧失败记录不根据状态推断副作用。

审批表为 `conversation_run_approvals`：`id`、`run_id`、唯一 `step_id`、JSON `request`、`request_hash`、`status`、
`decided_by`、`decision_note`、`created_at`、`updated_at`、`decided_at`。
`request` 是 Go 生成的工具定义、可选工作区、规范化参数与实际目标快照，SHA-256 绑定此次操作；客户端只能提交决定、说明以及服务端提供的授权范围选项，不能替换路径或影响类别。
无工作区时 `request.workspace` 为 `null`；文件工具的 `request.target` 包含 `requested_path`（解析基准后的请求路径）、`path`（解析链接后的实际绝对路径）、`kind`（`file` 或 `directory`）及 `within_workspace`。
`ApprovalPolicy` 与 `ApprovalsReviewer` 是独立设置，保存在 `app_settings` 的 `execution` 配置组中，客户端通过接口修改，无需重启 Go 服务。
`approval_policy` 默认 `on-request`，按需询问；`always` 对每次有效工具调用询问；`never` 不询问，但需要审批的操作直接拒绝。
`approvals_reviewer` 当前只支持 `user`，不接受 `auto_review`。模型不能覆盖这些设置。
`sandbox_mode` 默认 `workspace-write`，也支持 `read-only`。只读模式下普通补丁修改另需审批，目录写授权不绕过此限制。旧全局配置缺少该字段时，新任务按 `workspace-write` 受理；查询返回明确默认值而不改写存储或 revision。历史执行快照保持缺失，命令延续逐次审批；旧客户端更新原有两字段时保留当前沙箱模式。无需新增数据库列或改写历史记录。
`--approval-mode`、`--approval-policy`、`--approvals-reviewer` 已移除，启动参数不覆盖数据库中的用户选择。

应用设置表 `app_settings` 的字段为 `id`（UUIDv7）、唯一 `key`、JSON 对象 `value`、正整数 `revision`、`created_at`、`updated_at`。
当前注册 `execution`（`approval_policy`、`approvals_reviewer`、`sandbox_mode`）与 `context_compaction`（见上下文压缩章节）配置组；新增配置内容通过业务代码定义类型和校验，不为每个设置项增加数据库列。
迁移只初始化新设置组，重复启动保留用户配置。值上限 16 KiB；缺少必要字段、未知字段、重复键、错误类型和不支持的值均拒绝。
`settings.get` 接收 `key` 并返回完整记录。`settings.update` 接收 `key`、读取到的 `revision` 和完整 `value`，成功后返回版本递增的记录。
过期版本返回错误码 `1003`，客户端重新读取后保存；非法参数返回 `-32602`。写入失败不改变值或版本。

每次受理在同一事务中读取设置并写入 `conversation_runs.settings_snapshot`，快照包含审批组的 `revision`、实际权限值，以及嵌套的 `context_compaction` 设置及其独立 `revision`。旧快照缺少压缩设置时保持缺失，不补造历史策略。
设置更新完成后发起的新执行使用新值；正在执行或等待审批的任务继续使用各自快照，后续轮次也不会切换策略。
配置读取或校验失败时不创建会话、消息、执行和步骤，也不回退到默认权限。
快照由服务端生成，受理回调及事件消费者不能通过共享指针修改执行器内的配置。

工具在可信定义中声明 `effects`，支持 `read`、`write`、`destructive`、`external`、`unknown`，可包含多个类别。
参数及目标校验通过后，纯读取和工作区内普通修改按规则自动放行；工作区外及无工作区普通修改、删除、大范围覆盖、外发和影响未知的操作需要审批。只读与其他影响混合时，读取类别不能抵消其他审批要求。
任何破坏性、外发或未知类别都不能被同时声明的读取类别抵消；未声明类别按未知操作处理，无效类别不允许注册。
普通修改只适用于经工具实现确认的常规文件修改；影响较大的覆盖必须声明 `destructive`，不能仅声明 `write`。
这些类别不能由模型参数声明，默认批准只授权固定目标的本次操作；用户明确选择会话目录授权时保存独立授权，后续操作仍须通过参数、路径及影响校验。
当前提供三个读取工具、`apply_patch` 创建修改工具和 `run_command` 命令工具。独立的删除及外发工具尚未接入；命令保留 `unknown` 分类，审批另结合服务端固定的系统沙箱边界判断。

每个工具步骤的 `input` 保存 `settings_revision`、`approval_policy`、`approvals_reviewer`、`sandbox_mode`，以及 `authorization.action`（`allow`、`ask`、`deny`）和规则标识 `authorization.reason`。
判定是执行前的规则结果；`ask` 的实际用户决定通过关联审批记录查询。自动放行及策略拒绝不会伪造用户审批记录。
文件目标由可信工具实现独立固定和校验，通用执行器不要求工具依赖工作区或文件系统；`sandbox_mode` 支持 `workspace-write`（默认）与 `read-only`，macOS 命令使用系统沙箱；未提供永久“完全访问”模式。
模型请求及工具执行仍分别受 `--call-timeout`、`--tool-timeout` 限制；审批等待没有超时配置。
审批状态为 `pending`、`approved`、`rejected`、`cancelled`。批准后再次校验权限与取消信号，并在事务中认领尚未执行的工具步骤。
相同决定重复提交保持幂等，冲突决定返回错误；本机用户决定记录为 `local_user`，不是外部身份认证。
拒绝转为结构化结果交回模型，同一执行内相同规范化操作不会再次请求审批或自动执行。
取消或关闭会结束等待，保存待审批记录的取消状态；已批准的记录保留，批准不表示操作已完成。
重启恢复中断状态及新工具步骤的保守操作事实，不自动重试工具。

手工验证时使用独立开发数据目录，已有厂商配置后创建模型即可测试工具。以下示例绑定工作区，相对路径与工作区内绝对路径均可使用：

```sh
go run ./cmd/callum --data-dir .callum/dev-approval
```

读取设置后，用实际返回的 `revision` 保存整组设置；下面以新数据目录的版本 `1` 为例，切换为每次审批：

```json
{"jsonrpc":"2.0","id":"settings-read","method":"settings.get","params":{"key":"execution"}}
{"jsonrpc":"2.0","id":"settings-save","method":"settings.update","params":{"key":"execution","revision":1,"value":{"approval_policy":"always","approvals_reviewer":"user"}}}
```

```json
{"jsonrpc":"2.0","id":"1","method":"model.create","params":{"provider_id":"<provider_id>","display_name":"工具模型","remote_model_id":"<remote_model_id>","enabled":true,"model_type":["text"]}}
{"jsonrpc":"2.0","id":"2","method":"workspace.create","params":{"name":"项目","root_path":"/absolute/path/to/project"}}
{"jsonrpc":"2.0","id":"3","method":"conversation.send","params":{"model_id":"<model_id>","workspace_id":"<workspace_id>","stream":true,"content":[{"type":"text","text":"读取 main.go 并解释它的作用"}]}}
{"jsonrpc":"2.0","id":"4","method":"approval.list","params":{"run_id":"<run_id>","status":"pending"}}
{"jsonrpc":"2.0","id":"5","method":"approval.get","params":{"id":"<approval_id>"}}
{"jsonrpc":"2.0","id":"6","method":"approval.decide","params":{"approval_id":"<approval_id>","decision":"approved","decision_note":"允许本次读取"}}
```

收到 `approval.requested` 后查看其中 `approval.request`，批准或以 `decision: "rejected"` 拒绝；审批 ID 来自通知或查询，
每次 `conversation.send` 的受理响应仍先于该执行事件。保持 stdin 打开直到终态，`approval.updated` 表示决定已保存，`run.completed` 才表示整次执行成功。
`Runtime.Approvals()` 提供相同业务能力，其他通信入口无需复用 JSON-RPC 类型。
不绑定工作区时，可省略 `workspace_id`，在问题中提供测试文件的绝对路径；模型调用文件工具后按上述审批流程处理。完整请求示例见 [JSON-RPC 测试文档](docs/jsonrpc.md#11-工具调用及审批)。
工具协议已通过本机模拟回归；用户提供的 `newidea / deepseek-flash` 配置运行日志显示，流式工具调用、逐次审批、文件和目录读取、结果回传及继续回答均已完成。这只验证该次端点与模型组合，不代表其他配置均已验证。该次真实端点记录不包含新增补丁工具的验证；补丁和命令工具使用受控模型、真实临时文件、受控进程和 stdio 回归验证。上下文压缩通过受控模型及临时数据库回归验证，尚未验证真实厂商的摘要质量；用户补充输入已通过受控模型和独立临时数据库验证；MCP 与插件仍属于后续范围。

### 文件创建与修改

`apply_patch` 是模型可调用的工具，不是新的 JSON-RPC 方法。通过普通的 `conversation.send` 描述创建或修改要求；模型自主调用读取和补丁工具，调用链、审批和结果仍使用现有步骤及事件。

工具参数为 `patch` 字符串和 `base_versions` 数组。补丁支持 `*** Begin Patch`、`*** Add File: path`、`*** Update File: path`、`@@`（可附精确的定位行）、`*** End of File` 和 `*** End Patch`；正文的空格、`-`、`+` 分别表示上下文、移除和插入。更新仅接受唯一精确匹配，不模糊匹配或自动合并。删除与移动标记直接拒绝。

`read_file` 无论读取哪个行范围，都返回本次读取的整个文件的 `sha256`。每个更新目标在 `base_versions` 中提供相同路径及这个哈希；新增目标不得提供版本且必须不存在。新增缺失父目录属于同一次固定操作。新增内容使用 LF；已有文件保留未改动行的换行，新行沿用邻近格式，并保留被替换文件尾原有的无换行状态。

```json
{"patch":"*** Begin Patch\n*** Add File: notes.txt\n+hello\n*** End Patch\n","base_versions":[]}
```

```json
{"patch":"*** Begin Patch\n*** Update File: notes.txt\n@@\n-hello\n+hello Callum\n*** End Patch\n","base_versions":[{"path":"notes.txt","sha256":"<read_file 返回的 sha256>"}]}
```

上述对象展示模型工具参数，不能直接作为 stdio 请求发送。模型参数仍限 16 KiB，工具结果限 64 KiB；每次补丁最多 8 个文件、16 个缺失父目录，目标路径最多 1024 字节、计划 JSON 最多 24 KiB，每个输入和结果文本文件最多 1 MiB。只接受 UTF-8 且不含 NUL 的文本，拒绝特殊文件、硬链接、带特殊权限位或 Linux 文件 capabilities 的目标；Windows 同时拒绝设备路径和备用数据流。

多目标快照使用 `approval.request.plan.targets`，其中包含文件、拟创建目录、实际路径、`action`、`expected_sha256` 以及固定的 `recovery_path`、`staging_path`；旧读取工具继续使用单个 `request.target`。可信计划的额外影响合并进工具 `effects`，不能抵消原有类别；任一目标越出工作区即按工作区外访问处理。准备和审批等待不读取正文、不创建目录或辅助文件。

服务端保守地将可能覆盖全部原文件，或移除字节上限至少 4 KiB 且达到原文件一半的更新归为 `destructive`。字节上限按每条移除行正文加最多两个换行字节计算；小文件也可能因此需要审批。请求路径或实际路径含 `.git`、`.codex`、`.agents` 目录时增加 `unknown` 影响。普通工作区创建、小范围修改仍按现有策略处理，模型不能自行降低分类。

批准后先检查全部目标的身份、版本及补丁上下文，全部通过才写入。重叠目标在同一运行时串行写入，等待锁可取消，不占用审批等待时间；跨外部编辑器采用乐观并发检查，不提供事务隔离。通过原文件句柄写入以保留已有所有者、权限及 ACL；单文件也不承诺原子替换。取消在已经开始的一次有界写入、截断和同步后处理，再停止后续文件，系统写入错误仍可能留下部分内容。

写入前在目标父目录生成 `.callum-patch-<操作指纹>-before` 原文副本（仅更新）及 `-after` 预期内容副本，并同步内容；Unix 使用 0600，macOS 同时删除新对象的继承 ACL，Windows 设置当前进程用户的受保护 DACL，新增目录使用私有权限。已有同名辅助文件不会被覆盖。辅助路径可重复计算并绑定审批，不能通过随机名字改变同一操作的重试指纹。成功后清理副本；部分写入、结果未知或清理失败时返回保留副本的位置，供人工核实，不自动回滚、清理遗留副本或重试。

结果 `changes` 逐文件保存路径、操作、`outcome`、实际读取到的 `before_sha256`、`intended_sha256` 和可获得的 `observed_sha256`，另有 `created_directories`、`recovery_files`。失败或取消不会清除已发生的修改；进程异常退出时未完成步骤保持保守未知，固定辅助路径可从步骤 input 查询，不宣称能够还原未持久化的精确进度。手动核实后再决定保留文件、恢复原文或发起新的补丁。

macOS 的文件读写、权限、ACL、取消、冲突、并发及 stdio 流式链路使用临时资源验证；Linux、Windows 交叉构建和测试编译与各自原生运行验证分开记录。当前环境没有 Linux/Windows 原生运行结果。


### 非交互命令执行

`run_command` 由模型在 `conversation.send` 中调用，不是独立的 JSON-RPC 方法。参数示例：

```json
{"command":["go","test","./..."],"cwd":"."}
```

`command` 是可执行文件及参数数组，最多 256 项，整个参数 JSON 继续限 16 KiB；空参数合法，空可执行文件、NUL、重复键、未知字段及非字符串项被拒绝。
`cwd` 省略时使用绑定工作区；相对路径以工作区为基准且不能越界，未绑定工作区时必须明确提供绝对 `cwd`，不使用 Callum 进程目录。
可执行文件支持绝对路径、相对于实际工作目录的路径，或在服务端固定 PATH 中查找名称；忽略空和相对 PATH 项。Windows 直接启动 `.exe` / `.com`，无扩展名依次查找这两种格式，不隐式执行 `.bat` / `.cmd`。
不自动套 Shell。需要 Shell 语法时，在数组中明确指定相应解释器及其参数；解释器自身的转义规则仍由调用方遵守。stdin 连接空设备，无 PTY、后续输入或持久后台会话。

环境由 app 显式传入并在工具装配时筛选，只保留 PATH、HOME、USERPROFILE、TMPDIR、TEMP、TMP、SYSTEMROOT、WINDIR、COMSPEC、PATHEXT、LANG、LC_ALL、LC_CTYPE，另按实际目录生成 PWD；Windows 名称不区分大小写。模型不能设置环境变量。API Key、令牌、代理凭据等任意环境项不会继承。
审批 `request.details` 保存解析后的可执行文件、身份及大小/修改时间、实际 cwd 及身份、环境名单与配置摘要，不保存环境值；规范化参数保留在 `request.arguments`。详情不超过 16 KiB，参与 `request_hash`，执行前重新比较并检查原路径、句柄、工作区和可执行文件元数据。未实现补充快照的文件工具省略 details，原审批格式与指纹保持兼容。
此检查不锁定整个软件依赖图，也不承诺执行前检查与路径启动之间的原子隔离。

命令 effects 仍为 `unknown`，不按 `ls`、`cat` 等命令文字伪装为读取。服务端另固定实际执行边界：macOS 的新任务默认通过 `/usr/bin/sandbox-exec` 使用 Seatbelt；命令及后代可读取当前用户可读的文件，网络关闭，`workspace-write` 仅开放真实项目及每次命令的私有临时目录写入，`read-only` 或未绑定项目仅开放该临时目录写入。项目路径下 `.git`、`.codex`、`.agents`、`.aws` 的直接写入被拒绝。常用 Go/npm 缓存定向到私有临时目录，结束后删除；未安装的依赖不能通过命令联网下载。模型客户端的网络请求不受命令沙箱限制。

`on-request` 自动执行普通沙箱命令；已识别的删除、权限修改及敏感 Git 命令仍询问，批准后也保留原沙箱。该识别不保证分析任意脚本的业务影响。`always` 对有效调用逐次询问；`never` 执行允许的沙箱调用并拒绝仍需审批的操作。沙箱执行失败直接保留退出码及错误，不自动在沙箱外重试。系统沙箱无法启动时明确失败，不降级为无隔离执行。

确需扩大权限时，模型必须提交 `sandbox_permissions: "require_escalated"` 及非空 `justification`。服务端生成本次沙箱外执行快照并请求独立审批；批准只允许本次固定命令使用当前系统用户的文件和网络权限，不保存命令前缀或目录授权。`never` 拒绝该请求。可选 `sandbox_permissions: "use_default"` 与省略一致。Linux/Windows 尚未接入系统沙箱，仍对每次命令请求审批，不宣称具有 macOS 的隔离能力。

Seatbelt 是共享文件系统上的路径访问边界，不是容器或独立文件系统：项目内既有文件的其他硬链接会共享修改，项目内已可写的别名也不构成独立副本；新建硬链接被默认拒绝。路径固定及执行前复核不保证抵抗同时发生的外部重命名，也不锁定程序的完整依赖图。应将项目根目录设为实际任务目录，而非整个用户目录。

进程受现有 `--tool-timeout` 限制（默认 30 秒），审批等待不计入。macOS、Linux 创建独立进程组并在正常主进程退出、取消、超时时终止组内残留成员；主动 setsid/setpgid 脱离的进程、通过其他服务启动的工作不在回收保证内；Unix 上 Callum 被强制终止或崩溃也不保证回收仍运行的子进程，重启后需按未知结果核实。Windows 10+ / Server 2016+ 创建进程时加入 Job Object，禁用普通 breakaway 并启用最后句柄关闭终止成员；Job 只负责进程生命周期，不构成文件或网络沙箱。终止后主进程与管道各有最多 2 秒收尾等待，Windows 另有最多 2 秒 Job 成员确认；清理失败明确返回错误及不完整清理状态，不宣称强制终止一定成功。

工具输出无论模型请求的 stream 为 true 或 false 都可实时推送；模型在命令结束后收到最终结果。通知示例：

```json
{"jsonrpc":"2.0","method":"tool.output","params":{"conversation_id":"<conversation_id>","run_id":"<run_id>","step_id":"<step_id>","output":{"sequence":1,"stream":"stdout","encoding":"base64","data":"aGVsbG8K"}}}
```

sequence 在单个步骤内从 1 递增；两个流各自保持读取顺序，不承诺还原跨 stdout/stderr 的原始写入顺序。客户端应持续读取 JSONL、按 step_id 分流并解码字节，不把片段当作完整 UTF-8 字符或最终成功信号。
每片最多 4 KiB 原始字节，每次调用实时推送最多 64 KiB、128 片。保存 stdout/stderr 各自前 16 KiB 原始字节，编码后各不超过 24 KiB JSON，最终结果仍不超过 64 KiB。达到上限继续排空管道，分别设置 truncated 和 stream_truncated，并保留实际读取字节数；输出片段只即时发布，不逐片落库或承诺重放。慢订阅者沿用有界队列规则，断开对应消费者，不取消共享执行；最终结果通过 step.data 查询。

结果包含 execution_boundary（sandboxed、sandbox_mode、escalated 及可选 risk）、started、pid（未启动时为 null，仅供核实，不作为重启后的进程控制凭据）、exit_code（未获得时为 null）、可选 signal、timed_out、cancelled、cleanup_complete、stdout、stderr、stream_truncated。每个输出对象为 encoding（utf-8 或 base64）、data、bytes、truncated；二进制或不能构成完整 UTF-8 的保留前缀使用 base64；UTF-8 文本按实际 JSON 转义长度和字符边界截断。
启动失败为 not_applied；正常退出且生命周期收尾成功为 applied，仅证明命令调用已经执行完毕，非零 exit_code 仍原样交给模型判断业务失败。启动后取消、超时、信号终止或结果不明为 unknown，并保留已经收到的输出。未记录结果的中断命令沿用持久化保守未知及重新审批规则，不自动重试。

`.github/workflows/verify.yml` 配置 macOS、Linux、Windows 的原生竞态测试、静态检查和无 CGO 构建；Go 版本读取 go.mod。工作流配置存在不等于运行通过，平台原生结果以实际 CI 记录为准；本地交叉编译只证明对应目标可构建。


## 用户补充输入

模型可调用 `request_user_input` 请求用户选择或补充信息，参数为 `question` 和 `options`。问题通过 `user_input.requested` 通知客户端；`user_input.get/list` 可恢复等待快照，`user_input.submit` 提交选项下标、文字或显式跳过。详细字段、边界、状态和示例见 [JSON-RPC 用户补充输入](docs/jsonrpc.md#21-用户补充输入)。

用户输入独立于工具审批，不作为文件、命令或其他操作的授权。等待时不持有模型请求槽位或数据库事务，仍受活动执行上限和会话互斥保护；问题和回复沿用现有步骤表，不新增迁移。相同回复幂等，取消、连接关闭和重启恢复后不自动继续执行。

### 模型独立推理内容

Chat Completions 已接入供应商明确返回的 `reasoning_content`，与回答正文分别交付；不解析正文标签来推测思考内容。主对话按模型步骤发布 `reasoning.delta` 并定期保存已接受的推理，结束、取消和失败时保留统一 `output.reasoning` 投影供查询。异常退出可能丢失最近尚未保存的片段；恢复不自动重新执行。工具循环通过统一消息保留并回传推理字段，输入 Token 估算和累计内容限制计入推理，实际 usage 不作改写。未返回推理内容的模型不生成思考面板。持久化沿用现有步骤 JSON，无数据库迁移；旧记录未包含统一字段时保持缺失。协议见 `docs/jsonrpc.md`。
