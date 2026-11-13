# Agent 工具与审批

本文件说明工具循环、路径规则、权限设置、审批、文件补丁与命令执行。所有命令均从仓库根目录执行；协议字段和请求示例见 [JSON-RPC 文档](jsonrpc.md)，压缩与输入预算见 [上下文统计与压缩](context-compaction.md)。

## 会话目录授权

目录授权由服务端根据已固定的实际目标与影响类别推导（真实绝对目录 `root_path`、`access: read/read_write`），不改变原始操作快照与指纹，历史待核实操作继续匹配原身份；仅对 on-request 下因工作区外普通修改而询问的有效操作提供；审批对象顶层 `directory_scope` 存在时客户端才可展示选项。新普通修改授权提供包含读取的 read_write；历史 read 授权继续保留、可查询和撤销，但默认纯读取不再依赖它；不隐式扩大为删除、外发、未知操作或命令执行权限。

`approval.decide` 的可选 `scope` 默认 `once`；选择 `conversation_directory` 时，审批决定与 `conversation_directory_grants` 授权在同一事务保存。授权绑定当前 conversation_id，跨提问及重启保留，其他会话不继承。历史一次性审批不会自动升级。授权范围是实际目录路径，目录改名不会自动迁移授权；判断使用路径层级而非字符串前缀，并覆盖多文件计划及辅助路径，符号链接按实际目标校验。

`approval.directory.list` 按 conversation_id 分页查询（limit 默认 100、最大 200，offset 默认 0），返回包含撤销记录的数组；记录包含 id、conversation_id、approval_id、scope、created_at、revoked_at。`approval.directory.revoke` 接收 conversation_id 和 id，重复撤销幂等，不改变来源审批。创建及撤销发布 `directory_grant.updated`，通知包含 conversation_id 与 directory_grant。工具步骤 input.directory_grant_ids 和 authorization.reason=conversation_directory_grant 记录继承来源，不伪造新的用户决定。工具认领事务再次核验撤销状态及范围；撤销不撤回已开始或已单独批准的操作，只使依赖该目录授权的后续操作重新判断。被拒绝或结果待核实的操作、always/never 策略继续执行各自的限制。


## 模型与工具循环

`internal/agent` 接替原 `internal/run` 的执行管理职责，并编排模型与工具循环；数据库表名、`run_id` 和现有事件名称保持兼容。
所有模型默认收到已注册工具定义并自主选择是否调用，不需要工具能力开关；流式参数全部组装并校验后，Go 才执行权限判断及操作。
所有内置文件和命令工具的参数必须为单个 UTF-8 JSON 对象，不超过 16 KiB；统一拒绝重复字段（含嵌套对象）、未知或大小写不匹配的字段、`null`、类型错误、过深嵌套及尾随内容。业务默认值、路径和数值范围由各工具校验，`Validate` 与 `Prepare` 采用同一解析与业务规则。三个读取工具此前接受的重复字段现在返回参数无效；合法请求的参数规则保持一致。三个读取工具的当前定义版本为 3，说明文字更新为只读策略；新准备操作使用新定义计算指纹，历史定义、操作指纹和授权记录不重写；目录选项仍从已固定的目标快照推导。
内置 `list_directory`、`read_file`、`search_text`、`apply_patch`，同轮工具按模型返回顺序串行执行，工具步骤以该轮 `model_call` 为父步骤。
模型步骤保存实际请求参数、可用工具定义和上下文指纹，不重复保存整套历史或凭据。流式工具调用的供应商未知扩展字段按属性保留。
工具定义种类数不设本地固定上限，由实际请求大小和远端协议校验。工具参数各不超过 16 KiB，响应中调用 ID、名称及参数的累计字段字节不超过 8 MiB；上游原始响应仍受 16 MiB 限制，流式与非流式采用相同调用字节边界。执行前检查整批调用及最小结果结构能否容纳，每次取得真实工具结果后再次检查累计上下文；超限时保留已发生操作的审计与结果，不自动重试。当前问题保留原文，正在执行的整批工具调用和结果不被拆开。较早已完成批次可生成步骤摘要；每轮模型请求前保留最近完整批次，容量不足时明确失败。
模型调用轮次、累计工具调用次数及单次响应提出的工具调用数量均不设固定上限。执行持续到模型完成回答、用户取消、连接或应用退出，或发生无法继续的错误；总模型正文仍受 1 MiB 上限约束，辅助标题独立记录。并发容量、单次模型及工具超时、审批和 Token/内容边界继续生效。
每轮模型请求单独取得并释放请求槽位。审批等待不设置过期时间，也不持有事务或模型请求槽位；由用户决定或执行取消结束等待，仍占用全局活动执行容量和会话互斥。

