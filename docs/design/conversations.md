# 对话、执行与上下文设计

本文记录已确认的目标设计。已完成普通对话、支持可选工作区的文件读取及补丁工具循环与审批；已接通手动及自动压缩、摘要版本与来源追溯；用户补充输入已通过模型提问工具、步骤持久化及 stdio 查询/提交接入。实际完成情况以 README 和实现为准。

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
- 普通对话按当前完整请求的输入 Token 预算规划压缩，并在消息数量和实际请求字节超限前尝试压缩，模型容量未知时要求显式预算；保留本地请求、响应及累计内容字节安全上限。估算不保证与远端 tokenizer 一致，远端仍校验最终容量。
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
settings_snapshot 保存受理事务读取的配置版本和实际审批设置及独立的 context_compaction 配置快照；新执行必须保存快照，旧执行缺失时保持空值，不补造历史配置。

### conversation_run_steps

id TEXT PRIMARY KEY；conversation_id TEXT；run_id TEXT；parent_step_id TEXT?；sequence INTEGER；type TEXT；name TEXT；definition TEXT JSON?；status TEXT DEFAULT 'pending'；input TEXT JSON DEFAULT '{}'；output TEXT JSON?；usage TEXT JSON?；model TEXT JSON?；provider_request_id TEXT?；tool_call_id TEXT?；error_kind TEXT?；error_message TEXT?；created_at INTEGER；started_at INTEGER?；updated_at INTEGER；finished_at INTEGER?。

回答模型使用的 context_version_id 保存在步骤 input JSON，摘要模型用 generated_context_version_id 标明产物；通过版本查询追溯，不增加循环外键。run_id、sequence 唯一；父步骤同执行，步骤和执行同会话。
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

已实现：id TEXT PRIMARY KEY；conversation_id TEXT；version INTEGER；base_version_id TEXT?；generated_by_step_id TEXT；through_sequence INTEGER；summary TEXT；source_manifest TEXT JSON；size_stats TEXT JSON?；status TEXT DEFAULT 'building'；created_at INTEGER；updated_at INTEGER；activated_at INTEGER?；invalidated_at INTEGER?。

状态为 building、active、superseded、invalid、failed。会话内版本号唯一且最多一个 active。
保留来源原文及工具调用、结果的配对关系；摘要验证保存成功后才事务切换。失败保留旧有效版本。
size_stats 保存实际 JSON 字节数、消息数及估算的前后 Token 数和方法，不是模型计费 usage。context_compaction 默认 enabled=true、token_budget=0、output_reserve=4096、safety_margin=1024、image_token_estimate=8192、retain_turns=4。已知窗口扣除输出预留及安全余量，正的 token_budget 只收紧预算；未知窗口要求显式正预算。自动压缩计入工具定义、操作事实和当前工具链，摘要批次也按预算选择完整轮次；压缩后重新计算，无法满足预算时明确失败，原文保留。数量和字节仍是独立资源边界，超限时先尝试安全压缩，再复查完整请求。unicode-v1 是启发式估算，实际 prompt_tokens 仅可参考同一执行的相同前缀及工具定义，不累加历次用量、不混入 usage。
长工具链支持执行内检查点：完整模型调用及其全部工具结果构成一个不可拆分批次，当前用户要求和正在执行的批次保持原文。较早已完成批次生成摘要，模型请求前保留最近完整批次；检查点以版本化 JSON 保存在 context_compression 步骤 output.checkpoint，记录来源执行、覆盖步骤范围、父检查点、批次来源指纹、摘要和大小统计。激活事务重查来源及父检查点，并原子完成压缩步骤；必须减少字节和估算 Token，失败不覆盖旧检查点。重启恢复使用检查点和未覆盖的完整后缀，独立未知操作事实继续限制重试。此机制复用现有步骤存储，无新增表或迁移；原始模型与工具结果和 usage 保留。
历史受理只读取有界完整轮次前缀，剩余页面在压缩成功后从新的有效摘要边界继续读取。轨迹过长时先按完整批次压缩，避免在到达自动压缩判断前因整段加载超限而失败。分页和摘要均不静默丢弃原文，不重放旧工具。
触发方式记录在压缩步骤 input.trigger.type（manual、automatic）及 reason。
context_compression 编排下建立 model_call 子步骤保存生成摘要的模型结果与 usage。生成时不持有事务和工具，复用模型调用槽位与超时；激活时原子核对来源、切换唯一有效版本并完成父步骤。失败与中断不激活半成品，已提交版本保留。不设固定批次上限，每批必须推进来源范围并减少摘要 Token 估算；准备窗口和单次请求仍有资源边界，循环检查取消，无进展明确失败；来源清单仅记录新增覆盖范围，旧摘要沿 base_version_id 追溯。

