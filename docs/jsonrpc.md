# JSON-RPC 通信与手工测试

本文按当前代码整理已实现的 stdio JSON-RPC 接口，共 39 个方法。协议版本为 `1`，数据库结构版本为 `4`，两者含义不同。

## 1. 启动及输入约定

在项目根目录执行。以下命令创建独立临时数据目录和测试工作区，避免使用日常应用数据：

```sh
cd /Users/xiaoyu/www/go/works/callum
CALLUM_RPC_TEST_ROOT="$(mktemp -d /private/tmp/callum-jsonrpc.XXXXXX)"
mkdir "$CALLUM_RPC_TEST_ROOT/workspace"
printf '%s\n' 'Callum JSON-RPC test file' > "$CALLUM_RPC_TEST_ROOT/workspace/note.txt"
printf '测试工作区路径：%s\n' "$CALLUM_RPC_TEST_ROOT/workspace"
go run ./cmd/callum --data-dir "$CALLUM_RPC_TEST_ROOT/data"
```

保存终端显示的工作区路径，作为后文的 `<workspace_path>`。上面的准备命令由 Shell 输出提示，Go 服务本身的 stdout 只输出协议消息。

- stdin 每行输入一个完整 JSON 对象，按回车发送；下文请求示例均可逐行复制。
- stdout 每行输出一个 JSON 对象；诊断日志写入 stderr。
- 不支持批量请求数组，也不支持 `params` 数组；未知参数字段会被拒绝。
- 测试时建议每个请求携带不同的字符串 `id`，用于匹配响应。
- `<provider_id>` 等字符串是占位值，必须替换为实际响应返回的 ID；它们不会自动插值。
- 业务记录 ID 为服务端生成的 UUIDv7；请求的 `id` 可以自行填写，不是业务 ID。
- 保持 stdin 打开直到任务终态。输入结束、关闭终端或执行 `system.shutdown`，会取消该连接发起且尚未结束的执行。
- 不要直接把所有示例一次性输入：部分请求需要前一步返回的 ID、当前版本或运行状态。

厂商地址、远端模型标识和密钥需要使用你自己的有效值；密钥只在本机输入，不要把真实值保存到此文档。本文中的所有响应均为结构示例，ID、时间、模型内容和用量以实际返回为准。

### 1.1 请求、响应和事件

请求：

```json
{"jsonrpc":"2.0","id":"info-1","method":"system.info","params":{}}
```

成功响应保留相同的请求 `id`：

```json
{"jsonrpc":"2.0","id":"info-1","result":{"service":"callum","protocol_version":1,"max_concurrent_runs":4,"call_timeout_ms":300000,"max_request_bytes":1048576,"max_response_bytes":8388608,"max_text_bytes":1048576}}
```

失败响应：

```json
{"jsonrpc":"2.0","id":"bad-1","error":{"code":-32602,"message":"参数无效，请检查字段、类型和必填值"}}
```

服务端事件没有顶层 `id`，事件名位于 `method`：

```json
{"jsonrpc":"2.0","method":"system.ready","params":{"service":"callum","protocol_version":1,"max_concurrent_runs":4,"call_timeout_ms":300000,"max_request_bytes":1048576,"max_response_bytes":8388608,"max_text_bytes":1048576}}
```

所有输入输出都是逐行 JSON，不带 SSE 的 `data:` 前缀。单条请求最多 1 MiB，单条响应最多 8 MiB；文本累计限制为 1 MiB，与 Token 容量无关。
普通对话的文本限制累计覆盖同一执行的所有回答轮次。流式超限后以 `run.failed` 结束，消息及 `message.updated` 只保留通过检查的部分正文；步骤仍保存原始响应和实际返回的 usage。

## 2. 接口总览

| 模块 | 已实现方法 |
| --- | --- |
| 服务 | `system.info`、`system.shutdown` |
| 应用设置 | `settings.get`、`settings.update` |
| 厂商 | `provider.create`、`provider.update`、`provider.get`、`provider.list`、`provider.delete` |
| 模型配置 | `model.create`、`model.update`、`model.get`、`model.list`、`model.set_enabled`、`model.delete` |
| 无历史调用 | `model.invoke` |
| 工作区 | `workspace.create`、`workspace.update`、`workspace.get`、`workspace.list`、`workspace.delete`、`workspace.restore` |
| 会话及消息 | `conversation.send`、`conversation.resend`、`conversation.get`、`conversation.list`、`conversation.update`、`conversation.messages`、`conversation.delete`、`conversation.restore` |
| 执行 | `run.get`、`run.list`、`run.cancel` |
| 步骤 | `step.get`、`step.list`、`step.data` |
| 审批 | `approval.get`、`approval.list`、`approval.decide` |

不存在 `conversation.create` 或客户端直接执行工具的 JSON-RPC 方法。首次提问创建会话，工具由模型自主选择。

## 3. 推荐测试顺序

| 顺序 | 操作 | 需要保存的数据／预期结果 |
| --- | --- | --- |
| 1 | 启动并发送 `system.info` | 收到 `system.ready` 和查询响应 |
| 2 | `settings.get` | 保存当前 `revision` |
| 3 | `provider.create` | 保存 `result.id` 为 `<provider_id>` |
| 4 | `model.create` | 保存 `result.id` 为 `<model_id>`，初始不选择扩展功能 |
| 5 | `conversation.send`，`stream:false` | 保存会话、执行、用户消息及回答消息 ID；等待终态 |
| 6 | 同会话 `conversation.send`，`stream:true` | 验证历史上下文和文本增量 |
| 7 | 查询消息、执行、步骤和 usage | 记录用户消息 ID、步骤 ID；验证持久化 |
| 8 | `conversation.resend` | 编辑最后一条用户消息；旧回答从消息列表移除 |
| 9 | 可选测试 `model.invoke` | 使用尚未选择扩展功能的模型，观察兼容事件 |
| 10 | 可选的 `workspace.create` | 工具默认提供；相对路径需要工作区，绝对路径不要求绑定工作区 |
| 11 | 读取最新设置，保存 `always` | 新执行逐次审批；运行中旧执行仍用原快照 |
| 12 | 在绑定工作区的新会话中提问 | 模型实际调用工具后收到审批；批准、拒绝或取消 |
| 13 | 可选测试图片、多会话、非法参数 | 按后文说明逐项执行 |
| 14 | 停止任务后测试归档、删除、恢复 | 最后执行 `system.shutdown` |