## 文件读取与路径

文件工具支持绝对路径；相对路径必须是工作区内的本地路径，不接受 `..` 越界，工作区内的绝对路径采用相同审批规则。
多目录工作区的主目录仍是相对路径基准。主对话模型请求会包含主目录和其他目录的路径，访问其他目录使用绝对路径，写入按实际路径走现有审批规则。
可通过 `workspace.update.root_path` 将已有源目录切换为主目录，原主目录保留为其他目录。切换与名称更新在同一事务保存，保留项目标识和已有会话，不移动磁盘文件；后续相对路径以新主目录为基准。有未结束执行时拒绝切换；目标必须仍是实际存在的目录，且不能占用其他项目（包括已软删除项目）的主目录。源目录可在编辑时增删：同时提交完整的 `root_path`、`other_paths` 和原目录快照 `expected_folders`（第一项为原主目录），在同一事务校验快照、活动执行和路径冲突后保存。原快照已变化返回冲突；移除仅解除目录关联，不删除磁盘文件。`root_path` 为空且 `other_paths` 为空数组可移除全部目录，项目和对话保留；无目录项目使用与未绑定工作区一致的工具路径规则，必须提供绝对路径。
无工作区时不会使用服务进程当前目录解析相对路径，而是返回 `path_base_required`，要求绝对路径或绑定工作区。
符号链接按解析后的实际目标分类。纯读取不受工作区写入边界限制，`on-request` 和 `never` 下无需逐次审批，可访问当前系统用户可读的本地路径；`always` 仍逐次询问。普通修改在工作区外或无工作区时需要审批，已有匹配的会话目录写授权可复用；`never` 拒绝仍需审批的修改。
准备调用时只固定句柄和元数据，不读取正文或目录条目；单文件固定文件句柄，目录通过 `os.Root` 限制在此次目标目录内。
执行前重新检查路径、目标身份及工作区状态；目标被替换或链接被重定向时拒绝执行，默认批准单个文件不授权其父目录及相邻文件；目录及子目录授权必须由用户另行明确选择。
单个文本文件最多读取 1 MiB；`read_file` 默认读取 200 行、单次最多 1000 行；工具结果最多 64 KiB，参数最多 16 KiB。
`list_directory` 默认返回 100 项、最多 200 项。`search_text` 执行字面匹配，默认最多 50 条、可设至 100 条；
最多检查 1000 个路径、32 层深度和累计 8 MiB 文件内容，跳过符号链接与 `.git`，通过 `truncated` 表示省略。
搜索空目录正常返回空匹配；遍历中的空子目录不会中断其他文件的搜索。
实际读取和搜索仍可能因文件被删除、权限变化或内容不再可读而失败，工具失败以结构化结果回传模型。

## 过程说明与轨迹

主对话的每轮模型请求加入固定系统提示，要求在一组工具操作前用 1～2 句话说明目的，在完成阶段、发现问题或改变方向时概括已确认的发现和下一步；连续检查相关文件合并说明，普通问答直接回答，用户可要求减少过程说明。
提示位于 `internal/agent/service/prompt.go`，通过 `loop.go` 的固定上下文每轮发送，参与输入 Token 估算及预算校验；历史压缩后仍重新加入，不保存为用户消息，也不作为摘要来源。辅助标题、历史摘要及兼容的 `model.invoke` 不加入该提示。较小上下文预算可能更早触发压缩或容量错误。
流式正文保存若被同一调用上下文的取消或超时打断，保留取消或超时终态；实际写入错误及终态保存失败继续报告存储错误。消息保存使用单条原子 UPDATE，避免额外隐式事务的取消回滚覆盖原错误；显式传入的调用方事务仍有效。
说明由所选模型生成，实际遵循程度取决于模型；服务端不拼接虚构的进展、不因缺少说明额外调用模型或重试工具。说明不是审批或成功凭据，工具执行和权限校验仍依据实际结果与服务端规则。