待核实工具操作独立从服务端审计读取，不依赖摘要措辞；摘要不能清除重新确认要求。摘要作为有来源的用户级历史数据，不提升为系统指令。摘要质量属于有损生成，结构与指纹校验不能证明语义完全保真，原文和图片引用保留。

## 编辑重发

仅编辑最后一条用户消息，要求原执行已结束。保留用户消息 ID、sequence，更新内容和 run_id，删除旧回答，创建新执行与回答。
清除旧模型结果副本和摘要中的被替换消息内容；保留实际工具操作、结果及审批事实。不删除源图片。
有关写入在事务内完成，网络调用在事务外。后续版本依赖被改动来源时一并失效。

## 通信

业务服务和事件契约不依赖通信协议。`app.New` 创建共享运行时，无需输入输出；业务调用方直接调用服务并订阅事件。
当前接入为 JSON-RPC 2.0，stdin/stdout 每行一个 JSON。请求仅返回一次受理响应，返回 conversation_id、run_id 等 ID 后，再推送无顶层 id 的服务端通知。
事件名位于 method，params 按事件携带 conversation_id、run_id、step_id、message_id、approval_id。
message.delta 使用 block_index 和带 type 的 delta，标题、工具、摘要输出不混入回答正文。

当前事件：conversation.created、conversation.updated、run.started、run.updated、run.completed、run.failed、run.cancelled、message.created、message.delta、message.updated、step.created、step.updated、tool.output、approval.requested、approval.updated、user_input.requested、user_input.updated。

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
建表按业务分组，`000001_01_create_catalog.go` 管理厂商和模型，`000001_02_create_conversations.go` 管理对话及执行表，`000001_03_create_approvals.go` 管理审批和目录授权，`000001_04_create_settings.go` 管理应用设置。文件名前缀对应结构版本，组内序号表示建表顺序。空库在一个事务内直接建成版本 `1`，默认执行和压缩设置组的 revision 均为 `1`。

打开已有库同时验证版本和实际结构。旧结构或不支持的版本保留原数据并返回错误。初始化失败整体回滚。开发联调可指定独立数据目录建库。
验证包括迁移、约束、事务回滚、多会话隔离、取消、重发、标题竞争、流式异常、资源关闭及 usage 保存，并运行项目规定的完整检查。


## 工具与审批实现约定