同一会话执行未结束时不能再次发送或重发；不同会话可以同时执行，默认全局活动执行上限为 4。

## 4. 服务信息

### system.info

```json
{"jsonrpc":"2.0","id":"system-info","method":"system.info","params":{}}
```

返回服务名称、协议版本、并发上限、模型调用超时和通信字节限制。结构见 1.1 节；这些数值会受相应启动参数影响。

### system.shutdown

放在测试最后执行：

```json
{"jsonrpc":"2.0","id":"system-shutdown","method":"system.shutdown","params":{}}
```

返回与 `system.info` 相同的信息，然后结束当前连接。独立 stdio 进程会继续完成资源关闭并退出；共享 Runtime 中其他调用方的执行不因该连接退出而取消。

## 5. 应用设置

当前仅注册 `execution` 配置组，其中两个字段分别控制审批策略和审批人。

| 字段 | 支持值 | 默认值 |
| --- | --- | --- |
| `approval_policy` | `on-request`、`always`、`never` | `on-request` |
| `approvals_reviewer` | 当前仅 `user` | `user` |

`on-request` 按操作类别判断；`always` 逐次询问；`never` 不询问，并拒绝需要审批的操作。`never` 不扩大工作区或工具权限，不是完全访问模式。

### settings.get

```json
{"jsonrpc":"2.0","id":"settings-get","method":"settings.get","params":{"key":"execution"}}
```

响应示例：

```json
{"jsonrpc":"2.0","id":"settings-get","result":{"id":"<settings_id>","key":"execution","value":{"approval_policy":"on-request","approvals_reviewer":"user"},"revision":1,"created_at":1700000000000,"updated_at":1700000000000}}
```

### settings.update

下例中的 `revision:1` 必须换成上一次 `settings.get` 返回的版本。`value` 是整个配置组，两个字段都要提供。

```json
{"jsonrpc":"2.0","id":"settings-update","method":"settings.update","params":{"key":"execution","revision":1,"value":{"approval_policy":"always","approvals_reviewer":"user"}}}
```

成功返回更新后的完整设置记录，`revision` 加一。再次携带旧版本保存，返回 `1003`；重新读取后再保存。`auto_review`、未知配置字段、缺少字段、重复键或错误类型返回 `-32602`。

修改成功后发起的新执行无需重启即可使用新设置。已受理、正在执行或等待审批的任务继续使用自己的快照，可通过 `run.get.result.settings_snapshot` 查看。设置更新当前不推送专门的 `settings.updated` 事件，其他窗口通过 `settings.get` 读取最新值。

审批设置不再接受启动参数 `--approval-mode`、`--approval-policy` 或 `--approvals-reviewer`。

## 6. 厂商配置

### provider.create

把占位值替换为实际根地址和密钥。当前协议固定使用 `chatcompletions`；根地址通常带 `/v1`，不要把 `/chat/completions` 请求路径当作根地址填写。

```json
{"jsonrpc":"2.0","id":"provider-create","method":"provider.create","params":{"name":"我的模型厂商","protocol":"chatcompletions","base_url":"https://<api_host>/v1","api_key":"<your_api_key>"}}
```

成功响应：

```json
{"jsonrpc":"2.0","id":"provider-create","result":{"id":"<provider_id>"}}
```

创建时可省略 `api_key`，数据库保存为空；需要认证的端点仍应提供有效密钥。根地址只接受 HTTP/HTTPS，不接受 URL 用户信息、查询参数或片段。

### provider.get / provider.list

```json
{"jsonrpc":"2.0","id":"provider-get","method":"provider.get","params":{"id":"<provider_id>"}}
{"jsonrpc":"2.0","id":"provider-list","method":"provider.list","params":{}}
```

`get` 返回单个对象，`list` 返回数组。对象字段如下：

```json
{"id":"<provider_id>","name":"我的模型厂商","protocol":"chatcompletions","base_url":"https://<api_host>/v1","has_api_key":true,"created_at":1700000000000,"updated_at":1700000000000}
```

查询不会返回密钥明文，`has_api_key` 仅表示是否保存了非空密钥。

### provider.update

厂商名称、协议、根地址按完整配置更新。省略 `api_key` 保留原密钥：

```json
{"jsonrpc":"2.0","id":"provider-update","method":"provider.update","params":{"id":"<provider_id>","name":"修改后的厂商名称","protocol":"chatcompletions","base_url":"https://<api_host>/v1"}}
```

需要替换密钥时额外提交字符串 `api_key`；空字符串表示清空，`null` 不被接受。成功返回 `{"id":"<provider_id>"}` 作为 `result`。

### provider.delete

```json
{"jsonrpc":"2.0","id":"provider-delete","method":"provider.delete","params":{"id":"<provider_id>"}}
```

仍有关联模型时返回 `1003`，需要删除关联模型后再删除厂商。厂商、模型删除属于实际删除配置记录，建议放在测试最后。

## 7. 模型配置

### model.create

普通文本测试可以使用以下请求：

```json
{"jsonrpc":"2.0","id":"model-create","method":"model.create","params":{"provider_id":"<provider_id>","display_name":"测试模型","remote_model_id":"<remote_model_id>","enabled":true,"model_type":[]}}
```

成功返回 `result.id`，将其保存为 `<model_id>`。`remote_model_id` 是厂商实际接受的模型标识，不是本地 UUID；同一厂商下不能重复。

完整配置字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `provider_id` | string | 厂商 UUIDv7，必填 |
| `display_name` | string | 客户端展示名称，必填 |
| `remote_model_id` | string | 厂商模型标识，必填 |
| `enabled` | boolean | 必须显式提交 `true` 或 `false` |
| `context_window` | integer/null | 已确认的上下文窗口；填写时必须大于零，未知则省略或 `null` |
| `max_output_tokens` | integer/null | 已确认的最大输出 Token 数；填写时必须大于零 |
| `capabilities` | object/null | 具体能力，支持 `image_input`、`tools`、`reasoning` 布尔值；`tools` 仅保留兼容元数据，不作为开关 |
| `model_type` | string[] | 可多选的内容类型：`text`、`image`、`audio`、`video`，不能重复；空数组表示尚未标注 |
| `reasoning_levels` | object[]/null | 每项包含 `display_name`、`remote_value`；远端值以真实厂商定义为准 |

