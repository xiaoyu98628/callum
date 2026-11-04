# 对话、执行与上下文设计

本文记录已确认的目标设计。已完成普通对话、支持可选工作区的文件读取及补丁工具循环与审批；补充输入及压缩仍为规划。实际完成情况以 README 和实现为准。

## 交付范围

1. 普通对话：工作区、会话、消息、执行、步骤、标题、模型结果与 usage、取消和重发。
2. 工具交互：工具调用循环、审批和用户补充输入。
3. 上下文压缩：手动与自动压缩、摘要版本、来源追溯和失效处理。

表、接口及对应约束随功能引入，不建立空实现。现有厂商、模型配置和 model.invoke 接口保留。

## 数据约定

- 主键由 Go 生成 UUIDv7，以 TEXT 保存；时间为 UTC Unix 毫秒。
- JSON 使用 TEXT，保存前验证结构；问号表示允许 NULL。
- 应用数据目录与工作区项目目录分开。测试使用临时目录。
- 不保存模型配置凭据到会话、运行和步骤快照。
- 不做本地 Token 预算检查，由远端校验容量；保留本地请求、响应及累计内容字节上限。
- usage 原样保存模型返回的对象，缺失为 NULL，不混入估算值。

## 表结构

### workspaces

id TEXT PRIMARY KEY；name TEXT；root_path TEXT UNIQUE；pinned_at INTEGER?；archived_at INTEGER?；deleted_at INTEGER?；created_at INTEGER；updated_at INTEGER。

root_path 为规范绝对路径。软删除保留会话；相同路径再次创建时恢复原工作区。工作区恢复不清除会话自身的归档、删除状态。有未结束执行时不允许归档、删除等影响执行的变更。

### conversations

id TEXT PRIMARY KEY；workspace_id TEXT?；title TEXT；title_source TEXT DEFAULT 'default'；model TEXT JSON；pinned_at INTEGER?；archived_at INTEGER?；deleted_at INTEGER?；created_at INTEGER；updated_at INTEGER。

首次提问被接受才创建会话，不提供创建空会话接口。title_source 为 default、manual、generated。
默认标题为首次问题文本去首尾空白后前 30 个 Unicode 码点，英文按字母、空格、标点计数；仅图片默认“新对话”。
标题生成与首次回答并行：只在默认标题且首次问题仍匹配时应用结果。人工改名不被覆盖；失败、超时或并发不足保留默认标题。
主回答结束时取消并等待尚未结束的标题任务，标题失败不导致回答失败。

model 是本地模型 id、display_name、remote_model_id 和 provider.id、provider.name 的快照。配置删除后历史可展示，后续调用重新检查有效配置。

### conversation_messages

id TEXT PRIMARY KEY；conversation_id TEXT；run_id TEXT；sequence INTEGER；role TEXT；content TEXT JSON DEFAULT '[]'；status TEXT；created_at INTEGER；updated_at INTEGER；completed_at INTEGER?。

不保留 parent_message_id。同一会话 sequence 从 1 开始且唯一；同一 run_id、role 唯一，角色为 user、assistant。
content 为有序内容块数组，例如 [{"type":"text","text":"分析图片"},{"type":"image","path":"/absolute/path/image.png"}]。
图片记录路径，附件管理不在当前范围；编辑消息不删除源文件。
用户消息保存即完成；回答正常完成后填写 completed_at，失败、取消、中断保留部分内容但不标记成功。

### conversation_runs

id TEXT PRIMARY KEY；conversation_id TEXT；replaces_run_id TEXT?；model TEXT JSON；stream INTEGER；settings_snapshot TEXT JSON?；status TEXT DEFAULT 'running'；error_kind TEXT?；error_message TEXT?；created_at INTEGER；updated_at INTEGER；finished_at INTEGER?。

状态为 running、awaiting_approval、awaiting_user_input、completed、failed、cancelled、interrupted。
同一会话最多一个未结束执行；不同会话受全局并发限制。手动压缩也可以创建执行，不要求有用户消息。
replaces_run_id 仅关联被替代执行，不保留旧消息分支。所有终态填写 finished_at。
settings_snapshot 保存受理事务读取的配置版本和实际审批设置；新执行必须保存快照，旧执行缺失时保持空值，不补造历史配置。