业务编排位于 internal/agent，filesystem 与 command 根包分别通过 Tools()、Tools(environment) 汇总工具；具体文件工具位于 filesystem 子包，命令实现位于 internal/tool/builtin/command/run。根包不承载具体参数校验、执行或资源生命周期；具体工具通过 New() 实现统一工具契约，数据库表名和通信 run 标识保持兼容。
会话更新的业务输入为 conversation.Update，只接受模型 ID；conversation/service 校验并解析快照后生成 conversation.Change 仓储参数。stdio 负责参数转换，现有 JSON-RPC 方法、字段和错误映射保持兼容。
工具审批通过 tool/policy.Operation 接收可信事实，由 policy 统一计算历史拒绝、结果待核实、沙箱边界和目录授权后的决定。Agent 负责快照校验、授权查询、审批等待和工具编排，授权撤销与执行认领仍在同一数据库事务边界协调。
文件和命令工具共用 internal/tool/builtin/internal/arguments 的 JSON 结构解析：单个 UTF-8 对象、16 KiB 上限、字段名精确匹配、拒绝各层重复字段和 null、未知字段、类型错误、过深嵌套及尾随内容。各工具保留实际参数范围、默认值和业务校验，Validate 与 Prepare 共用同一业务解码函数。读取工具重复字段从保留最后值改为拒绝，合法参数的规范化及审批快照/指纹保持原义。
主对话由 internal/agent/service/prompt.go 定义固定的过程沟通提示，loop.go 在每轮主调用的 fixed 上下文首位加入 system 消息，随后追加待核实操作事实及有效历史。提示参与预算估算和请求指纹，不落为用户消息，不参与历史摘要；自动压缩后继续注入，既有工具轨迹与来源不改写。标题、摘要与 model.invoke 保持原用途。提示引导操作前概括目的、阶段之间报告真实发现与下一步，不对每个文件重复说明；实际文本由模型输出，不补造内容，也不为补写说明增加模型请求。审批和副作用判断独立于这些文字。

普通对话及 Agent 默认向所有模型提供已注册工具，不依赖 model_type、capabilities.tools 或工作区绑定，模型自主选择是否调用。辅助标题与兼容 model.invoke 保持文本用途。每轮最多一个回答模型请求，同轮工具串行；
model_type 是 text、image、audio、video 内容类型元数据，可多选，不作为执行开关；图片输入由 capabilities.image_input 明确声明，推理等级只作元数据。
不限制模型调用轮次、累计工具次数及单次响应提出的工具调用数量。模型完成、取消、应用/连接退出或不可继续的错误结束执行；保留并发、单次请求超时、Token 预算及内容边界。工具定义种类数不设本地固定上限，工具参数各不超过 16 KiB，调用 ID/名称/参数累计字段字节不超过 8 MiB。执行前验证整批最小结果结构可容纳，真实结果逐次检查累计上下文，已发生操作及结果照常审计。当前执行工具链不被拆分压缩，达到资源边界时明确失败。文件工具为 list_directory、read_file、search_text、apply_patch，支持绝对路径；相对路径需要工作区基准，不接受 .. 越界。无工作区的相对路径返回 path_base_required。
工作区内绝对路径与相对路径遵守相同规则，符号链接按实际目标分类。单文件固定文件句柄，目录操作通过 os.Root 限定在单次目标目录内，目录搜索不跟随子项符号链接。
准备阶段不读取正文或目录条目；执行前重新检查请求路径、目标身份及工作区。目标或链接在等待审批期间被替换时拒绝执行。
模型调用与工具执行分别使用 call-timeout、tool-timeout，默认 5 分钟和 30 秒。审批没有过期时间或超时配置，等待用户决定或执行取消。
审批等待保留有界执行容量和会话互斥，但释放模型请求槽位，不持有数据库事务。