普通对话与 Agent 默认向所有模型提供工具，无需填写 `capabilities.tools`，也不在 `model_type` 中选择工具。`capabilities:null`、`model_type:[]` 的配置同样可以调用工具。
`reasoning` 和推理等级仅作元数据，不自动传入推理参数，也不阻止文本请求。图片输入需 `capabilities.image_input:true`；标注 `model_type:["image"]` 不自动声明图片输入或输出。图片输出及音视频输入输出尚未接入，类型字段只保存元数据。
模型响应或流式分片含非 NULL 的 `audio` 字段时，执行明确失败，错误类型为 `unsupported`；`audio:null` 和未知厂商元数据可以保留。合法空文本响应仍可成功，音频内容不会被当作空文本成功处理。

上下文窗口、最大输出 Token 和推理等级属于配置元数据，不会自动变成每次请求的 `max_tokens` 或推理参数，也不用于本地 Token 预算检查。

### model.get / model.list

```json
{"jsonrpc":"2.0","id":"model-get","method":"model.get","params":{"id":"<model_id>"}}
{"jsonrpc":"2.0","id":"model-list-all","method":"model.list","params":{"selectable_only":false}}
{"jsonrpc":"2.0","id":"model-list-enabled","method":"model.list","params":{"selectable_only":true}}
```

`get` 返回单个模型对象；`list` 返回数组。对象包含上表配置字段，以及 `id`、`created_at`、`updated_at`。`selectable_only` 默认为 `false`；`true` 只过滤未启用模型，不保证当前适配器支持其选中的所有功能。

### model.update

完整替换配置，省略的可选元数据会被清空或恢复为空值。只切换启用状态时使用下一节的方法。

```json
{"jsonrpc":"2.0","id":"model-update","method":"model.update","params":{"id":"<model_id>","provider_id":"<provider_id>","display_name":"测试模型","remote_model_id":"<remote_model_id>","enabled":true,"context_window":null,"max_output_tokens":null,"capabilities":null,"model_type":[],"reasoning_levels":null}}
```

成功返回 `result.id`。

### model.set_enabled

```json
{"jsonrpc":"2.0","id":"model-disable","method":"model.set_enabled","params":{"id":"<model_id>","enabled":false}}
{"jsonrpc":"2.0","id":"model-enable","method":"model.set_enabled","params":{"id":"<model_id>","enabled":true}}
```

禁用后，新调用返回 `1004`。进行正常对话测试前恢复启用。成功返回 `result.id`。

### model.delete

```json
{"jsonrpc":"2.0","id":"model-delete","method":"model.delete","params":{"id":"<model_id>"}}
```

成功返回 `result.id`。已有会话、执行保留模型身份快照；继续调用仍需要可用的实时模型配置。

## 8. 普通对话与消息

### conversation.send：首次提问，非流式模型响应

```json
{"jsonrpc":"2.0","id":"chat-first","method":"conversation.send","params":{"model_id":"<model_id>","stream":false,"content":[{"type":"text","text":"我叫小雨，请记住我的名字。"}]}}
```

受理响应：

```json
{"jsonrpc":"2.0","id":"chat-first","result":{"conversation_id":"<conversation_id>","run_id":"<run_id>","user_message_id":"<user_message_id>","message_id":"<message_id>","removed_message_ids":null}}
```

- `user_message_id` 是用户问题的 ID；`message_id` 是模型回答消息的 ID。
- 首次提问不提供 `conversation_id`；服务同时创建会话、消息、执行和初始步骤。
- `stream:false` 表示向模型发非流式请求，Callum 仍然异步受理，回答不在这条 RPC 的 `result` 中。
- 等待 `message.updated` 获取完整回答，并以 `run.completed`、`run.failed`、`run.cancelled` 判断终态。
- 默认标题来自首条问题前 30 个 Unicode 码点；辅助标题与回答并行，事件可能交错。

### conversation.send：继续同一会话，流式回答

等待前一次执行终态后发送：

```json
{"jsonrpc":"2.0","id":"chat-next","method":"conversation.send","params":{"conversation_id":"<conversation_id>","stream":true,"content":[{"type":"text","text":"我叫什么名字？"}]}}
```

服务自动读取历史；客户端只传当前问题。`stream` 省略时默认为 `true`。已有会话可以省略 `model_id`；显式传入另一个可用模型时，会更新会话采用的模型。

文本增量事件示例：

```json
{"jsonrpc":"2.0","method":"message.delta","params":{"conversation_id":"<conversation_id>","run_id":"<run_id>","step_id":"<step_id>","message_id":"<message_id>","block_index":0,"delta":{"type":"text","text":"你叫小雨"}}}
```

文本片段不保证按词、句或完整 JSON 切分，客户端按对应消息追加。收到 `message.updated` 时使用其中完整的 `message.content` 替换本地消息内容。

### conversation.messages

```json
{"jsonrpc":"2.0","id":"messages-latest","method":"conversation.messages","params":{"conversation_id":"<conversation_id>","limit":100}}
{"jsonrpc":"2.0","id":"messages-before","method":"conversation.messages","params":{"conversation_id":"<conversation_id>","before_sequence":3,"limit":100}}
```

返回消息数组。`before_sequence` 为严格小于该顺序号，省略时读取最近一页；每页内部按 `sequence` 升序排列。`limit` 默认 100，范围 1 至 200；受内容字节限制影响，一页可能少于请求数量。

消息对象示例：

```json
{"id":"<message_id>","conversation_id":"<conversation_id>","run_id":"<run_id>","sequence":2,"role":"assistant","content":[{"type":"text","text":"好的，我记住了，你叫小雨。"}],"status":"completed","created_at":1700000000000,"updated_at":1700000001000,"completed_at":1700000001000}
```