工具调用轮中的文本和原始模型结果保存在模型步骤。回答消息在工具调用轮结束后清空占位内容，下一轮继续流式输出，
客户端应以 `message.updated` 替换当前消息内容；成功结束后消息表保存最终回答。
模型与工具步骤额外保存统一的 `context_message`，后续对话恢复完整调用和结果配对；摘要标题不进入工具轨迹。
上下文轨迹读取前检查总大小和数量，不加载全部原始模型响应；编辑重发后旧工具审计保留，但旧分支不再进入上下文。
每次真实模型请求分别保存原始 usage，不累计估算值。

## 操作事实与恢复

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
## 执行设置与审批

`request` 是 Go 生成的工具定义、可选工作区、规范化参数与实际目标快照，SHA-256 绑定此次操作；客户端只能提交决定、说明以及服务端提供的授权范围选项，不能替换路径或影响类别。
无工作区时 `request.workspace` 为 `null`；文件工具的 `request.target` 包含 `requested_path`（解析基准后的请求路径）、`path`（解析链接后的实际绝对路径）、`kind`（`file` 或 `directory`）及 `within_workspace`。
`ApprovalPolicy` 与 `ApprovalsReviewer` 是独立设置，保存在 `app_settings` 的 `execution` 配置组中，客户端通过接口修改，无需重启 Go 服务。
`approval_policy` 默认 `on-request`，按需询问。`always` 对每次有效工具调用发起审批。受限沙箱下的 `never` 直接拒绝需要审批的操作。
`approvals_reviewer` 默认 `user`。选择 `auto_review` 后，服务可将原本需要审批的项目内普通文件操作、无已知风险的只读沙箱命令及项目内单层目录创建提交给独立审核调用。其他可写沙箱命令、沙箱外命令、危险操作、待核实操作和项目外文件操作交给用户。审核使用本次任务的模型、最新用户文字和固定操作快照，审核请求占用同一模型并发限额，最多等待 20 秒。批准或拒绝写入 `decided_by=auto_review`，仅绑定同一次固定操作。超时、格式错误和无法判断时走原有人工审批。模型、原始响应、供应商请求标识和实际 usage 保存于关联的审核 model_call 步骤；output 在保留原始响应字段的同时追加 `approval_review` 规范化结论。工具父步骤 input 仍只记录规范化结论，原始结果通过 step.data 分页读取；格式错误、拒绝和转人工的原始响应同样保留。取消随所属执行传播，重启恢复终态而不重放操作。
`sandbox_mode` 默认 `workspace-write`，也支持 `read-only` 和 `danger-full-access`。完全访问需显式选择，必须与 `never/user` 配对。命令以当前系统账户权限访问文件和网络，常规操作无需逐次审批。目标和资源校验继续生效，历史拒绝与待核实结果仍受限制。只读模式下普通补丁修改另需审批，目录写授权不绕过此限制。旧全局配置缺少该字段时，新任务按 `workspace-write` 受理；查询返回明确默认值而不改写存储或 revision。历史执行快照保持缺失，命令延续逐次审批；旧客户端更新原有两字段时保留当前沙箱模式。无需新增数据库列或改写历史记录。
`--approval-mode`、`--approval-policy`、`--approvals-reviewer` 已移除，启动参数不覆盖数据库中的用户选择。

应用设置表 `app_settings` 的字段为 `id`（UUIDv7）、唯一 `key`、JSON 对象 `value`、正整数 `revision`、`created_at`、`updated_at`。
当前注册 `execution`（`approval_policy`、`approvals_reviewer`、`sandbox_mode`）与 `context_compaction`（见 [上下文统计与压缩](context-compaction.md)）配置组；新增配置内容通过业务代码定义类型和校验，不为每个设置项增加数据库列。
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
文件目标由可信工具实现独立固定和校验，通用执行器不要求工具依赖工作区或文件系统；macOS 与 Windows 的 `workspace-write` 与 `read-only` 命令使用各自的系统沙箱；显式设置 `danger-full-access` 后，新任务命令使用当前系统账户权限。
模型请求及工具执行仍分别受 `--call-timeout`、`--tool-timeout` 限制；审批等待没有超时配置。
审批状态为 `pending`、`approved`、`rejected`、`cancelled`。批准后再次校验权限与取消信号，并在事务中认领尚未执行的工具步骤。
同一人工决定按原授权范围重复提交保持幂等，冲突决定返回错误。已完成的 AI 决定保留 `auto_review` 身份，后续人工提交返回冲突。本机用户决定记录为 `local_user`。
拒绝转为结构化结果交回模型，同一执行内相同规范化操作不会再次请求审批或自动执行。
取消或关闭会结束等待，保存待审批记录的取消状态；已批准的记录保留，批准不表示操作已完成。
重启恢复中断状态及新工具步骤的保守操作事实，不自动重试工具。