### conversation_run_steps

id TEXT PRIMARY KEY；conversation_id TEXT；run_id TEXT；parent_step_id TEXT?；sequence INTEGER；type TEXT；name TEXT；definition TEXT JSON?；status TEXT DEFAULT 'pending'；input TEXT JSON DEFAULT '{}'；output TEXT JSON?；usage TEXT JSON?；model TEXT JSON?；context_version_id TEXT?；provider_request_id TEXT?；tool_call_id TEXT?；error_kind TEXT?；error_message TEXT?；created_at INTEGER；started_at INTEGER?；updated_at INTEGER；finished_at INTEGER?。

context_version_id 及外键随压缩功能引入。run_id、sequence 唯一；父步骤同执行，步骤和执行同会话。
类型为 model_call、tool_call、user_input、context_compression；状态在执行状态基础上增加 pending。
Skill、MCP、插件产生的实际工具调用统一为 tool_call，definition 记录工具名称、来源实例及定义版本或内容指纹。
model_call.input 保存参数、上下文来源及内容指纹；output 保存完整结构化模型结果和协议标识。
非流式保存响应对象，流式保存组装结果，不逐包保存网络分片。部分结果不视为成功；HTTP 认证信息不入库。
每次真实模型请求记录一份 usage。回答、标题、摘要模型调用分别统计，编排父步骤不重复累计。
工具输入、输出为实际参数、结果；user_input.input 为问题、选项和要求，output 为用户答案。

### conversation_run_approvals

已随工具交互引入：id TEXT PRIMARY KEY；run_id TEXT；step_id TEXT UNIQUE；request TEXT JSON；request_hash TEXT；status TEXT DEFAULT 'pending'；decided_by TEXT?；decision_note TEXT?；created_at INTEGER；updated_at INTEGER；decided_at INTEGER?。

状态为 pending、approved、rejected、cancelled。审批绑定确定的操作，执行前核对操作摘要及当前权限；参数变化创建新步骤及审批。批准不表示执行成功。普通补充输入使用 user_input，批准、拒绝说明保存到 decision_note。

### conversation_context_versions

随压缩引入：id TEXT PRIMARY KEY；conversation_id TEXT；version INTEGER；base_version_id TEXT?；generated_by_step_id TEXT；summary TEXT?；source_manifest TEXT JSON；token_stats TEXT JSON?；status TEXT DEFAULT 'building'；created_at INTEGER；updated_at INTEGER；activated_at INTEGER?；invalidated_at INTEGER?。

状态为 building、active、superseded、invalid、failed。会话内版本号唯一且最多一个 active。
保留来源原文及工具调用、结果的配对关系；摘要验证保存成功后才事务切换。失败保留旧有效版本。
token_stats 表示压缩效果及计数方式，不是模型计费 usage；自动压缩触发依据另行确定。
触发方式记录在压缩步骤 input.trigger.type（manual、automatic）及 reason。
context_compression 编排下建立 model_call 子步骤保存生成摘要的模型结果与 usage。

## 编辑重发

仅编辑最后一条用户消息，要求原执行已结束。保留用户消息 ID、sequence，更新内容和 run_id，删除旧回答，创建新执行与回答。
清除旧模型结果副本和摘要中的被替换消息内容；保留实际工具操作、结果及审批事实。不删除源图片。
有关写入在事务内完成，网络调用在事务外。后续版本依赖被改动来源时一并失效。

## 通信

业务服务和事件契约不依赖通信协议。`app.New` 创建共享运行时，无需输入输出；业务调用方直接调用服务并订阅事件。
当前接入为 JSON-RPC 2.0，stdin/stdout 每行一个 JSON。请求仅返回一次受理响应，返回 conversation_id、run_id 等 ID 后，再推送无顶层 id 的服务端通知。
事件名位于 method，params 按事件携带 conversation_id、run_id、step_id、message_id、approval_id。
message.delta 使用 block_index 和带 type 的 delta，标题、工具、摘要输出不混入回答正文。

拟定事件：conversation.created、conversation.updated、run.started、run.updated、run.completed、run.failed、run.cancelled、message.created、message.delta、message.updated、step.created、step.updated、step.output_delta、approval.requested、approval.updated、user_input.requested、user_input.updated。