顺序从 1 开始：用户问题为 1、模型回答为 2、下一条问题为 3。工具调用和审批保存在步骤表，不作为对话消息插入。

### conversation.resend

等原执行结束，从消息列表取最后一条 `role:user` 消息的 ID。这里必须使用用户消息 ID：

```json
{"jsonrpc":"2.0","id":"chat-resend","method":"conversation.resend","params":{"conversation_id":"<conversation_id>","message_id":"<last_user_message_id>","stream":true,"content":[{"type":"text","text":"更正一下，我叫小宇，请按这个名字回答。"}]}}
```

受理结果仍返回会话、执行及消息 ID。用户消息 ID 和顺序保持，回答消息及执行使用新 ID；`removed_message_ids` 列出被清除的旧回答消息 ID。旧消息分支不保留，实际工具和审批审计保留；旧模型响应副本清理。

编辑较早的用户消息、在执行中重发、把回答消息 ID 当作用户消息 ID，均不会正常受理。`conversation.send` 不接受 `message_id`，重发必须使用 `conversation.resend`。

## 9. 会话管理

### conversation.get / conversation.list

```json
{"jsonrpc":"2.0","id":"chat-get","method":"conversation.get","params":{"id":"<conversation_id>"}}
{"jsonrpc":"2.0","id":"chat-list","method":"conversation.list","params":{"archived":false,"deleted":false,"limit":100,"offset":0}}
{"jsonrpc":"2.0","id":"chat-list-workspace","method":"conversation.list","params":{"workspace_id":"<workspace_id>","limit":100,"offset":0}}
{"jsonrpc":"2.0","id":"chat-list-unbound","method":"conversation.list","params":{"unbound":true,"limit":100,"offset":0}}
```

`get` 返回单个会话对象；`list` 返回数组。会话字段为 `id`、`workspace_id`、`title`、`title_source`、模型身份快照 `model`、`pinned_at`、`archived_at`、`deleted_at`、`created_at`、`updated_at`。

`workspace_id` 与 `unbound:true` 不可同时设置。默认列出未归档、未删除且工作区未删除的会话；`archived:true` 选择已归档记录，`deleted:true` 选择已删除记录。分页默认 `limit:100`、`offset:0`，`limit` 为 1 至 200。

### conversation.update

除 `id` 外仅发送需要修改的字段：

```json
{"jsonrpc":"2.0","id":"chat-title","method":"conversation.update","params":{"id":"<conversation_id>","title":"手动设置的标题"}}
{"jsonrpc":"2.0","id":"chat-pin","method":"conversation.update","params":{"id":"<conversation_id>","pinned":true}}
{"jsonrpc":"2.0","id":"chat-unpin","method":"conversation.update","params":{"id":"<conversation_id>","pinned":false}}
{"jsonrpc":"2.0","id":"chat-archive","method":"conversation.update","params":{"id":"<conversation_id>","archived":true}}
{"jsonrpc":"2.0","id":"chat-unarchive","method":"conversation.update","params":{"id":"<conversation_id>","archived":false}}
{"jsonrpc":"2.0","id":"chat-workspace","method":"conversation.update","params":{"id":"<conversation_id>","workspace_id":"<workspace_id>"}}
{"jsonrpc":"2.0","id":"chat-detach-workspace","method":"conversation.update","params":{"id":"<conversation_id>","detach_workspace":true}}
{"jsonrpc":"2.0","id":"chat-model","method":"conversation.update","params":{"id":"<conversation_id>","model_id":"<another_model_id>"}}
```

成功返回更新后的会话对象。人工修改标题后 `title_source` 为 `manual`，辅助标题不覆盖人工标题。改变模型、工作区、归档或删除状态需要该会话没有活动执行。绑定工作区与 `detach_workspace:true` 不可同时提交。

已有会话不能通过 `conversation.send.workspace_id` 改绑工作区，应使用本接口。

### conversation.delete / conversation.restore

```json
{"jsonrpc":"2.0","id":"chat-delete","method":"conversation.delete","params":{"id":"<conversation_id>"}}
{"jsonrpc":"2.0","id":"chat-list-deleted","method":"conversation.list","params":{"deleted":true,"limit":100,"offset":0}}
{"jsonrpc":"2.0","id":"chat-restore","method":"conversation.restore","params":{"id":"<conversation_id>"}}
```

会话删除是软删除，返回更新后的会话对象。恢复清除删除状态，原有归档状态不会自动清除；如仍已归档，应执行 `conversation.update`，设置 `archived:false` 后继续对话。

## 10. 工作区

### workspace.create / workspace.get / workspace.list

把 `<workspace_path>` 换成第 1 节输出的绝对路径。工具测试需要该目录实际存在并有可读文件。

```json
{"jsonrpc":"2.0","id":"workspace-create","method":"workspace.create","params":{"name":"JSON-RPC 测试工作区","root_path":"<workspace_path>"}}
{"jsonrpc":"2.0","id":"workspace-get","method":"workspace.get","params":{"id":"<workspace_id>"}}
{"jsonrpc":"2.0","id":"workspace-list","method":"workspace.list","params":{"archived":false,"deleted":false,"limit":100,"offset":0}}
```

`create`、`get` 返回工作区对象：

```json
{"id":"<workspace_id>","name":"JSON-RPC 测试工作区","root_path":"<workspace_path>","pinned_at":null,"archived_at":null,"deleted_at":null,"created_at":1700000000000,"updated_at":1700000000000}
```

`list` 返回对象数组。过滤与分页默认值同会话列表；`limit` 为 1 至 200。工作区根路径必须是绝对路径，重复的规范化路径返回冲突。

### workspace.update

```json
{"jsonrpc":"2.0","id":"workspace-update","method":"workspace.update","params":{"id":"<workspace_id>","name":"修改后的工作区名称","pinned":true}}
{"jsonrpc":"2.0","id":"workspace-unpin","method":"workspace.update","params":{"id":"<workspace_id>","pinned":false}}
{"jsonrpc":"2.0","id":"workspace-archive","method":"workspace.update","params":{"id":"<workspace_id>","archived":true}}
{"jsonrpc":"2.0","id":"workspace-unarchive","method":"workspace.update","params":{"id":"<workspace_id>","archived":false}}
```