## 手工联调

手工验证时使用独立开发数据目录，已有厂商配置后创建模型用于测试工具。启用自动压缩时，需要为模型填写已核实的 `context_window`，或在 `context_compaction` 中设置显式 `token_budget`；下方模型示例只展示基础字段，预算配置见 [上下文统计与压缩](context-compaction.md#压缩设置与输入预算)。以下示例绑定工作区，相对路径与工作区内绝对路径均可使用：

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
不绑定工作区时，可省略 `workspace_id`，在问题中提供测试文件的绝对路径；模型调用文件工具后按上述审批流程处理。完整请求示例见 [JSON-RPC 测试文档](jsonrpc.md#11-工具调用及审批)。
工具协议已通过本机模拟回归；用户提供的 `newidea / deepseek-flash` 配置运行日志显示，流式工具调用、逐次审批、文件和目录读取、结果回传及继续回答均已完成。这只验证该次端点与模型组合，不代表其他配置均已验证。该次真实端点记录不包含新增补丁工具的验证；补丁和命令工具使用受控模型、真实临时文件、受控进程和 stdio 回归验证。上下文压缩通过受控模型及临时数据库回归验证，尚未验证真实厂商的摘要质量；用户补充输入已通过受控模型和独立临时数据库验证；MCP 与插件仍属于后续范围。

## 文件创建与修改

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


## 非交互命令执行

### 技能与附件调用

`DataDir/skills/.system/` 保存客户端自带技能，`DataDir/skills/<技能名>/` 保存用户技能。每轮扫描两处，有效同名用户技能优先；删除覆盖后恢复系统技能，备份隐藏目录不扫描。元数据计入预算，模型按需读取正文及脚本；技能不能改变权限。Go 不实现 Office 解析器。

运行时安装到数据目录下精确的 `runtimes/` 子树，该树只读。macOS 命令只额外允许查询数据根目录元数据，供原生程序解析资源路径；根目录枚举、其他私有路径的元数据与正文、数据库身份及所有数据目录写入仍受保护。

普通附件经 `open_attachment` 获取本轮只读输入副本，参数只有当前会话中的 `attachment_ref`。返回的 `path` 保留原文件名，`work_directory` 用于生成器、模块链接、缓存及渲染中间文件。本轮目录在完成、失败及取消后回收；后续追问重新调用，不使用过期路径。

已提供技能依赖时，`load_workspace_dependencies({})` 返回实际 Python、Node、`node_modules`、原生命令和本轮工作目录；命令子进程同时得到可信的 `RUNTIME_*` 路径。不要安装到共享资源目录或依赖其他应用的运行时路径。`details.run_work_directory` 单独记录本轮中间目录，不混入项目的 `writable_roots`。

`view_image({"path":"<absolute_page_png>"})` 通过固定句柄读图，把托管引用作为下一次模型请求的视觉输入；用户所选模型必须在 `model_type` 中启用 `image`。工具定义仍对所有模型提供，文字模型实际调用会明确失败；切换到文字模型时，历史工具图片以省略说明保留，不能声称已进行视觉检查。适配器在整批工具结果之后放入图片，不打断调用配对。

`publish_artifact({"path":"<absolute_output_file>"})` 保留独立会话交付副本，并在结束前追加到回答；不会把未发布的临时文件当作可长期交付的文件。源文件保持原样，单文件不超过 16 MiB。按固定消息索引读取和另存见 [附件协议](jsonrpc.md#12-附件输入与文件交付)。以上工具复用读取审批，不开放应用数据目录；命令自身的写入、外部访问与沙箱外执行仍遵守原审批规则。

`run_command` 由模型在 `conversation.send` 中调用，不是独立的 JSON-RPC 方法。参数示例：

```json
{"command":["go","test","./..."],"cwd":"."}
```

`command` 是可执行文件及参数数组，最多 256 项，整个参数 JSON 继续限 16 KiB；空参数合法，空可执行文件、NUL、重复键、未知字段及非字符串项被拒绝。
`cwd` 省略时使用绑定工作区；相对路径以工作区为基准且不能越界，未绑定工作区时必须明确提供绝对 `cwd`，不使用 Callum 进程目录。
可执行文件支持绝对路径、相对于实际工作目录的路径，或在服务端固定 PATH 中查找名称；忽略空和相对 PATH 项。Windows 直接启动 `.exe` / `.com`，无扩展名依次查找这两种格式，不隐式执行 `.bat` / `.cmd`。
不自动套 Shell。需要 Shell 语法时，在数组中明确指定相应解释器及其参数；解释器自身的转义规则仍由调用方遵守。stdin 连接空设备，无 PTY、后续输入或持久后台会话。

`run_command` v9 的命令选择指引适用于是否配置内置运行时的两种情况：单个命令直接传入可执行程序和参数数组；多个独立操作分别调用工具；目录查看优先用 `list_directory` 的 `path`、`limit`。不要仅为打印 echo/printf 分隔标题、串联独立命令或列出多个目录而构造 Shell 脚本。例如查看 Git 状态并列出目录时，分别调用 `run_command` 的 `["git","status","--short"]` 与 `list_directory`，无需拼接 `git status; echo ...; ls`。

只有确需管道、重定向、条件控制等 Shell 语法时才显式使用非登录 Shell；Windows 使用 PowerShell 的 `-NoProfile -NonInteractive` 或 cmd 的 `/d` 模式。服务端同时执行 v9 静态校验：对 `sh/bash -c`，若能确认脚本只含字面量程序调用、分号或换行串联及 echo/printf 分隔输出，且含普通外部程序，则在准备操作前返回 `invalid_arguments`，不启动任何进程。使用 `mvdan.cc/sh/v3/syntax` v3.14.1 解析语法树，不解释或执行脚本。管道、重定向、条件控制、变量展开、通配符、环境赋值及状态性 Shell 内建操作保留原有执行能力；语法无法可靠识别时不猜测用途。脚本文件和其他 Shell（包括 PowerShell、cmd、zsh）不套用这套 POSIX/Bash 规则。此检查是调用形式约束，权限和副作用仍由既有沙箱与审批判定。界面按真实步骤状态展示，执行前拒绝不会标成“已运行”。

配置 `--runtime-dir` 后，`python`、`python3`、`node`、`git` 裸命令在服务端固定到清单中的绝对程序入口，工具定义版本为 `9`。解释器目录同时前置到命令子进程 PATH，供脚本及 Shell 启动子进程使用；Windows 资源包含 `python3.exe` 别名。当前资源版本固定为 Python 3.12.14、Node 26.13.0 和 Git 2.56.1（Windows 为 2.56.1.windows.1），来源与准备方法见 [运行与开发](development.md#内置运行时资源)。缺失或损坏明确失败；不会通过 PATH 静默选择其他同名解释器。显式路径继续采用既有路径和审批规则；未配置运行时的独立 Go 服务保持原有命令查找行为。

工具说明在每轮模型请求中明确给出默认调用规则和当前资源的实际绝对入口。运行 Python/JavaScript/Git 默认直接使用 `python3`/`node`/`git`，无需用户重复指定“用内置解释器”，也无需探测或安装系统版本。以下示例的 `cwd` 需替换为实际平台的绝对目录。直接调用示例：

```json
{"command":["python3","-c","print(1)"],"cwd":"/absolute/project"}
{"command":["node","-e","console.log(1)"],"cwd":"/absolute/project"}
{"command":["git","status","--short"],"cwd":"/absolute/project"}
```

文件脚本分别使用 `["python3","script.py"]`、`["node","script.js"]`。不要仅为运行这些脚本或单个 Git 命令再套 `bash`、`sh`、`zsh`、PowerShell 或 cmd，尤其避免 `bash -lc`、`sh -lc`、`zsh -lc`。登录 Shell 可能重设 PATH，Shell 字符串里的程序不会经过裸命令入口映射。确需管道或重定向时，使用非登录 Shell，并按对应 Shell 语法引用工具说明提供的 Python、Node、Git 内置绝对路径；子进程 PATH 前置不替代显式入口。Windows 执行 PowerShell/批处理脚本或需要对应 Shell 语法时显式使用该 Shell，Python/JavaScript 直接指定解释器。用户明确指定项目虚拟环境或其他解释器时，使用其显式路径并遵守相同的校验、审批和沙箱。

Git 默认直接使用 `command=["git","status","--short"]` 等数组，服务固定入口，并在工具说明中给出实际绝对路径。Go 为所有命令子进程生成 `GIT_EXEC_PATH`、`GIT_TEMPLATE_DIR`、`GIT_CONFIG_SYSTEM`，指向同套 Git 辅助程序、模板及发行包配置；Windows 同时将本包 DLL、HTTPS/GCM 与 MSYS shell/ssh 目录前置到 PATH。显式选择其他 Git 路径的命令仍继承这套子进程环境，不提供独立 Git 环境切换。

`GIT_OPTIONAL_LOCKS=0` 禁止 status 的可选索引刷新；`GIT_TERMINAL_PROMPT=0`、`GCM_INTERACTIVE=0` 与空的 askpass 环境禁止内置 Git/GCM 交互认证，缺少可用凭据时失败。用户的仓库及全局配置仍按 Git 原有规则读取，Callum 不修改全局作者身份或配置，也不提供登录界面。Git 非零退出码需按子命令解释，例如 `diff --exit-code` 的 `1` 表示差异。

Git 复用现有沙箱及审批：只读查询不获得写权限；`.git` 写保护保持不变，init/add/commit 等写入需请求沙箱外权限；clone/fetch/push 的网络访问也需审批。内置 Git 不自动批准提交或推送。临时仓库的 init/add/commit/status/diff/log/本地 clone 已纳入真实资源回归，测试不访问用户仓库或远端。

这些规则用于引导模型的默认工具选择；服务端不改写 Shell 字符串或全局禁止 Shell，也不修改用户的登录配置和系统 PATH。未接入内置运行时的独立服务不注入上述内置入口说明。必要管道与临时目录重定向已通过真实 Python/Node/Git 的显式绝对入口回归；客户端受控模型回归验证每轮收到选用规则，并将命令与目录查看作为独立工具调用。这不等于真实模型在每次请求中都遵循指引。设计参考 [Codex 的登录 Shell 执行控制](https://learn.chatgpt.com/docs/enterprise/agent-security#execution-controls-executor)，通过直接参数数组和内置绝对入口明确解释器选择，避免依赖登录 Shell 的 PATH。

参数类型错误提供由服务端生成的字段说明，例如 `command: 必须是字符串数组`，不回显模型传入的值；模型可据此重新调用，纠正后的操作重新经过固定目标、审批与执行前校验。不会把字符串自动猜测或转换成命令数组。

当 Chat Completions 完整响应的工具身份有效、结束原因是 `tool_calls`，但某项 `function.arguments` 不是合法 JSON 对象时，整批工具都不执行。原模型步骤记为失败并保留组装响应、原始 usage；Agent 在下一次请求的固定上下文中添加有界的服务端纠正说明，重新生成本轮调用。纠正来源写入后续模型步骤的 `input.tool_arguments_correction`（`source_step_id`、从 0 开始的 `call_index`），不写成用户消息，也不把坏批次加入工具历史或摘要。反馈计入当前请求的内容及已启用的 Token 预算，实际 usage 逐次保留；取消、超时、保存失败、响应关闭失败及其他协议损坏沿原失败路径结束。部分生效或结果未知的工具不会进入参数纠正流程。

连续返回相同坏 JSON，或连续整轮都因相同工具参数被执行前拒绝且没有执行进展时，以明确的 `invalid_response` 结束。这是无进展检查，不是固定模型轮次或工具次数上限。已经完成的操作不重放，改变参数后的调用重新走原审批规则。

资源目录与私有数据目录分开。补丁工具拒绝创建或修改运行时文件，普通读取保持可用。macOS 受限命令禁止改写运行时及重命名项目内的资源祖先；Windows 为整个可信运行时树增加读取和执行范围，并拒绝写入现有资源对象及删除其祖先。`details.read_only_roots` 记录固定目录，读取根目录也进入 Windows 审批详情。显式完全访问或经批准的沙箱外进程仍使用当前账户权限；这些目录规则不构成独立文件系统，也不能阻止同账户外部程序修改完整依赖图。

Go 额外生成 `PYTHONDONTWRITEBYTECODE=1`、`PYTHONNOUSERSITE=1`、`PYTHONUTF8=1`，禁用默认字节码和用户 site-packages；模型显式使用 Python 的隔离参数时，解释器自身规则仍适用。受限命令的 `PIP_CACHE_DIR` 与 npm/Go 缓存一起定向到本次私有临时目录。第三方包应安装到获准的项目虚拟环境或项目依赖目录；共享运行时保持只读。Python 包管理可直接调用 `["python3","-m","pip",...]`，Windows 不自动执行 `npm.cmd`，需要时显式使用内置 Node 启动 npm 的 JS 入口。

环境由 app 显式传入并在工具装配时筛选，只保留 PATH、HOME、USERPROFILE、TMPDIR、TEMP、TMP、SYSTEMROOT、WINDIR、COMSPEC、PATHEXT、LANG、LC_ALL、LC_CTYPE，另按实际目录生成 PWD；Windows 名称不区分大小写。模型不能设置环境变量。API Key、令牌、代理凭据等任意环境项不会继承。
审批 `request.details` 保存解析后的可执行文件、身份及大小/修改时间、实际 cwd 及身份、环境名单与配置摘要，不保存环境值；规范化参数保留在 `request.arguments`。详情不超过 16 KiB，参与 `request_hash`，执行前重新比较并检查原路径、句柄、工作区和可执行文件元数据。未实现补充快照的文件工具省略 details，原审批格式与指纹保持兼容。
此检查不锁定整个软件依赖图，也不承诺执行前检查与路径启动之间的原子隔离。

命令 effects 固定为 `unknown`。macOS 使用 `/usr/bin/sandbox-exec` 启动 Seatbelt，默认只读，网络关闭，仅本次命令的私有临时目录可写。内置文件工具和 Seatbelt 命令禁止访问 Callum 自身数据目录，目录搜索跳过该目录。沙箱内项目写入也限制重命名该目录在项目内的祖先路径。文件工具拒绝以其他路径访问 SQLite 文件及附属文件。SQLite 文件存在硬链接时拒绝启动 Seatbelt 命令。项目中的 `.git`、`.codex`、`.agents`、`.aws` 仍受写入限制。Go/npm 临时缓存使用本次命令的私有临时目录。

Windows 10/11 的平台实现使用每次执行独立的 AppContainer、无网络 capabilities、`WRITE_RESTRICTED` 令牌和创建时关联的 Job Object。默认只读，可写范围为本次私有临时目录和 AppContainer 的隔离配置存储；项目写入仍需要单次审批。程序先以挂起状态创建并复核 AppContainer SID 与限制 SID，再恢复运行；建立失败直接返回错误，不降级为普通进程。当前仅完成交叉构建和原生测试用例编译，尚无 Windows 原生运行结果，不能据此确认隔离有效或 PowerShell、cmd、Node、Git、Go 的实际兼容性。

Windows 读取范围为固定工作目录、可选项目、可执行文件所在目录，以及 Windows 已授予 AppContainer 访问的系统资源。审批详情中的 `sandbox_backend=windows-appcontainer` 和 `readable_roots` 展示固定范围；其他本地读取可用文件工具。受限命令只接受本地盘符路径，UNC 不可用。目录或依赖无法授权时明确失败。现有 `.git`、`.codex`、`.agents`、`.aws` 对象拒绝写入，应用数据禁止访问，其祖先目录不授予删除和重命名权限，SQLite 及附属文件有硬链接时拒绝启动。ACL 保护跟随现有对象，不等同于 Seatbelt 对将来新建同名目录的路径规则；并发外部重命名及项目文件硬链接也不构成独立文件副本。

Windows 在执行前临时为独立 SID 合并 ACL，结束时只移除该 SID 的条目；保留其他 SID 的权限。准备与审批阶段不创建配置或改写 ACL。恢复日志先同步写入应用数据目录的 `command-sandboxes/`，启动取得实例锁后、打开数据库前恢复，确认 Job 已结束后回收配置和权限；失败保留日志并阻止继续初始化。单次计划最多 65,536 个对象、日志最多 16 MiB；回收扫描同样有界，失败不伪报完成。用户或外部程序将对象移出所有固定目录时，无法通过原路径发现移出对象上的遗留 SID；该 SID 不复用，旧令牌与进程结束后不再获得运行授权。并发外部 ACL 编辑不受 Callum 的跨进程互斥保护。Windows 原生验收仍需覆盖这些边界，详见[设计与验收说明](design/conversations.md#windows-原生命令沙箱实现与待验收范围)。

`on-request` 自动执行普通只读沙箱命令，已识别的高风险命令逐次询问。`always` 对有效调用逐次询问，`never` 拒绝需要审批的调用。沙箱失败保留退出码及错误，任何更高权限都需要新的明确请求。

`sandbox_permissions` 省略或为 `use_default` 时，macOS 与 Windows 命令保持只读沙箱。需要项目写入时，提交 `workspace_write` 及非空 `justification`。服务端要求绑定工作区、受理快照允许项目写入和单次审批。批准后，命令可修改或删除真实工作区内的文件，网络仍关闭。请求 `require_escalated` 同样需要说明和单次审批，批准后使用当前系统用户的文件和网络权限。Linux 当前命令逐次审批，`workspace_write` 在没有系统沙箱时不可用。Windows 的 AppContainer 实现与原生验收状态见前述说明。

Seatbelt 是共享文件系统上的路径访问边界，不是容器或独立文件系统：项目内既有文件的其他硬链接会共享修改，项目内已可写的别名也不构成独立副本；新建硬链接被默认拒绝。路径固定及执行前复核不保证抵抗同时发生的外部重命名，也不锁定程序的完整依赖图。应将项目根目录设为实际任务目录，而非整个用户目录。

进程受现有 `--tool-timeout` 限制（默认 30 秒），审批等待不计入。macOS、Linux 创建独立进程组并在正常主进程退出、取消、超时时终止组内残留成员；主动 setsid/setpgid 脱离的进程、通过其他服务启动的工作不在回收保证内；Unix 上 Callum 被强制终止或崩溃也不保证回收仍运行的子进程，重启后需按未知结果核实。Windows 10+ / Server 2016+ 创建进程时加入 Job Object，禁用普通 breakaway 并启用最后句柄关闭终止成员；Job 只负责进程生命周期，不构成文件或网络沙箱。终止后主进程与管道各有最多 2 秒收尾等待，Windows 另有最多 2 秒 Job 成员确认；清理失败明确返回错误及不完整清理状态，不宣称强制终止一定成功。

工具输出无论模型请求的 stream 为 true 或 false 都可实时推送；模型在命令结束后收到最终结果。通知示例：

```json
{"jsonrpc":"2.0","method":"tool.output","params":{"conversation_id":"<conversation_id>","run_id":"<run_id>","step_id":"<step_id>","output":{"sequence":1,"stream":"stdout","encoding":"base64","data":"aGVsbG8K"}}}
```

sequence 在单个步骤内从 1 递增；两个流各自保持读取顺序，不承诺还原跨 stdout/stderr 的原始写入顺序。客户端应持续读取 JSONL、按 step_id 分流并解码字节，不把片段当作完整 UTF-8 字符或最终成功信号。
每片最多 4 KiB 原始字节，每次调用实时推送最多 64 KiB、128 片。保存 stdout/stderr 各自前 16 KiB 原始字节，编码后各不超过 24 KiB JSON，最终结果仍不超过 64 KiB。达到上限继续排空管道，分别设置 truncated 和 stream_truncated，并保留实际读取字节数；输出片段只即时发布，不逐片落库或承诺重放。慢订阅者沿用有界队列规则，断开对应消费者，不取消共享执行；最终结果通过 step.data 查询。

结果包含 execution_boundary（sandboxed、sandbox_mode、escalated 及可选 risk）、started、pid（未启动时为 null，仅供核实，不作为重启后的进程控制凭据）、exit_code（未获得时为 null）、可选 signal、timed_out、cancelled、cleanup_complete、stdout、stderr、stream_truncated。每个输出对象为 encoding（utf-8 或 base64）、data、bytes、truncated；二进制或不能构成完整 UTF-8 的保留前缀使用 base64；UTF-8 文本按实际 JSON 转义长度和字符边界截断。
启动失败为 not_applied；若权限建立后的回收失败则保守记录 unknown 并保留恢复日志。正常退出且生命周期收尾成功为 applied，仅证明命令调用已经执行完毕，非零 exit_code 仍原样交给模型判断业务失败。启动后取消、超时、信号终止或结果不明为 unknown，并保留已经收到的输出。未记录结果的中断命令沿用持久化保守未知及重新审批规则，不自动重试。

`.github/workflows/verify.yml` 配置 macOS、Linux、Windows 的原生竞态测试、静态检查和无 CGO 构建；Go 版本读取 go.mod。工作流配置存在不等于运行通过，平台原生结果以实际 CI 记录为准；本地交叉编译只证明对应目标可构建。