应用设置保存在 app_settings：id TEXT UUIDv7 PRIMARY KEY；key TEXT UNIQUE；value TEXT JSON 对象；revision INTEGER 默认 1 且大于零；created_at INTEGER；updated_at INTEGER。
执行配置组 key=execution，value 保存独立的 approval_policy、approvals_reviewer、sandbox_mode，不为每个设置项增加数据库字段。
settings.get 读取配置组，settings.update 携带 key、当前 revision 和完整 value 原子替换；冲突返回 1003，非法参数返回 -32602，失败不改变配置。
受理执行与读取设置在同一事务中完成，保存 settings_snapshot；更新成功后发起的新任务读取新值，已受理任务的审批策略保持不变，无需重启。
approval_policy 支持 on-request（默认）、always、never。approvals_reviewer 支持 user（默认）和 auto_review。auto_review 仅用于 on-request 或 always 的受限沙箱模式。完全访问显式配置为 sandbox_mode=danger-full-access，并与 never、user 配对。
启动时保留用户选择；读取异常阻止受理，不回退默认权限；旧执行缺失快照不伪造数据。
on-request 放行纯读取及工作区内普通修改（只读沙箱除外）。纯读取可访问当前系统用户可读的本地路径，无需绑定工作区或额外目录授权。无工作区或工作区外普通修改以及删除、大范围覆盖、外发或未知影响需要询问。always 逐次询问。受限沙箱下的 never 直接拒绝需要审批的操作。
工具 effects 由可信代码声明，可包含 read、write、destructive、external、unknown；缺少类别按未知操作处理，无效类别拒绝注册。
混合类别采用需要审批的结果，读取声明不能抵消破坏性或外发影响；工具负责将影响较大的覆盖声明为 destructive。
当前已提供三个读取工具、apply_patch 文件创建修改及 run_command 命令工具。独立删除和外发工具尚未接入；命令保持 unknown 分类，审批另结合服务端固定的执行边界。
参数、目标范围及工作区状态独立校验；默认批准只授权本次固定目标操作；会话目录授权由用户明确选择并单独保存。策略拒绝和参数校验失败不生成用户审批记录。
权限判定属于 Go，模型不能声明自己已获授权。旧 approval-mode、approval-policy、approvals-reviewer 启动参数移除，macOS 受限模式命令使用 Seatbelt。审批人可配置为用户或 AI 审核。完全访问由用户选择并写入执行快照，命令使用当前系统账户权限。
工具步骤 input 保存 settings_revision、approval_policy、approvals_reviewer、sandbox_mode 和 authorization（action、reason），供审计查询；审批表结构和通信接口保持不变。
request 保存服务端生成的工具完整定义、版本、可选工作区、实际目标及规范化参数，request_hash 为该快照的 SHA-256。无工作区时 workspace 为 null；target 保存 requested_path、path、kind 和 within_workspace，不能由模型参数覆盖。
approval.get/list 查询记录，approval.decide 接收 approval_id、decision、decision_note 和可选 scope。人工决定的身份由服务端填写为 local_user。
当审批人为 auto_review 时，原本需询问的项目内普通文件操作、无已知风险的只读沙箱命令及系统 `mkdir` 在项目内创建单层目录可提交独立审核。审核使用本轮模型和有限的最新用户文字、固定操作快照，占用模型请求槽位且最多等待 20 秒。其他可写沙箱命令、沙箱外命令及待核实操作继续交给用户。审核超时、失败或缺乏足够信息时交给用户。审核使用独立模型步骤记录模型、结论和实际用量，不进入主对话上下文。AI 决定绑定同一执行、工具步骤和请求哈希，按 auto_review 记录审批人，不创建目录授权。
同一人工决定与授权范围重复提交幂等，冲突决定或覆盖 AI 决定的请求会被拒绝。批准与取消竞争时，仍需在执行前检查上下文、工作区及操作指纹，数据库原子认领保证同一步骤不会启动两次。
拒绝转为结构化工具结果，模型可解释或采用其他允许的操作；同一执行内相同被拒绝的操作不再次请求审批。
进程关闭或取消结束 pending 审批，已经作出的决定保留；重启保留审计、标记中断，不自动执行已批准但结果未知的操作。

## 操作事实与上下文恢复

internal/agent/transcript 负责执行轨迹配对校验、失败回答过滤、未执行结果补齐及中断工具输出恢复；输入仅为业务类型，不依赖 GORM 或 SQLite 表映射。SQLite 仓储负责有界查询、当前分支筛选和恢复结果的原子保存，历史恢复与压缩检查点共用这些规则；conversation/history 负责会话消息与执行轨迹的整体装配。

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