返回更新后的工作区对象；本接口不接受 `root_path`。工作区有关联活动执行时，归档或删除状态变更会返回 `1006`。

### workspace.delete / workspace.restore

```json
{"jsonrpc":"2.0","id":"workspace-delete","method":"workspace.delete","params":{"id":"<workspace_id>"}}
{"jsonrpc":"2.0","id":"workspace-list-deleted","method":"workspace.list","params":{"deleted":true,"limit":100,"offset":0}}
{"jsonrpc":"2.0","id":"workspace-restore","method":"workspace.restore","params":{"id":"<workspace_id>"}}
```

工作区软删除不删除磁盘上的目录和文件。恢复仅清除删除状态；归档状态如需清除，另发 `workspace.update`。

## 11. 工具调用及审批

### 11.1 默认工具调用

使用已有启用模型即可。所有模型在普通对话中默认获得工具定义，不需要更新模型能力或类型；已有 `capabilities.tools:false` 也不会关闭工具。
若要验证逐次审批，读取最新设置并保存 `approval_policy:always`，使用实际 `revision`。必须在发送本次问题之前保存，因为受理后设置已固定成快照。

绑定工作区创建会话，可在问题中使用相对路径或工作区内绝对路径：

```json
{"jsonrpc":"2.0","id":"chat-tools","method":"conversation.send","params":{"model_id":"<model_id>","workspace_id":"<workspace_id>","stream":true,"content":[{"type":"text","text":"请使用文件读取工具读取工作区的 note.txt，告诉我完整内容。"}]}}
```

也可以不创建、不绑定工作区，直接提供第 1 节临时测试文件的绝对路径，将 `<workspace_path>` 替换为实际值：

```json
{"jsonrpc":"2.0","id":"chat-file-without-workspace","method":"conversation.send","params":{"model_id":"<model_id>","stream":true,"content":[{"type":"text","text":"请使用文件读取工具读取 <workspace_path>/note.txt，根据实际内容回答。"}]}}
```

工具提供不以绑定工作区为前提。无工作区时文件工具必须使用绝对路径；相对路径会得到 `path_base_required` 工具错误，不会以 Go 服务当前目录作为基准。

客户端不提供 `tool`、工具参数或“已批准”字段。只有模型实际发起工具调用时，才出现 `tool_call` 步骤及相应审批；纯文本回答不会触发审批。

当前提供的模型工具如下，它们不是 JSON-RPC 方法：

| 工具 | 主要参数 | 用途 |
| --- | --- | --- |
| `list_directory` | `path`，可选 `limit` | 查看指定目录；有工作区时可用 `path:"."` |
| `read_file` | `path`，可选 `start_line`、`end_line` | 按行读取 UTF-8 文本，例如 `path:"note.txt"` |
| `search_text` | `path`、`query`，可选 `limit` | 搜索字面文本 |

三个工具都接受绝对路径；相对路径需要工作区且不能使用 `..` 越界。工作区内绝对路径与相对路径采用相同规则，符号链接按实际目标判断范围。
无工作区或访问工作区外目标时，`on-request` 请求用户审批，`always` 请求审批，`never` 拒绝操作。批准只授权该次固定目标，不授权父目录或其他文件。
目录访问限制在本次目标目录内，搜索不跟随子项符号链接；等待期间目标被替换或链接被重定向时，批准后仍拒绝执行。当前没有文件写入、删除、Shell、MCP 或插件工具。
`search_text` 搜索空目录返回空匹配，遇到空子目录继续搜索；空目录不作为工具错误。

### 11.2 approval.get / approval.list

```json
{"jsonrpc":"2.0","id":"approval-pending","method":"approval.list","params":{"run_id":"<run_id>","status":"pending","limit":100,"offset":0}}
{"jsonrpc":"2.0","id":"approval-get","method":"approval.get","params":{"id":"<approval_id>"}}
```

`get` 返回单条审批，`list` 返回数组。`run_id`、`status` 都可省略；状态支持 `pending`、`approved`、`rejected`、`cancelled`。分页默认 100，最大 200。

审批字段包括 `id`、`run_id`、`step_id`、`request`、`request_hash`、`status`、`decided_by`、`decision_note`、`created_at`、`updated_at`、`decided_at`。

`request` 是 Go 生成的工具定义、可选工作区、实际目标和规范化参数快照；`request_hash` 绑定本次操作。审批没有过期时间。
无工作区时 `request.workspace` 为 `null`。文件工具的 `request.target` 包含 `requested_path`（请求绝对路径，相对路径已基于工作区解析）、`path`（解析符号链接后的实际绝对路径）、`kind`（`file` 或 `directory`）、`within_workspace`（实际目标是否在工作区内）。这些字段由服务端生成，客户端只提交审批决定及说明。

待审批通知示例，下面只展示事件定位和待决定状态所需的关键字段；实际 `approval` 还包含上述操作快照、哈希及时间字段：

```json
{"jsonrpc":"2.0","method":"approval.requested","params":{"conversation_id":"<conversation_id>","run_id":"<run_id>","step_id":"<step_id>","approval_id":"<approval_id>","approval":{"id":"<approval_id>","run_id":"<run_id>","step_id":"<step_id>","status":"pending","decided_by":null,"decision_note":null,"decided_at":null}}}
```

### 11.3 approval.decide

批准：

```json
{"jsonrpc":"2.0","id":"approval-approve","method":"approval.decide","params":{"approval_id":"<approval_id>","decision":"approved","decision_note":"允许本次读取"}}
```

或者拒绝同一条尚待决定的审批：

```json
{"jsonrpc":"2.0","id":"approval-reject","method":"approval.decide","params":{"approval_id":"<approval_id>","decision":"rejected","decision_note":"不允许本次操作"}}
```

二选一进行测试；同一决定重复提交保持幂等，提交相反决定返回 `1003`。说明可省略，提供时必须是最多 4096 字节的 UTF-8 文本。

返回保存后的完整审批记录；用户身份由服务写入为 `local_user`。不能由客户端提供或修改 `request`、`request_hash`、`decided_by`。

批准保存成功不代表工具或整个执行已完成，继续等待执行终态。拒绝会将结构化拒绝结果交给模型，模型可以解释或选择其他允许的操作，不必导致 `run.failed`。同一执行中相同被拒绝操作不会重复申请审批。