压缩使用 type=context_compression 的步骤事件。步骤父子关系支持折叠、展开，折叠偏好由客户端保存。
状态落库后通知；内容增量有界传输，终态前保存最终或部分结果。重新打开查询数据库状态，不承诺逐事件重放。
旧 model.invoke 保留 run.text_delta、run.completed、run.failed、run.cancelled 的兼容格式。

`run/events` 提供按会话、执行和类型过滤的有界订阅，运行时最多 32 个订阅，各排队 64 个事件。
慢订阅者溢出即断开，其他订阅和业务执行继续。事件只读；大型步骤原始输出不进入订阅队列，公共事件不保存原始错误链。
会话更新由业务服务在提交后发布，直接调用和 stdio 调用具有相同通知行为。
连接退出时只取消并等待该连接发起的任务，其他调用方不受影响；独立 stdio 进程入口仍在连接退出后关闭整个运行时。
HTTP、WebSocket 和 CLI 接入尚未实现，后续入口复用业务服务及事件，并在各自边界转换协议。

## 资源、迁移与验证

创建、编辑和状态变更按业务动作定义事务，不在等待模型、工具、用户输入时占用事务。
关闭时停止接收、取消并等待任务、结束待审批记录，再关闭通信、HTTP 连接池和数据库。恢复中断执行前确认数据目录实例所有权。
迁移为 000001_create_model_config.go、000002_create_conversation_core.go、000003_create_run_approvals.go、000004_create_app_settings.go；当前结构版本 4，版本 1、2、3 经校验后保留数据升级；压缩随功能增加具体迁移。
开发阶段不转换旧 model_type 功能标签；模型配置使用内容类型规则校验。
版本判断同时验证实际结构，兼容当前版本 1 的 UUIDv7 配置库；迁移失败回滚，不重建用户数据库。
验证包括迁移、约束、事务回滚、多会话隔离、取消、重发、标题竞争、流式异常、资源关闭及 usage 保存，并运行项目规定的完整检查。


## 工具与审批实现约定

业务编排位于 internal/agent，内置工具位于 internal/tool/builtin/filesystem；数据库表名和通信 run 标识保持兼容。
普通对话及 Agent 默认向所有模型提供已注册工具，不依赖 model_type、capabilities.tools 或工作区绑定，模型自主选择是否调用。辅助标题与兼容 model.invoke 保持文本用途。每轮最多一个回答模型请求，同轮工具串行；
model_type 是 text、image、audio、video 内容类型元数据，可多选，不作为执行开关；图片输入由 capabilities.image_input 明确声明，推理等级只作元数据。
回答最多 8 轮模型调用、32 次工具调用。具体工具为 list_directory、read_file、search_text、apply_patch，支持绝对路径；相对路径需要工作区基准，不接受 .. 越界。无工作区的相对路径返回 path_base_required。
工作区内绝对路径与相对路径遵守相同规则，符号链接按实际目标分类。单文件固定文件句柄，目录操作通过 os.Root 限定在单次目标目录内，目录搜索不跟随子项符号链接。
准备阶段不读取正文或目录条目；执行前重新检查请求路径、目标身份及工作区。目标或链接在等待审批期间被替换时拒绝执行。
模型调用与工具执行分别使用 call-timeout、tool-timeout，默认 5 分钟和 30 秒。审批没有过期时间或超时配置，等待用户决定或执行取消。
审批等待保留有界执行容量和会话互斥，但释放模型请求槽位，不持有数据库事务。