已接入文件创建修改和非交互命令执行；macOS 命令使用 Seatbelt 系统沙箱，Linux/Windows 仍逐次审批。macOS、Linux、Windows 的原生验证与交叉编译结果分别说明，不将编译通过视为原生运行通过。

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


## 命令执行与输出

`run_command` 是模型工具，通过现有 conversation.send、工具步骤及审批接口执行；不增加直接命令 RPC、终端会话、stdin 写入或 PTY。参数为 command 字符串数组（1 至 256 项）及可选 cwd，严格拒绝重复键、NUL、null 和非字符串项。工作区提供省略/相对 cwd 的基准；未绑定时必须提供绝对 cwd。可执行文件名在装配时固定的绝对 PATH 项中查找，不使用进程隐含目录；参数不自动经过 Shell。Windows 直接支持 exe/com，脚本需显式调用解释器。

`SnapshotInvocation` 是可选的工具契约，详情保存到审批 request.details，最多 16 KiB 非空 JSON 对象。命令准备仅固定句柄和元数据，详情记录实际可执行文件及身份、大小和修改时间、cwd 及身份、环境配置名单与摘要，不保存环境值；固定参数及可选工作区仍使用原字段。执行器复制快照并纳入 request_hash，执行前比较；命令再次核对请求路径、目标与工作区身份。未实现此接口的工具不新增字段，旧文件审批指纹保持原样。元数据校验不是依赖图锁定或原子执行保证。

环境由 app 传入并按 base-v1 白名单筛选，只保留基础路径和语言变量，不允许模型覆盖，不继承模型 Key、令牌或任意代理配置。PermissionTool 接收服务端快照中的 SandboxMode，CommandInvocation 返回固定执行边界，执行器将其纳入快照并在执行前复核。命令 effects 固定为 `unknown`。macOS 使用 `/usr/bin/sandbox-exec` 启动 Seatbelt，默认只读，网络关闭，仅本次命令的私有临时目录可写。内置文件工具和 Seatbelt 命令禁止访问 Callum 自身数据目录，目录搜索跳过该目录。沙箱内项目写入也限制重命名该目录在项目内的祖先路径。文件工具拒绝以其他路径访问 SQLite 文件及附属文件。SQLite 文件存在硬链接时拒绝启动 Seatbelt 命令。项目中的 `.git`、`.codex`、`.agents`、`.aws` 仍受写入限制。Go/npm 临时缓存使用本次命令的私有临时目录。

`on-request` 自动执行普通只读沙箱命令，已识别的高风险命令逐次询问。`always` 对有效调用逐次询问，`never` 拒绝需要审批的调用。沙箱失败保留退出码及错误，任何更高权限都需要新的明确请求。

`sandbox_permissions` 省略或为 `use_default` 时，macOS 命令保持只读沙箱。需要项目写入时，提交 `workspace_write` 及非空 `justification`。服务端要求绑定工作区、受理快照允许项目写入和单次审批。批准后，命令可修改或删除真实工作区内的文件，网络仍关闭。请求 `require_escalated` 同样需要说明和单次审批，批准后使用当前系统用户的文件和网络权限。Linux/Windows 当前命令逐次审批，`workspace_write` 在没有系统沙箱时不可用。

Seatbelt 是共享文件系统上的路径访问边界，不是容器或独立文件系统：项目内既有文件的其他硬链接会共享修改，项目内已可写的别名也不构成独立副本；新建硬链接被默认拒绝。路径固定及执行前复核不保证抵抗同时发生的外部重命名，也不锁定程序的完整依赖图。应将项目根目录设为实际任务目录，而非整个用户目录。

`StreamingInvocation` 是可选的执行契约，普通工具保持最终 Result 接口。命令持续排空 stdout/stderr，业务服务发布 tool.output，stdio 转换为带 conversation_id、run_id、step_id 和 output 的通知；output 包含步骤内递增 sequence、stdout/stderr 的 stream、encoding=base64、data。片段最多 4 KiB，单次实时输出最多 64 KiB 与 128 片；不受模型 stream 开关控制，不混入模型正文，不逐片持久化。回调失败停止进程并等待读取任务退出；Hub 慢订阅仅断开对应消费者。两个流各自有序，不承诺跨流写入顺序。