待审批时可使用 `run.cancel` 取消整个执行，审批随之进入 `cancelled`。`approval.decide` 不接受 `cancelled`、`expired` 或 `user_input` 作为决定。

### 11.4 三种策略的测试区别

| 策略 | 工作区内读取（含绝对路径） | 无工作区／工作区外访问及其他需要审批的操作 |
| --- | --- | --- |
| `on-request` | 自动执行，不创建虚假的用户批准记录 | 询问用户 |
| `always` | 每次有效调用都询问 | 询问用户 |
| `never` | 自动执行 | 直接拒绝，不创建待审批记录 |

当前产品工具均为读取工具，手工测试 `never` 时工作区内读取仍会成功，无工作区及工作区外读取直接拒绝。删除、外发等类别的拒绝规则通过受控工具测试验证，并不表示当前存在相应产品工具。

## 12. 图片输入

仅在厂商模型实际支持图片输入时执行。以下完整更新声明图片输入能力，并标注文本、图片内容类型；其余可选元数据会清空：

```json
{"jsonrpc":"2.0","id":"model-enable-image","method":"model.update","params":{"id":"<model_id>","provider_id":"<provider_id>","display_name":"测试模型","remote_model_id":"<remote_model_id>","enabled":true,"capabilities":{"image_input":true},"model_type":["text","image"]}}
```

工具默认提供，不需要为图片与工具组合另设开关。类型数组可以为空；是否允许图片输入取决于 `capabilities.image_input`，不从类型数组推断。

图片内容块使用本地绝对路径：

```json
{"jsonrpc":"2.0","id":"chat-image","method":"conversation.send","params":{"model_id":"<model_id>","stream":true,"content":[{"type":"text","text":"请描述这张图片。"},{"type":"image","path":"<absolute_image_path>"}]}}
```

图片由适配器读取并转换，客户端不传 Base64 或远端协议格式。文本块使用 `text`，图片块使用 `path`；图片输出尚未启用。
读取只接受核对身份后的普通文件，使用已打开的固定句柄。Unix 使用非阻塞打开；路径在检查后被替换为命名管道时明确失败，不等待管道写端。

## 13. 执行查询和取消

### run.get / run.list

```json
{"jsonrpc":"2.0","id":"run-get","method":"run.get","params":{"id":"<run_id>"}}
{"jsonrpc":"2.0","id":"run-list","method":"run.list","params":{"conversation_id":"<conversation_id>","limit":100,"offset":0}}
```

`get` 返回执行对象，`list` 返回数组。分页默认 100，最大 200。

执行字段如下：

| 字段 | 说明 |
| --- | --- |
| `id`、`conversation_id` | 执行与会话 ID |
| `replaces_run_id` | 编辑重发时被替代执行的 ID，其他情况为 `null` |
| `model` | 模型及厂商身份快照，含本地 ID、展示名和远端标识 |
| `stream` | 本次模型请求是否流式 |
| `settings_snapshot` | 实际设置及 `revision`；旧执行可能为 `null` |
| `status` | 当前执行状态 |
| `error_kind`、`error_message` | 执行错误，正常完成时为 `null` |
| `created_at`、`updated_at`、`finished_at` | Unix 毫秒时间，未结束时完成时间为 `null` |

当前运行流程产生 `running`、`awaiting_approval`、`completed`、`failed`、`cancelled` 状态；重启恢复可将未结束执行标为 `interrupted`。`awaiting_user_input` 虽已在结构中定义，补充输入流程尚未接入。

### run.cancel

```json
{"jsonrpc":"2.0","id":"run-cancel","method":"run.cancel","params":{"run_id":"<run_id>"}}
```

响应：

```json
{"jsonrpc":"2.0","id":"run-cancel","result":{"cancel_requested":true}}
```

返回值表示是否请求取消，不等待清理完成。执行不存在或已确定终态时返回 `false`；已请求取消后仍以终态事件或查询结果为准。

同一会话再次发送会返回忙碌错误；测试并行时，省略 `conversation_id` 发起另一条首次提问，或使用另一个已存在的会话。取消一个执行不会取消其他会话。

## 14. 步骤、原始模型响应和用量

### step.list / step.get

```json
{"jsonrpc":"2.0","id":"step-list","method":"step.list","params":{"run_id":"<run_id>","after_sequence":0,"limit":100}}
{"jsonrpc":"2.0","id":"step-get","method":"step.get","params":{"id":"<step_id>"}}
```

`step.list` 选择 `sequence > after_sequence` 的记录，默认从 0 开始，`limit` 默认 100、最大 200。`step.get` 返回单条步骤元数据。

步骤包含 `id`、`conversation_id`、`run_id`、`parent_step_id`、`sequence`、`type`、`name`、`status`、`input`、模型身份、请求／工具调用标识、错误和时间字段，以及 `output_bytes`、`usage_bytes`、`definition_bytes`。

通知、列表和 `step.get` 均将大型 `output`、`usage`、`definition` 置为 `null`，这不代表数据库中没有数据。通过下面的 `step.data` 获取原始字段。

当前实际创建 `model_call`、`tool_call` 步骤；辅助标题也是单独的 `model_call`。工具步骤通过 `parent_step_id` 关联产生调用的模型步骤，适合客户端按层级折叠和展开。

### step.data

```json
{"jsonrpc":"2.0","id":"step-input","method":"step.data","params":{"step_id":"<step_id>","field":"input","offset":0,"limit":65536}}
{"jsonrpc":"2.0","id":"step-output","method":"step.data","params":{"step_id":"<step_id>","field":"output","offset":0,"limit":65536}}
{"jsonrpc":"2.0","id":"step-usage","method":"step.data","params":{"step_id":"<step_id>","field":"usage","offset":0,"limit":65536}}
{"jsonrpc":"2.0","id":"step-definition","method":"step.data","params":{"step_id":"<step_id>","field":"definition","offset":0,"limit":65536}}
```

只支持 `input`、`output`、`usage`、`definition` 四个字段。`offset` 是字节偏移，必须非负且位于合法 UTF-8 边界；`limit` 默认 65536，范围 4 至 65536。

