# 上下文统计与压缩

本文件说明 Token 估算、输入预算、压缩设置及摘要恢复规则。请求字段和完整联调步骤见 [JSON-RPC 协议](jsonrpc.md#91-上下文压缩)，执行轨迹及事务设计见 [对话设计](design/conversations.md)。

## 上下文统计

`conversation.context_usage` 提供不调用模型、不保存草稿的只读上下文统计。新对话传 `model_id` 和可选 `workspace_id`、`draft`；已有对话传 `conversation_id`、可选 `model_id` 与 `draft`，不得再指定 workspace_id。统计共用实际请求的系统指令、项目目录、工具定义、有效摘要、未压缩历史和工具轨迹；活动执行还计入已接受的流式正文与推理。执行期间使用受理时的模型及设置快照，空闲预览使用当前配置。过长轨迹无法安全恢复时返回 unavailable，不显示局部计数。

`estimated_tokens` 与 `actual_input_tokens` 分开：前者是当前有效上下文及草稿的估算，后者是本模型最近一次主调用返回的实际输入用量，缺失保持 null，不累加 usage 或解释厂商私有结果。实际值作为统一投影保存在现有模型步骤 output.context_usage，旧记录保持未知。`input_limit` 为扣除输出及安全预留后的可用输入预算；模型窗口或输入预算未知时保持 null，不推测容量。活动执行通过 `conversation.context_usage.updated` 发送数字快照，高频通知最多每 100ms 一次；结束发送 refresh_required，客户端重新读取持久化历史。缓存仅持有活动执行的数字，结束释放；无数据库结构变更。

## 压缩设置与输入预算

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

## 调用与结果

```json
{"jsonrpc":"2.0","id":"compact-settings","method":"settings.get","params":{"key":"context_compaction"}}
{"jsonrpc":"2.0","id":"compact-save","method":"settings.update","params":{"key":"context_compaction","revision":1,"value":{"enabled":true,"token_budget":0,"output_reserve":4096,"safety_margin":1024,"image_token_estimate":8192,"retain_turns":4}}}
{"jsonrpc":"2.0","id":"compact-run","method":"conversation.compact","params":{"conversation_id":"<conversation_id>"}}
{"jsonrpc":"2.0","id":"compact-versions","method":"conversation.context_versions","params":{"conversation_id":"<conversation_id>","limit":50,"offset":0}}
{"jsonrpc":"2.0","id":"compact-detail","method":"conversation.context_version","params":{"conversation_id":"<conversation_id>","id":"<context_version_id>"}}
```

写设置时必须使用实际读取到的 revision。压缩受理只返回 `conversation_id`、`run_id`，保持 stdin 打开并等待执行终态。没有较早完整轮次时不调用模型，步骤结果为 `compressed:false`。手动压缩可指定 `model_id`，省略时使用会话当前模型；自动压缩使用本次回答模型。摘要调用不提供工具，原始响应和实际 usage 独立保存在 `context_compression` 下的 `model_call` 子步骤，父步骤不重复计量。

## 摘要版本与检查点

摘要版本保存本次新增覆盖的消息 ID、运行 ID、顺序、消息与工具轨迹指纹，通过 `base_version_id` 追溯较早摘要。激活时在一个事务中重查来源、切换唯一 active 版本并完成编排步骤。取消、失败或重启不会激活未完成产物；已经提交的有效版本保留。编辑重发在同一受理事务中使覆盖被编辑消息的摘要失效，并清除其摘要正文及生成该摘要的模型结果副本；未受影响的有效摘要及其模型结果继续保留。

后续回答使用“有效摘要＋未压缩历史”；摘要作为有来源的历史数据，不作为权限或新的用户指令。待核实的工具操作另从服务端审计记录读取：摘要错误地声称成功也不能解除重新确认要求；单次最多恢复 4096 个待核实固定操作，超限明确失败。摘要校验结构和大小，不能证明语义完全无损；原文可通过消息、步骤接口核对。图片来源指纹覆盖消息中的附件引用，托管引用包含原图内容哈希。压缩保留图片副本。

摘要为包含 `goals`、`constraints`、`decisions`、`facts`、`open_questions` 五个字符串数组的 JSON 文本，最多 64 KiB。`size_stats` 记录压缩前后的消息数、JSON 字节数及 estimated_before_tokens、estimated_after_tokens、estimation_method；估算针对被摘要替换的历史部分，不是完整请求或 usage。摘要 Token 估算未减少、结构不合法、超时、来源变化或保存失败均使本次压缩失败，旧有效版本继续保留。摘要请求自身也按 Token 预算规划源批次，并保留每批最多 248 条模型消息、7 MiB 的安全边界；沿完整轮次边界持续分批，手动压缩直到没有可压缩的较早轮次，自动压缩直到请求回到预算内；不设固定批次上限。每批必须消费新的来源范围并缩小摘要 Token 估算，无进展时明确失败。当前问题、正在执行的工具批次、保留轮次或不可拆分的单个批次无法满足预算或安全边界时明确失败。

执行内步骤摘要保存在 `context_compression.output.checkpoint`，格式版本为 `1`，记录来源执行、父检查点、覆盖步骤范围、批次指纹、结构化摘要及大小统计。摘要请求每批最多 248 条来源消息、7 MiB；提交时在短事务中复查完整批次、来源指纹和父检查点，并原子完成压缩步骤。摘要必须同时减少 JSON 字节和估算 Token，失败或取消保留此前有效检查点。后续请求和重启恢复使用“步骤摘要＋未覆盖工具轨迹”；当前问题、未完成批次和独立操作事实不被摘要替代。原始步骤、模型结果和真实 usage 保留，无新增数据库表或迁移。旧库中的超长轨迹可在恢复阶段分批压缩，不自动重放工具操作。

压缩增加模型调用耗时与费用。自动压缩无需前端新入口；桌面客户端需更新随附 Go 程序才能使用该能力，本仓库不会替换另一个仓库中的二进制。

## 请求与历史的资源边界

实际模型请求的有效上下文最多 256 条、JSON 不超过 8 MiB；历史可通过摘要持续复用，不以会话累计消息数作为上限。历史消息按完整轮次分页读取，轨迹按完整模型与工具批次读取；压缩准备窗口最多 512 条、16 MiB。自动压缩在模型调用前及工具结果追加后统一检查 Token、消息数量、上下文 JSON 和适配器实际请求字节；超限先压缩可安全替换的来源，再复查全部边界，不能容纳的单个批次明确失败，不静默截断历史。Token 预算与资源安全边界独立计算。
未成功完成的历史回答不作为完整回答传入后续上下文。托管图片从应用数据目录中的副本读取，旧消息的绝对路径仍按原路径读取。

图片输入的文件与实际请求大小约束见 [图片输入](jsonrpc.md#12-图片输入)。