应用设置保存在 app_settings：id TEXT UUIDv7 PRIMARY KEY；key TEXT UNIQUE；value TEXT JSON 对象；revision INTEGER 默认 1 且大于零；created_at INTEGER；updated_at INTEGER。
当前配置组 key=execution，value 保存独立的 approval_policy、approvals_reviewer，不为每个设置项增加数据库字段。
settings.get 读取配置组，settings.update 携带 key、当前 revision 和完整 value 原子替换；冲突返回 1003，非法参数返回 -32602，失败不改变配置。
受理执行与读取设置在同一事务中完成，保存 settings_snapshot；更新成功后发起的新任务读取新值，已受理任务的审批策略保持不变，无需重启。
approval_policy 支持 on-request（默认）、always、never；approvals_reviewer 当前只支持 user，不接受尚未实现的 auto_review。
启动时保留用户选择；读取异常阻止受理，不回退默认权限；旧执行缺失快照不伪造数据。
on-request 放行工作区读取和普通修改，无工作区或工作区外文件访问以及删除、大范围覆盖、外发或未知影响需要询问；always 逐次询问；never 将需要审批的操作直接拒绝。
工具 effects 由可信代码声明，可包含 read、write、destructive、external、unknown；缺少类别按未知操作处理，无效类别拒绝注册。
混合类别采用需要审批的结果，读取声明不能抵消破坏性或外发影响；工具负责将影响较大的覆盖声明为 destructive。
当前已提供三个读取工具及 apply_patch 文件创建、修改工具；删除及外发操作类别使用受控工具验证，对应产品工具尚未接入。
参数、目标范围及工作区状态独立校验；批准只授权本次固定目标操作，不扩大到其他路径。策略拒绝和参数校验失败不生成用户审批记录。
权限判定属于 Go，模型不能声明自己已获授权。旧 approval-mode、approval-policy、approvals-reviewer 启动参数移除，未加入操作系统沙箱或完全访问模式。
工具步骤 input 保存 settings_revision、approval_policy、approvals_reviewer 和 authorization（action、reason），供审计查询；审批表结构和通信接口保持不变。
request 保存服务端生成的工具完整定义、版本、可选工作区、实际目标及规范化参数，request_hash 为该快照的 SHA-256。无工作区时 workspace 为 null；target 保存 requested_path、path、kind 和 within_workspace，不能由模型参数覆盖。
approval.get/list 查询记录，approval.decide 只接受 approval_id、decision、decision_note；批准或拒绝身份由服务端填写为 local_user。
相同决定幂等，冲突决定拒绝。批准与取消竞争时，仍需在执行前检查上下文、工作区及操作指纹，数据库原子认领保证同一步骤不会启动两次。
拒绝转为结构化工具结果，模型可解释或采用其他允许的操作；同一执行内相同被拒绝的操作不再次请求审批。
进程关闭或取消结束 pending 审批，已经作出的决定保留；重启保留审计、标记中断，不自动执行已批准但结果未知的操作。

## 操作事实与上下文恢复

工具的 `Result` 分别报告有界 JSON `Output` 与 `Outcome`。事实由可信实现提供，与步骤成功、失败、取消及整个执行状态独立；关闭资源失败、超时或取消不得覆盖已生效操作的有效输出。缺失或无效事实按可信 `effects` 保守处理：纯读取为 `none`，其他为 `unknown`，并返回工具契约错误。

新工具步骤通过 `input.operation_version:1` 声明契约。`output.operation` 包含 `version:1`、`outcome` 和规范化固定操作的 `request_hash`；`output.result` 保留实际输出，`output.error` 保存独立的受控错误，`context_message` 使用同一事实和输出。无有效输出的失败保留 `result.error` 兼容结构。

| outcome | 含义 |
| --- | --- |
| `none` | 操作不产生修改、外发等副作用 |
| `not_applied` | 确认没有生效，包括未进入执行边界的调用 |
| `applied` | 请求的操作已生效，不代表后续模型回答成功 |
| `partially_applied` | 只有部分操作生效，需核实完成范围 |
| `unknown` | 无法确认是否或完全生效，不能当作未执行 |

工具执行前持久化保守边界，保存失败则不调用工具。执行终态保存和启动恢复在同一事务内结束遗留步骤、审批及运行状态，并为未完成的新工具步骤保存 `recovered:true`：有明确事实时保留原事实及输出；尚未启动则 `not_applied`；已启动但缺少结果的纯读取为 `none`，其余为 `unknown`。数据库及依赖不变，无需结构迁移；旧步骤未声明契约，不推断其副作用。

成功执行继续恢复完整轨迹；失败、取消或中断执行仅在存在 `applied`、`partially_applied` 或 `unknown` 事实时恢复调用轨迹。轨迹位于所属用户消息后，保留工具调用和结果完整配对，同轮尚未执行的调用补为 `not_applied`；失败回答正文不进入后续上下文。读取前检查消息数量和累计字节，恢复后仍遵守输入边界。编辑重发保留操作审计，但替换分支不再进入上下文。