usage 片段响应示例：

```json
{"jsonrpc":"2.0","id":"step-usage","result":{"step_id":"<step_id>","field":"usage","present":true,"offset":0,"next_offset":18,"total_bytes":18,"done":true,"fragment":"{\"total_tokens\":7}"}}
```

`fragment` 是字符串，拼接完整后再作为 JSON 解析。未读完时，把返回的 `next_offset` 原样作为下一次请求的 `offset`；不要按 Unicode 字符数自行计算偏移。字段缺失时 `present:false`、片段为空，不生成估算用量。

- 模型调用步骤的 `output` 保存协议适配后的完整结果及相关原始响应数据；使用返回的 JSON 格式标识解释内容。
- 模型调用步骤的 `usage` 原样保存厂商返回的用量；主回答、每轮继续调用及辅助标题分别记录，不能只看第一条步骤。
- 工具步骤的 `input` 保存规范化请求、哈希、审批策略、审批人、`settings_revision` 和权限判定。
- `authorization.action` 为 `allow`、`ask`、`deny`；它是规则判定，实际批准或拒绝通过审批记录查询。
- 新工具步骤的 `input.operation_version` 为 `1`。`output` 保存 `result`、`operation`、可选的独立受控 `error`，以及上下文恢复所需的统一消息。错误时仍保留已获得的有效实际输出；没有有效输出的失败保持 `result.error` 结构。

工具结果中的 `operation` 由服务端保存，例如：

```json
{"result":{"written":true},"operation":{"version":1,"outcome":"applied","request_hash":"<fixed_operation_hash>"},"error":{"kind":"cancelled","message":"操作已取消"}}
```

此例说明“操作已生效、调用随后取消”的记录格式；当前内置工具仍为读取工具，并未开放文件修改接口。
`outcome` 为 `none`（无副作用）、`not_applied`（确认未生效）、`applied`（已生效）、`partially_applied`（部分生效）或 `unknown`（无法确认）；不能仅用步骤或运行的失败／取消状态判断是否生效。
恢复未完成的新步骤时附加 `recovered:true`，已保存的明确事实及实际输出保留，未启动的调用为 `not_applied`，已启动但缺少结果的只读调用为 `none`，其他调用为 `unknown`。旧记录不补造这些字段，数据库结构版本不变。

后续对话会恢复当前分支中失败、取消或中断执行留下的已生效、部分生效或未知操作，保持工具调用及结果配对，不恢复失败回答正文。同轮未执行的调用补齐为未执行；编辑重发前的旧分支仍只作审计。
同一固定操作留下未知、部分生效或已生效但报错的结果后，再次调用会要求用户确认，`authorization.reason` 为 `previous_result_requires_confirmation`；`never` 直接拒绝。批准仍需通过执行前校验，明确成功才解除待核实状态；不会自动重试未知操作。
本节字段通过现有 `step.data` 读取，通知结构和 JSON-RPC 方法不变。工具原始输出仍限 64 KiB，外层事实及上下文封装不计入该工具输出上限。

## 15. 无历史模型调用 model.invoke

此接口用于兼容调用，不创建会话、消息或持久化执行。客户端自己提交完整文本上下文；只接受 `system`、`user`、`assistant` 角色及字符串 `content`。

```json
{"jsonrpc":"2.0","id":"invoke-stream","method":"model.invoke","params":{"model_id":"<model_id>","stream":true,"messages":[{"role":"system","content":"请用中文回答。"},{"role":"user","content":"你好，请介绍一下你自己。"}]}}
```

非流式模型请求：

```json
{"jsonrpc":"2.0","id":"invoke-json","method":"model.invoke","params":{"model_id":"<model_id>","stream":false,"messages":[{"role":"user","content":"用一句话说明 Go 的用途。"}]}}
```

两者均先返回：

```json
{"jsonrpc":"2.0","id":"invoke-stream","result":{"run_id":"<invoke_run_id>"}}
```

`stream` 默认 `true`。流式增量使用 `run.text_delta`：

```json
{"jsonrpc":"2.0","method":"run.text_delta","params":{"run_id":"<invoke_run_id>","text":"你好","provider_request_id":"<provider_request_id>"}}
```

非流式完整回答在完成事件中：

```json
{"jsonrpc":"2.0","method":"run.completed","params":{"run_id":"<invoke_run_id>","text":"Go 适合构建网络服务和命令行工具。","provider_request_id":"<provider_request_id>"}}
```

流式完成事件不要求重复携带完整 `text`，客户端累计前面的增量。失败／取消事件的 `params.error` 为带 `code`、`message`、可选 `data` 的受控错误对象。

此接口只接受文本，不提供工具定义或工具循环；模型内容类型及能力元数据不阻止文本请求。图片输入和工具审批使用 `conversation.send` 测试。

`<invoke_run_id>` 是临时执行标识，不是持久化 UUIDv7；可用于 `run.cancel`，不能用于 `run.get`、`run.list` 或 `step.list`。

## 16. 服务端事件与界面处理

### 16.1 普通对话事件

| 事件 | 主要 `params` 字段 | 处理方式 |
| --- | --- | --- |
| `system.ready` | 服务信息 | 当前连接已就绪 |
| `conversation.created` | `conversation_id`、`run_id`、`conversation` | 首次问题创建会话 |
| `conversation.updated` | `conversation_id`、`conversation`，执行内通常还有 `run_id` | 更新会话属性或标题 |
| `run.started` | `conversation_id`、`run_id`、`run` | 记录受理执行及设置快照 |
| `run.updated` | `conversation_id`、`run_id`、`run` | 更新运行／等待审批状态 |
| `message.created` | 会话、执行、消息标识及 `message` | 新增用户或回答消息 |
| `message.delta` | 消息标识、`block_index`、`delta` | 追加对应文本增量 |
| `message.updated` | 消息标识及完整 `message` | 替换当前消息，包括工具调用轮暂存内容清空 |
| `step.created` | 会话、执行、步骤标识及 `step` | 新增模型／工具步骤 |
| `step.updated` | 标识及 `step` | 更新步骤状态和数据大小，按需查询原始字段 |
| `approval.requested` | 会话、执行、步骤标识、`approval_id`、`approval` | 展示确定操作，等待用户决定 |
| `approval.updated` | 同上 | 更新审批决定；不能当作执行完成 |
| `run.completed` | 会话、执行标识及 `run` | 本次执行完成 |
| `run.failed` | 标识、`run`、通常还有 `error` | 展示失败；最终持久化失败时可能含 `persisted:false` |
| `run.cancelled` | 标识、`run`、通常还有 `error` | 本次执行取消 |

