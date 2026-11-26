# 后端架构

本文件说明 Callum Go 后端的模块职责、依赖方向和装配边界。桌面界面、Electron 进程与客户端状态由独立的 `callum-desktop` 仓库维护。

运行和验证见 [运行与开发](development.md)，配置与存储见 [存储与模型配置](storage.md)，工具行为见 [Agent 工具与审批](tools-and-approvals.md)；通信字段与请求示例见 [JSON-RPC 协议](jsonrpc.md)；数据一致性、执行恢复、压缩与平台沙箱设计见 [对话、执行与上下文设计](design/conversations.md)。

## 模块职责

当前模块与后续规划按职责组织如下：

| 目录 | 职责 | 状态 |
| --- | --- | --- |
| `internal/model` | 模型接口、内容块、响应、usage 和流式结果契约 | 已实现 |
| `internal/model/adapter/chatcompletions` | Chat Completions 请求、认证、响应转换和流式解析 | 已实现，含模拟测试 |
| `internal/model/provider` | 厂商与模型配置类型、存储接口、错误与标识校验 | 已实现契约 |
| `internal/model/provider/service` | 配置业务校验、UUIDv7 分配及启用规则 | 已实现业务服务 |
| `internal/credentials`、`system` | 凭据加密、系统主密钥及关闭时清理 | 已实现，平台验证分别说明 |
| `internal/config` | 应用配置加载及启动参数 | 已实现 |
| `internal/id` | 统一标准 UUIDv7 文本及 RFC4122 variant 校验，业务模块保留自己的错误类型 | 已实现 |
| `internal/skills` | 从数据目录发现通用技能及生成有界目录上下文，不安装或内嵌技能 | 已实现 |
| `internal/attachment` | 托管输入和交付副本、本轮工作副本及异常退出清理 | 已实现 |
| `internal/runtimeassets`、`prepare` | 运行时清单及文件校验；开发侧固定归档下载、安全解压与资源准备 | 已实现；运行期只读，不修改全局环境 |
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

## 业务边界与执行恢复

技能正文、脚本、参考资料、解释器和原生依赖由客户端持有，默认技能安装也在客户端完成。Go 从 `DataDir/skills` 扫描 `SKILL.md`，只向模型提供名称、说明和绝对入口；模型通过通用文件与命令工具按需加载。Go 不包含 Word、Excel、PowerPoint 或 PDF 的解析、生成、编辑和渲染实现。`load_workspace_dependencies` 只返回资源清单中的通用 Python、Node、模块及原生命令目录，不依赖特定文档格式。

`attachment.Access` 为已受理执行管理私有输入副本和中间目录；命令子进程结束后由 Agent 回收，Runtime 关闭等待全部执行后再关闭访问器和附件存储。`open_attachment` 按当前分支校验文件引用；`view_image` 把固定图片保存为稳定工具上下文；`publish_artifact` 保存独立交付副本，结束前追加到回答。执行中断时，存储层在标记中断的事务内从当前分支的成功步骤恢复交付物引用。图片在适配层转换为整批工具结果之后的视觉输入，原始统一轨迹保持调用和结果配对，压缩和恢复继续复用同一轨迹契约。

附件访问的全局锁只保护执行映射、关闭状态和短时公开快照。初始化、导出、生成与配额更新按执行独立串行化，等待文件操作时可取消；一个执行的文件 I/O 不阻塞其他执行。`End` 先撤销该执行的新操作入口，再等待在途操作并回收目录；`Close` 停止新增执行并等待已预留的初始化与文件操作。

`internal/runtimeassets` 返回启动时校验的不可变资源描述，不持有文件、进程或后台任务；`Runtime` 将其借给命令工具并把实际资源根目录登记为只读保护范围。资源描述同时固定 Python、Node、Git 入口及 Git 配套资源路径，子进程专用环境由命令工具生成。命令调用仍独立持有并关闭固定入口和资源目录句柄，沿用审批、进程组/Job 与输出生命周期。`prepare` 同时由随附 `callum prepare-runtimes` 离线子命令及独立维护入口 `cmd/prepare-runtimes` 使用：前者严格禁止下载且不创建应用 Runtime，后者仅在显式 `--download` 时获取缺失归档；正常 stdio 服务不调用准备流程。

`internal/conversation/compaction` 负责完整轮次的压缩规划、来源指纹与摘要校验，执行编排位于 `internal/agent/service`。用户补充输入使用现有步骤持久化，支持选项、文字、跳过和取消。工具和审批业务不依赖 stdio，后续入口复用同一 Agent 与审批服务。
会话更新通过 `conversation.Update` 接收模型 ID 及可更新字段，业务服务校验后生成仓储使用的 `conversation.Change`，模型快照由服务端解析；JSON-RPC 的请求、响应和通知字段保持兼容。
会话服务通过自身使用的 `ExecutionCoordinator` 接口受理、继续和协调执行，由 app 注入 Agent Engine；会话更新及会话侧队列操作的提交后通知由会话服务发布。Agent 继续负责执行过程中的引导和运行事件，不代办会话管理通知。
`policy.Operation` 只保存可信的判定事实，不持有设置快照或工具资源；Agent 校验执行快照并查询授权，policy 统一计算历史拒绝、未知结果、沙箱及目录授权的最终判定，执行认领仍在事务内复核授权。
`agent/transcript` 不依赖 ORM 或通信协议；SQLite 保留有界查询、分支筛选和事务写入，历史读取与压缩检查点共用轨迹恢复规则，`conversation/history` 负责将轨迹装配到对应用户消息后。
会话展示历史由 `conversation.HistoryPage` 统一返回当前分支的执行和消息；SQLite 同一读事务按执行游标分页，独立手动压缩与普通轮次使用相同排序，不依赖固定数量的最新执行来补齐旧消息。
其他协议和供应商兼容差异按实际需求接入。