同一 `request_hash` 留下未知、部分生效或已生效但报错的结果后，模型再次请求该固定操作必须重新获得用户确认；`never` 策略拒绝，判定原因 `previous_result_requires_confirmation`。这一约束沿当前分支上下文恢复，不自动重试操作；明确成功才解除待核实状态。审批不改变操作事实，也不豁免参数、目标及工作区的执行前校验。

已接入文件创建与修改；命令执行和操作系统沙箱尚未接入。macOS、Linux、Windows 的原生验证与交叉编译结果分别说明，不将编译通过视为原生运行通过。

模型与工具结果保留统一 context_message 以重建跨协议上下文，标题不参与工具轨迹；步骤仍保存原始结果和 usage。
工具调用轮中的暂存正文随 message.updated 清空，最终回答继续流式输出；完整中间文本可查询对应模型步骤。

## 文件补丁的执行边界

`filesystem.Tools` 装配三个读取工具及 `apply_patch`，无需修改 stdio 路由或数据库结构。补丁解析和文本变换只依赖统一工具契约；Unix、Windows 的句柄和私有权限处理放在平台文件中。`x/sys` 沿用既有版本，调整为直接依赖，不升级其他依赖。

`PlannedInvocation.Plan` 仅用于确有多目标或动态影响的调用，保存 `Targets` 及 `AdditionalEffects`。执行器复制并固定计划，额外类别只能增加影响；未分类工具不能靠额外读取声明降低风险。快照中的 `tool.effects` 是合并后的有效类别，执行前同时核对注册定义、原单目标及完整计划；任一变化即拒绝。旧工具和既有单目标快照继续兼容。

补丁使用创建、更新语法及精确上下文，原文件 SHA-256 是更新前提。读取工具返回完整内容的哈希，补丁参数为每个更新提供路径、哈希；哈希在批准后读取正文时验证，不把模型提供的值当作已核实事实。全部目标通过正文、版本、上下文检查后才创建目录或辅助文件。普通修改与保守的大范围覆盖分类、目录和路径上限详见 README。

审批快照包含完整补丁、有效影响、所有文件和缺失目录目标，以及固定辅助路径。参数规范化统一路径和补丁换行，哈希转小写、版本数组按路径排序。恢复及暂存名字根据规范化参数、工作区及实际目标计算；同一固定操作的请求指纹不因格式、版本数组顺序或随机辅助名字改变。副本已存在时拒绝覆盖，未知结果不能借新准备过程自动重试。

审批等待不持有写锁，执行时对重叠目标加可取消的运行时锁，结束后立即释放活动锁项。现存文件固定原句柄、最近父目录及工作区身份；新增目标固定最近现存祖先，父目录仅在批准后创建，不沿新出现的子项链接进入其他目录。写入前后检查真实路径、身份和内容；外部进程的并发修改仍存在乐观检查窗口，不宣称事务隔离。

写入不通过重命名新 inode 替换原文件，以保留原有所有者和 ACL；硬链接、特殊权限位及 Linux 文件 capabilities 目标拒绝修改。新建对象采用私有权限；macOS 通过固定句柄调用 fchmod_extended 删除继承 ACL 并设置模式，Windows 通过原句柄重新获得 WRITE_DAC 后设置受保护 DACL。权限收敛失败不继续写入内容。私有原文和预期内容副本先写入并同步，再按固定句柄进行有界写入、截断和同步。开始一次文件写入后完成该有界步骤再响应取消，避免在写入与截断之间主动放弃；系统错误仍需保留部分或未知事实。新文件采用排他创建，存在时不截断。

每个文件在 `changes` 中独立记录实际结果和内容哈希，目录创建及未清理副本也纳入报告。多文件不是文件系统事务，失败不回滚已生效文件；原文和预期副本在需要核实的错误中保留，成功清理也校验辅助目标身份。结果通过现有工具步骤、操作事实和上下文恢复链路保存；异常退出的未完成步骤继续保守记录为未知，不补造逐文件完成进度或自动恢复执行。