普通对话失败事件中，`params.error` 形状是 `{"kind":"incomplete","message":"模型输出未完整完成"}`；与无历史调用的错误对象不同。

对于同一次执行，受理响应排在该执行事件之前；不同会话、标题任务和多个连接的事件可能交错。通过 `conversation_id`、`run_id`、`message_id`、`step_id` 区分，不依赖所有步骤的固定到达顺序。

工具调用过程中可能先出现文本增量，再用 `message.updated` 清空暂存正文，接着继续生成最终回答。客户端收到完整消息更新时应执行替换，不能始终累加。

### 16.2 无历史调用事件

`model.invoke` 使用 `run.text_delta`、`run.completed`、`run.failed`、`run.cancelled`，其 `params` 不包含持久化 `run` 对象或 `conversation_id`。

当前不发送 `step.output_delta`、`user_input.requested`、`user_input.updated`、`run.interrupted` 或上下文压缩专用通知。重启恢复中断状态及新工具步骤的保守操作事实，客户端查询执行记录获取结果，不承诺重放旧事件。

## 17. 错误码与负向测试

| 错误码 | 常见含义 |
| --- | --- |
| `-32700` | JSON 语法或 UTF-8 无效 |
| `-32600` | 请求对象无效、协议版本错误、批量数组或 ID 类型无效 |
| `-32601` | 接口不存在 |
| `-32602` | 参数缺失、字段未知、类型错误或业务参数无效 |
| `-32603` | 内部处理、存储或查询失败 |
| `1001` | 对应资源不存在 |
| `1002` | 同一厂商下远端模型标识重复 |
| `1003` | 按接口表示设置版本冲突、审批改判冲突、状态限制、关联引用或工作区路径重复等 |
| `1004` | 模型已禁用 |
| `1005` | 所选功能或输出不受当前适配器支持 |
| `1006` | 同会话忙碌、工作区有活动执行或全局活动执行达到上限 |
| `1007` | 操作取消 |
| `1008` | 操作超时 |
| `1009` | 无历史模型调用的受控模型错误等，结合 `error.data.kind` 判断 |
| `1010` | 输出超过字节限制 |
| `1011` | 服务正在关闭 |

错误码存在跨接口复用，应结合方法、`message` 和实际存在的 `data.kind` 解读。普通对话执行后的失败主要通过终态事件中的 `error.kind`、`run.error_kind` 表达，不把 RPC 受理成功当作模型调用成功。

以下 JSON 均可单独发送，用于测试协议校验：

```json
{"jsonrpc":"2.0","id":"bad-method","method":"missing.method","params":{}}
{"jsonrpc":"2.0","id":"bad-field","method":"system.info","params":{"unknown":true}}
{"jsonrpc":"2.0","id":"bad-id","method":"model.get","params":{"id":"not-a-uuid"}}
{"jsonrpc":"2.0","id":"bad-reviewer","method":"settings.update","params":{"key":"execution","revision":1,"value":{"approval_policy":"always","approvals_reviewer":"auto_review"}}}
```

预期依次为 `-32601`、`-32602`、`-32602`、`-32602`。另可手动输入不完整 JSON 验证 `-32700`。

业务边界建议逐项验证：

| 测试 | 预期 |
| --- | --- |
| 保存设置后用旧 `revision` 再保存 | `1003`，原设置不被覆盖 |
| 禁用模型后发起调用 | `1004` |
| 同一会话执行中再次发送 | `1006` |
| 另一个会话同时发送，再取消其中一个 | 其他会话继续；取消结果按事件或查询确认 |
| 将较早的用户消息用于重发 | 拒绝，不能创建旧分支 |
| 待审批时查询及修改应用设置 | 查询／更新可完成，旧任务仍保持旧快照 |
| 批准后重复提交相同决定 | 成功且不重复执行同一步骤 |
| 对已批准的审批再提交拒绝 | `1003` |
| 拒绝后模型再次请求相同操作 | 不再次申请审批，不自动执行该操作 |
| 已归档或删除会话继续发送 | 状态错误；恢复并取消归档后可重试 |
| 工具读取工作区内绝对路径 | 与对应相对路径采用相同审批规则 |
| 工具尝试工作区外路径／无工作区绝对路径 | `on-request`／`always` 等待单次审批；`never` 拒绝 |
| 无工作区时工具使用相对路径 | 工具结果为 `path_base_required` |
| 等待审批时目标文件或符号链接被替换 | 批准后仍拒绝执行，不读取替换目标 |
| 进程关闭后重新启动同一测试数据目录 | 设置和消息保留，未结束执行标为中断，不自动重试 |

## 18. 实现依据

- 请求、参数和逐行协议：[protocol](/Users/xiaoyu/www/go/works/callum/internal/stdio/protocol)、[transport](/Users/xiaoyu/www/go/works/callum/internal/stdio/transport)。
- 路由及就绪信息：[router.go](/Users/xiaoyu/www/go/works/callum/internal/stdio/router.go)、[server.go](/Users/xiaoyu/www/go/works/callum/internal/stdio/server.go)。
- 配置接口：[catalog](/Users/xiaoyu/www/go/works/callum/internal/stdio/catalog)、[settings](/Users/xiaoyu/www/go/works/callum/internal/stdio/settings)。
- 对话及执行接口：[conversation](/Users/xiaoyu/www/go/works/callum/internal/stdio/conversation)、[execution](/Users/xiaoyu/www/go/works/callum/internal/stdio/execution)、[approval](/Users/xiaoyu/www/go/works/callum/internal/stdio/approval)、[workspace](/Users/xiaoyu/www/go/works/callum/internal/stdio/workspace)。
- 进一步的数据与执行约定见 [对话设计](design/conversations.md)。