## 工具组织与契约

目录按业务与职责划分；契约保留在模块根包，具体业务、存储和通信实现进入职责明确的子包。
测试跟随对应实现，跨模块测试通过公开入口组装；不为规划能力创建空包或占位实现。
文件和命令工具分别由 `filesystem.Tools(protection)` 和 `command.ToolsWithRuntime(environment, protection, bundle)` 汇总，根包提供工具集合入口。
具体工具子包通过 `New()` 提供统一工具契约，各自负责定义、业务参数校验、准备及执行。
命令实现位于 `command/run`，工具定义、参数解析和调用生命周期分别放置；环境由装配层显式传入。
沙箱请求的公共前置判定和私有缓存环境在该包内共用，系统隔离、权限和进程清理仍由对应平台实现。
各工具通过内部 `arguments` 包统一校验 JSON 结构，共享文件访问仍由 `filesystem/internal/fileaccess` 提供。

## 配置服务、通信与装配

`provider/service.Service` 通过 `provider.Repository` 接口访问配置，由 `sqlite/catalog.Repository` 实现该接口。
GORM 映射和驱动错误处理集中在存储包，业务服务不导入 ORM 或数据库驱动。
stdio 路由调用 catalog、execution 接口处理器，后者将协议参数转换为业务输入；
protocol 和 transport 包不依赖配置业务、模型执行或 stdio 根包。
执行管理器只依赖统一模型接口，具体适配器由 app 中的工厂装配。
app 创建连接、配置服务及执行管理器，并负责资源关闭顺序。无历史调用借用共享调度器，共享调度器由 Runtime 关闭。程序入口处理参数、信号和退出码。

## 存储查询与恢复

业务仓储的操作事实、最新检查点及恢复子查询使用 GORM 组合构造，JSON 与窗口表达式继续在 SQLite 内执行。查询保持最新结果的筛选顺序、当前分支隔离、容量检查和事务清理范围；检查点继续逐行读取。

表结构、事务、迁移、审批与压缩的具体约束见 [对话、执行与上下文设计](design/conversations.md)，运行数据路径与数据库兼容行为见 [存储与模型配置](storage.md)。

## 运行时与资源所有权

`app.New(ctx, cfg, logger)` 创建独立运行时，不需要 stdin/stdout。运行时持有数据库、实例锁、HTTP 连接池、
共享调度器、凭据 Vault 和事件发布器；初始化上下文仅用于启动，已接受的执行使用应用运行上下文。
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
`Close` 停止入口、取消并等待所有任务及连接，关闭发布器、HTTP 空闲连接，清空 Vault 主密钥缓存并关闭数据库，最后释放实例锁，可以重复调用。
HTTP、WebSocket 和 CLI 入口尚未实现；接入时复用同一运行时和业务事件，各自转换请求、错误、响应和推送格式。
macOS/Linux 标准流副本与非阻塞标志的恢复封装在 `stdio/transport`，入口不直接操作文件描述符。

工具参数纠正由模型适配器报告完整响应中的参数错误，再由 Agent 服务在确认本轮未执行任何工具后编排新请求；适配器不依赖 Agent 或审批层。临时纠正信息由单次执行持有、计入请求预算，失败响应及 usage 由模型步骤持久化；不改变数据库结构。命令工具的 Shell 语法解析仅发生在参数校验阶段，不启动 Shell 或解释脚本。

## MCP 连接

运行时持有 `internal/mcp` 的连接与授权管理器，`internal/stdio/mcp` 将桌面请求转为业务操作。MCP 进程的 stdin/stdout 与 Go 面向 Electron 的 stdio 独立。远程会话使用单独的 HTTP 连接池。OAuth 发现的远程授权端点使用受限连接池，临时回调由发起连接和运行时共同持有，取消、超时和退出时释放。

启用的服务在需要工具目录时连接，MCP 工具目录与执行设置版本绑定并参与 Token 预算。已发布的工具目录以只读快照读取，缺失或过期的目录在同一执行预算内最多并行发现四个服务。发现超时后仍可使用本轮有效的其他目录。工具执行时使用本轮固定的实例。停用、删除、修改连接或更换授权会使旧调用失效。退出时停止运行并关闭 MCP 会话，再关闭凭据库、数据库与实例锁。

MCP 配置在初始化、保存输入、仓储写入和列表返回时复制切片与映射，保存的秘密环境变量也独立复制。服务内部配置不借用调用方或查询结果中的可变字段，配置变化仍须通过版本校验和连接失效流程。

MCP 在条目锁内撤销旧会话的公共快照并关闭连接。预期取消和已关闭的会话保持静默。关闭失败通过应用注入的日志记录服务 ID 和错误类别，避免第三方错误原文进入日志。已持久化的配置和调用原有的结果继续按各自的业务契约返回。

本地 stdio 会话关闭子进程输入后，最多等待 250 毫秒让子进程自然结束，随后关闭管道并清理所属进程组或 Windows Job。强制清理后最多再等待 2 秒确认主进程退出。超时会记录诊断并继续应用清理，子进程仍由原来的 `cmd.Wait` 回收。