最终每个流保留前 16 KiB，按 utf-8 或 base64 编码并收敛到各 24 KiB JSON，总结果不超过既有 64 KiB；超出后继续排空，并保存实际字节数和截断标志。工具步骤保存 started、pid、exit_code、signal、timed_out、cancelled、cleanup_complete 及两个输出对象，模型在结束后收到同一有界结果。客户端以步骤终态和查询结果为准，断线不能要求重放所有输出片段。

正常退出并完成收尾报告 applied，含义仅为命令调用已经执行至退出；exit_code 独立表达业务成功/失败，非零退出码不是启动失败。启动失败 not_applied；启动后的超时、取消、信号终止和结果不明报告 unknown，保留输出，沿用持久化边界、中断恢复和重新审批，不自动重试。

单次命令沿用 tool-timeout，默认 30 秒，不计算审批等待。Unix 独立进程组覆盖未主动脱离的后代，主进程退出、取消和超时均终止组内残留；不承诺收回主动脱离进程组或经外部服务启动的进程；Unix 上 Callum 异常退出也不保证回收子进程，恢复仅记录未知而不重试。Windows 10+/Server 2016+ 在创建时通过 JOB_LIST 关联 Job，启用 KILL_ON_JOB_CLOSE，限制普通 breakaway；不具备操作系统沙箱。主进程和管道各最多 2 秒收尾，Windows Job 成员另有最多 2 秒确认，失败如实记录而不伪造清理成功。Close 取消并等待本次执行，再释放准备句柄，可重复调用。

测试使用 testdata 中的受控进程及临时目录，验证审批前不启动、参数/工作目录/环境、双流实时输出、截断、退出码、取消/超时、子进程和应用关闭。平台原生 CI 与本地交叉编译分开记录，新增工具不增加数据库迁移或 Go 依赖。

### 会话目录授权

目录授权由服务端根据已固定的实际目标与影响类别推导（真实绝对目录 `root_path`、`access: read/read_write`），不改变原始操作快照与指纹，历史待核实操作继续匹配原身份；仅对 on-request 下因工作区外普通修改而询问的有效操作提供；审批对象顶层 `directory_scope` 存在时客户端才可展示选项。新普通修改授权提供包含读取的 read_write；历史 read 授权继续保留、可查询和撤销，但默认纯读取不再依赖它；不隐式扩大为删除、外发、未知操作或命令执行权限。

`approval.decide` 的可选 `scope` 默认 `once`；选择 `conversation_directory` 时，审批决定与 `conversation_directory_grants` 授权在同一事务保存。授权绑定当前 conversation_id，跨提问及重启保留，其他会话不继承。历史一次性审批不会自动升级。授权范围是实际目录路径，目录改名不会自动迁移授权；判断使用路径层级而非字符串前缀，并覆盖多文件计划及辅助路径，符号链接按实际目标校验。

`approval.directory.list` 按 conversation_id 分页查询（limit 默认 100、最大 200，offset 默认 0），返回包含撤销记录的数组；记录包含 id、conversation_id、approval_id、scope、created_at、revoked_at。`approval.directory.revoke` 接收 conversation_id 和 id，重复撤销幂等，不改变来源审批。创建及撤销发布 `directory_grant.updated`，通知包含 conversation_id 与 directory_grant。工具步骤 input.directory_grant_ids 和 authorization.reason=conversation_directory_grant 记录继承来源，不伪造新的用户决定。工具认领事务再次核验撤销状态及范围；撤销不撤回已开始或已单独批准的操作，只使依赖该目录授权的后续操作重新判断。被拒绝或结果待核实的操作、always/never 策略继续执行各自的限制。
