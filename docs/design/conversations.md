# 对话、执行与上下文设计

本文记录已确认的目标设计。实际完成情况以 README 和实现为准。

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

id TEXT PRIMARY KEY；conversation_id TEXT；replaces_run_id TEXT?；model TEXT JSON；stream INTEGER；status TEXT DEFAULT 'running'；error_kind TEXT?；error_message TEXT?；created_at INTEGER；updated_at INTEGER；finished_at INTEGER?。

状态为 running、awaiting_approval、awaiting_user_input、completed、failed、cancelled、interrupted。
同一会话最多一个未结束执行；不同会话受全局并发限制。手动压缩也可以创建执行，不要求有用户消息。
replaces_run_id 仅关联被替代执行，不保留旧消息分支。所有终态填写 finished_at。

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

随工具交互引入：id TEXT PRIMARY KEY；run_id TEXT；step_id TEXT UNIQUE；request TEXT JSON；request_hash TEXT；status TEXT DEFAULT 'pending'；decided_by TEXT?；decision_note TEXT?；created_at INTEGER；updated_at INTEGER；decided_at INTEGER?；expires_at INTEGER?。

状态为 pending、approved、rejected、cancelled、expired。审批绑定确定的操作，执行前核对操作摘要及当前权限；参数变化创建新步骤及审批。批准不表示执行成功。普通补充输入使用 user_input，批准、拒绝说明保存到 decision_note。

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

JSON-RPC 2.0，stdin/stdout 每行一个 JSON。请求仅返回一次受理响应，返回 conversation_id、run_id 等 ID 后，再推送无顶层 id 的服务端通知。
事件名位于 method，params 按事件携带 conversation_id、run_id、step_id、message_id、approval_id。
message.delta 使用 block_index 和带 type 的 delta，标题、工具、摘要输出不混入回答正文。

拟定事件：conversation.created、conversation.updated、run.started、run.updated、run.completed、run.failed、run.cancelled、message.created、message.delta、message.updated、step.created、step.updated、step.output_delta、approval.requested、approval.updated、user_input.requested、user_input.updated。

压缩使用 type=context_compression 的步骤事件。步骤父子关系支持折叠、展开，折叠偏好由客户端保存。
状态落库后通知；内容增量有界传输，终态前保存最终或部分结果。重新打开查询数据库状态，不承诺逐事件重放。
旧 model.invoke 保留 run.text_delta、run.completed、run.failed、run.cancelled 的兼容格式。

## 资源、迁移与验证

创建、编辑和状态变更按业务动作定义事务，不在等待模型、工具、用户输入时占用事务。
关闭时停止接收、取消并等待任务，再关闭通信、HTTP 连接池和数据库。恢复中断执行前确认数据目录实例所有权。
保留 000001_create_model_config.go，新增 000002_create_conversation_core.go；后续审批、压缩各有具体迁移。
版本判断同时验证实际结构，兼容当前版本 1 的 UUIDv7 配置库；迁移失败回滚，不重建用户数据库。
验证包括迁移、约束、事务回滚、多会话隔离、取消、重发、标题竞争、流式异常、资源关闭及 usage 保存，并运行项目规定的完整检查。
