# 中文代码对照

本目录对应 Callum Go 仓库中的全部代码，按源码相对路径存放，并在原文件名后追加 `.md`。例如 `internal/agent/service/loop.go` 对应 `docs/code-zh/internal/agent/service/loop.go.md`。

当前覆盖 440 个 Go 文件（含 151 个测试文件、全部平台实现及测试辅助程序）、1 个 Shell 脚本和 1 个 HTML 回调页面，共 442 份代码对照、54,964 行原代码。

## 翻译规则

- 每份代码对照只保留一个代码块，按源文件语言标记为 `go`、`sh` 或 `html`；标题、路径说明、翻译规则和导航集中维护在 README 中。原代码含 Markdown 围栏时，外层代码块使用更长的围栏，保留原文内容。
- 保留 Go 关键字、内置类型、内置函数、内置常量和语法符号；`main`、`init` 及测试入口的 `Test`、`Benchmark`、`Example`、`Fuzz` 前缀等语言约定名称保持原样。HTML、CSS、JavaScript、Shell 和 Go 模板的语法、标准命令及固定选项保持原样。
- 变量、业务函数、包名、类型、字段和调用名称按含义翻译成中文。包的导入别名使用“中文名称＋包”，保留原导入路径；`_`、`.` 导入保持原样。JSON、UUID、HTTP、MCP、OAuth、SQLite 等技术缩写按通用名称保留。
- 按具体符号的职责使用一致译名。短接收者及参数名结合声明类型翻译，例如测试参数 `t` 译为“测试器”，引擎接收者 `e` 译为“当前引擎”。同名但职责不同的符号按上下文区分；同一类型中的字段、回调和方法使用可区分的中文名称。
- 保留原字符串、字符、数值、结构标签、导入路径、外部命令和固定数据，不改写日志字段、协议值、SQL、提示词或测试输入。Shell 引号及算术表达式中的变量引用与对应变量同步翻译；HTML 中的自定义 CSS 变量和模板字段与对应声明同步翻译。
- 按原文件顺序对应全部代码，保留声明、作用域、条件分支、参数、返回值、错误处理和资源清理，不省略、合并、重排或改写执行逻辑。保持原文件行数，只在保持代码对应关系的前提下调整中文名称后的空白对齐。
- 原有注释在原位置保留或翻译，不额外添加讲解性注释。`//go:build`、`//go:embed` 等构建与内嵌指令，以及脚本解释器声明保持原样。
- 对照稿用于阅读，中文名称对应的库调用及脚本变量不作为可直接编译或执行的实现；原代码的行为以源码为准。源码变更后需同步相应对照和导航，并重新核对译名、语法、字面量、指令及文件覆盖范围。

## 常用名称

| 原名称 | 中文含义 |
| --- | --- |
| `ctx` | 执行上下文 |
| `err` | 错误 |
| `ex` | 本次执行 |
| `Engine` | 引擎 |
| `Repository` | 仓储 |
| `Runtime` | 运行时 |
| `Run` | 执行 |
| `Step` | 步骤 |
| `Message` | 消息 |
| `Tool` | 工具 |
| `Invocation` | 工具调用 |
| `SettingsSnapshot` | 设置快照 |
| `ApprovalPolicy` | 审批策略 |
| `ApprovalsReviewer` | 审批审核方 |
| `FollowUp` | 后续消息 |
| `Checkpoint` | 检查点 |
| `ContextWindow` | 上下文窗口 |
| `MaxOutputTokens` | 最大输出词元 |
| `APIKey` | API密钥 |

## 文件导航

下表逐项链接原代码与中文对照；分组沿用源码目录。

### cmd/callum

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [cmd/callum/main.go](../../cmd/callum/main.go) | [main.go.md](cmd/callum/main.go.md) | Go |
| [cmd/callum/resource_lock.go](../../cmd/callum/resource_lock.go) | [resource_lock.go.md](cmd/callum/resource_lock.go.md) | Go |
| [cmd/callum/resource_lock_test.go](../../cmd/callum/resource_lock_test.go) | [resource_lock_test.go.md](cmd/callum/resource_lock_test.go.md) | Go 测试 |

### cmd/prepare-runtimes

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [cmd/prepare-runtimes/main.go](../../cmd/prepare-runtimes/main.go) | [main.go.md](cmd/prepare-runtimes/main.go.md) | Go |

### internal/agent

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/agent/approval.go](../../internal/agent/approval.go) | [approval.go.md](internal/agent/approval.go.md) | Go |
| [internal/agent/approval/directory.go](../../internal/agent/approval/directory.go) | [directory.go.md](internal/agent/approval/directory.go.md) | Go |
| [internal/agent/approval/service.go](../../internal/agent/approval/service.go) | [service.go.md](internal/agent/approval/service.go.md) | Go |
| [internal/agent/checkpoint.go](../../internal/agent/checkpoint.go) | [checkpoint.go.md](internal/agent/checkpoint.go.md) | Go |
| [internal/agent/context.go](../../internal/agent/context.go) | [context.go.md](internal/agent/context.go.md) | Go |
| [internal/agent/errors.go](../../internal/agent/errors.go) | [errors.go.md](internal/agent/errors.go.md) | Go |
| [internal/agent/events.go](../../internal/agent/events.go) | [events.go.md](internal/agent/events.go.md) | Go |
| [internal/agent/events/hub.go](../../internal/agent/events/hub.go) | [hub.go.md](internal/agent/events/hub.go.md) | Go |
| [internal/agent/events/hub_test.go](../../internal/agent/events/hub_test.go) | [hub_test.go.md](internal/agent/events/hub_test.go.md) | Go 测试 |
| [internal/agent/executor/manager.go](../../internal/agent/executor/manager.go) | [manager.go.md](internal/agent/executor/manager.go.md) | Go |
| [internal/agent/executor/manager_test.go](../../internal/agent/executor/manager_test.go) | [manager_test.go.md](internal/agent/executor/manager_test.go.md) | Go 测试 |
| [internal/agent/repository.go](../../internal/agent/repository.go) | [repository.go.md](internal/agent/repository.go.md) | Go |
| [internal/agent/service/approval_review.go](../../internal/agent/service/approval_review.go) | [approval_review.go.md](internal/agent/service/approval_review.go.md) | Go |
| [internal/agent/service/approval_review_test.go](../../internal/agent/service/approval_review_test.go) | [approval_review_test.go.md](internal/agent/service/approval_review_test.go.md) | Go 测试 |
| [internal/agent/service/capacity.go](../../internal/agent/service/capacity.go) | [capacity.go.md](internal/agent/service/capacity.go.md) | Go |
| [internal/agent/service/capacity_test.go](../../internal/agent/service/capacity_test.go) | [capacity_test.go.md](internal/agent/service/capacity_test.go.md) | Go 测试 |
| [internal/agent/service/checkpoint.go](../../internal/agent/service/checkpoint.go) | [checkpoint.go.md](internal/agent/service/checkpoint.go.md) | Go |
| [internal/agent/service/compaction.go](../../internal/agent/service/compaction.go) | [compaction.go.md](internal/agent/service/compaction.go.md) | Go |
| [internal/agent/service/compaction_model.go](../../internal/agent/service/compaction_model.go) | [compaction_model.go.md](internal/agent/service/compaction_model.go.md) | Go |
| [internal/agent/service/context.go](../../internal/agent/service/context.go) | [context.go.md](internal/agent/service/context.go.md) | Go |
| [internal/agent/service/context_preview.go](../../internal/agent/service/context_preview.go) | [context_preview.go.md](internal/agent/service/context_preview.go.md) | Go |
| [internal/agent/service/context_test.go](../../internal/agent/service/context_test.go) | [context_test.go.md](internal/agent/service/context_test.go.md) | Go 测试 |
| [internal/agent/service/context_usage.go](../../internal/agent/service/context_usage.go) | [context_usage.go.md](internal/agent/service/context_usage.go.md) | Go |
| [internal/agent/service/correction.go](../../internal/agent/service/correction.go) | [correction.go.md](internal/agent/service/correction.go.md) | Go |
| [internal/agent/service/correction_test.go](../../internal/agent/service/correction_test.go) | [correction_test.go.md](internal/agent/service/correction_test.go.md) | Go 测试 |
| [internal/agent/service/errors.go](../../internal/agent/service/errors.go) | [errors.go.md](internal/agent/service/errors.go.md) | Go |
| [internal/agent/service/execution.go](../../internal/agent/service/execution.go) | [execution.go.md](internal/agent/service/execution.go.md) | Go |
| [internal/agent/service/followup.go](../../internal/agent/service/followup.go) | [followup.go.md](internal/agent/service/followup.go.md) | Go |
| [internal/agent/service/followup_test.go](../../internal/agent/service/followup_test.go) | [followup_test.go.md](internal/agent/service/followup_test.go.md) | Go 测试 |
| [internal/agent/service/loop.go](../../internal/agent/service/loop.go) | [loop.go.md](internal/agent/service/loop.go.md) | Go |
| [internal/agent/service/model.go](../../internal/agent/service/model.go) | [model.go.md](internal/agent/service/model.go.md) | Go |
| [internal/agent/service/model_test.go](../../internal/agent/service/model_test.go) | [model_test.go.md](internal/agent/service/model_test.go.md) | Go 测试 |
| [internal/agent/service/prompt.go](../../internal/agent/service/prompt.go) | [prompt.go.md](internal/agent/service/prompt.go.md) | Go |
| [internal/agent/service/queries.go](../../internal/agent/service/queries.go) | [queries.go.md](internal/agent/service/queries.go.md) | Go |
| [internal/agent/service/request_context.go](../../internal/agent/service/request_context.go) | [request_context.go.md](internal/agent/service/request_context.go.md) | Go |
| [internal/agent/service/request_context_test.go](../../internal/agent/service/request_context_test.go) | [request_context_test.go.md](internal/agent/service/request_context_test.go.md) | Go 测试 |
| [internal/agent/service/service.go](../../internal/agent/service/service.go) | [service.go.md](internal/agent/service/service.go.md) | Go |
| [internal/agent/service/title.go](../../internal/agent/service/title.go) | [title.go.md](internal/agent/service/title.go.md) | Go |
| [internal/agent/service/tools.go](../../internal/agent/service/tools.go) | [tools.go.md](internal/agent/service/tools.go.md) | Go |
| [internal/agent/service/user_input.go](../../internal/agent/service/user_input.go) | [user_input.go.md](internal/agent/service/user_input.go.md) | Go |
| [internal/agent/transcript/recovery.go](../../internal/agent/transcript/recovery.go) | [recovery.go.md](internal/agent/transcript/recovery.go.md) | Go |
| [internal/agent/transcript/recovery_test.go](../../internal/agent/transcript/recovery_test.go) | [recovery_test.go.md](internal/agent/transcript/recovery_test.go.md) | Go 测试 |
| [internal/agent/transcript/restore.go](../../internal/agent/transcript/restore.go) | [restore.go.md](internal/agent/transcript/restore.go.md) | Go |
| [internal/agent/transcript/restore_test.go](../../internal/agent/transcript/restore_test.go) | [restore_test.go.md](internal/agent/transcript/restore_test.go.md) | Go 测试 |
| [internal/agent/transcript/types.go](../../internal/agent/transcript/types.go) | [types.go.md](internal/agent/transcript/types.go.md) | Go |
| [internal/agent/types.go](../../internal/agent/types.go) | [types.go.md](internal/agent/types.go.md) | Go |
| [internal/agent/user_input.go](../../internal/agent/user_input.go) | [user_input.go.md](internal/agent/user_input.go.md) | Go |
| [internal/agent/user_input_test.go](../../internal/agent/user_input_test.go) | [user_input_test.go.md](internal/agent/user_input_test.go.md) | Go 测试 |

### internal/app

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/app/agent_limits_test.go](../../internal/app/agent_limits_test.go) | [agent_limits_test.go.md](internal/app/agent_limits_test.go.md) | Go 测试 |
| [internal/app/agent_test.go](../../internal/app/agent_test.go) | [agent_test.go.md](internal/app/agent_test.go.md) | Go 测试 |
| [internal/app/app.go](../../internal/app/app.go) | [app.go.md](internal/app/app.go.md) | Go |
| [internal/app/app_test.go](../../internal/app/app_test.go) | [app_test.go.md](internal/app/app_test.go.md) | Go 测试 |
| [internal/app/approval_modes_test.go](../../internal/app/approval_modes_test.go) | [approval_modes_test.go.md](internal/app/approval_modes_test.go.md) | Go 测试 |
| [internal/app/approval_policy_test.go](../../internal/app/approval_policy_test.go) | [approval_policy_test.go.md](internal/app/approval_policy_test.go.md) | Go 测试 |
| [internal/app/approval_steering_test.go](../../internal/app/approval_steering_test.go) | [approval_steering_test.go.md](internal/app/approval_steering_test.go.md) | Go 测试 |
| [internal/app/argument_limit_test.go](../../internal/app/argument_limit_test.go) | [argument_limit_test.go.md](internal/app/argument_limit_test.go.md) | Go 测试 |
| [internal/app/attachments_test.go](../../internal/app/attachments_test.go) | [attachments_test.go.md](internal/app/attachments_test.go.md) | Go 测试 |
| [internal/app/command_tools_test.go](../../internal/app/command_tools_test.go) | [command_tools_test.go.md](internal/app/command_tools_test.go.md) | Go 测试 |
| [internal/app/compaction_capacity_test.go](../../internal/app/compaction_capacity_test.go) | [compaction_capacity_test.go.md](internal/app/compaction_capacity_test.go.md) | Go 测试 |
| [internal/app/compaction_test.go](../../internal/app/compaction_test.go) | [compaction_test.go.md](internal/app/compaction_test.go.md) | Go 测试 |
| [internal/app/compaction_tools_test.go](../../internal/app/compaction_tools_test.go) | [compaction_tools_test.go.md](internal/app/compaction_tools_test.go.md) | Go 测试 |
| [internal/app/context_usage_test.go](../../internal/app/context_usage_test.go) | [context_usage_test.go.md](internal/app/context_usage_test.go.md) | Go 测试 |
| [internal/app/conversation_test.go](../../internal/app/conversation_test.go) | [conversation_test.go.md](internal/app/conversation_test.go.md) | Go 测试 |
| [internal/app/conversation_update_test.go](../../internal/app/conversation_update_test.go) | [conversation_update_test.go.md](internal/app/conversation_update_test.go.md) | Go 测试 |
| [internal/app/correction_test.go](../../internal/app/correction_test.go) | [correction_test.go.md](internal/app/correction_test.go.md) | Go 测试 |
| [internal/app/directory_approval_test.go](../../internal/app/directory_approval_test.go) | [directory_approval_test.go.md](internal/app/directory_approval_test.go.md) | Go 测试 |
| [internal/app/file_tools_test.go](../../internal/app/file_tools_test.go) | [file_tools_test.go.md](internal/app/file_tools_test.go.md) | Go 测试 |
| [internal/app/followup_test.go](../../internal/app/followup_test.go) | [followup_test.go.md](internal/app/followup_test.go.md) | Go 测试 |
| [internal/app/general_settings_test.go](../../internal/app/general_settings_test.go) | [general_settings_test.go.md](internal/app/general_settings_test.go.md) | Go 测试 |
| [internal/app/httpclient.go](../../internal/app/httpclient.go) | [httpclient.go.md](internal/app/httpclient.go.md) | Go |
| [internal/app/httpclient_test.go](../../internal/app/httpclient_test.go) | [httpclient_test.go.md](internal/app/httpclient_test.go.md) | Go 测试 |
| [internal/app/input_validation_test.go](../../internal/app/input_validation_test.go) | [input_validation_test.go.md](internal/app/input_validation_test.go.md) | Go 测试 |
| [internal/app/mcp_test.go](../../internal/app/mcp_test.go) | [mcp_test.go.md](internal/app/mcp_test.go.md) | Go 测试 |
| [internal/app/models.go](../../internal/app/models.go) | [models.go.md](internal/app/models.go.md) | Go |
| [internal/app/models_test.go](../../internal/app/models_test.go) | [models_test.go.md](internal/app/models_test.go.md) | Go 测试 |
| [internal/app/patch_tools_test.go](../../internal/app/patch_tools_test.go) | [patch_tools_test.go.md](internal/app/patch_tools_test.go.md) | Go 测试 |
| [internal/app/runtime_test.go](../../internal/app/runtime_test.go) | [runtime_test.go.md](internal/app/runtime_test.go.md) | Go 测试 |
| [internal/app/session.go](../../internal/app/session.go) | [session.go.md](internal/app/session.go.md) | Go |
| [internal/app/settings_test.go](../../internal/app/settings_test.go) | [settings_test.go.md](internal/app/settings_test.go.md) | Go 测试 |
| [internal/app/skill_references_test.go](../../internal/app/skill_references_test.go) | [skill_references_test.go.md](internal/app/skill_references_test.go.md) | Go 测试 |
| [internal/app/skills_test.go](../../internal/app/skills_test.go) | [skills_test.go.md](internal/app/skills_test.go.md) | Go 测试 |
| [internal/app/stdio.go](../../internal/app/stdio.go) | [stdio.go.md](internal/app/stdio.go.md) | Go |
| [internal/app/step_data_test.go](../../internal/app/step_data_test.go) | [step_data_test.go.md](internal/app/step_data_test.go.md) | Go 测试 |
| [internal/app/title_test.go](../../internal/app/title_test.go) | [title_test.go.md](internal/app/title_test.go.md) | Go 测试 |
| [internal/app/token_budget_test.go](../../internal/app/token_budget_test.go) | [token_budget_test.go.md](internal/app/token_budget_test.go.md) | Go 测试 |
| [internal/app/tool_outcomes_test.go](../../internal/app/tool_outcomes_test.go) | [tool_outcomes_test.go.md](internal/app/tool_outcomes_test.go.md) | Go 测试 |
| [internal/app/transport.go](../../internal/app/transport.go) | [transport.go.md](internal/app/transport.go.md) | Go |
| [internal/app/user_input_test.go](../../internal/app/user_input_test.go) | [user_input_test.go.md](internal/app/user_input_test.go.md) | Go 测试 |
| [internal/app/workspace_test.go](../../internal/app/workspace_test.go) | [workspace_test.go.md](internal/app/workspace_test.go.md) | Go 测试 |

### internal/attachment

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/attachment/access.go](../../internal/attachment/access.go) | [access.go.md](internal/attachment/access.go.md) | Go |
| [internal/attachment/access_test.go](../../internal/attachment/access_test.go) | [access_test.go.md](internal/attachment/access_test.go.md) | Go 测试 |
| [internal/attachment/concurrency_test.go](../../internal/attachment/concurrency_test.go) | [concurrency_test.go.md](internal/attachment/concurrency_test.go.md) | Go 测试 |
| [internal/attachment/file.go](../../internal/attachment/file.go) | [file.go.md](internal/attachment/file.go.md) | Go |
| [internal/attachment/generated.go](../../internal/attachment/generated.go) | [generated.go.md](internal/attachment/generated.go.md) | Go |
| [internal/attachment/io_other.go](../../internal/attachment/io_other.go) | [io_other.go.md](internal/attachment/io_other.go.md) | Go |
| [internal/attachment/io_unix.go](../../internal/attachment/io_unix.go) | [io_unix.go.md](internal/attachment/io_unix.go.md) | Go |
| [internal/attachment/store.go](../../internal/attachment/store.go) | [store.go.md](internal/attachment/store.go.md) | Go |

### internal/config

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/config/config.go](../../internal/config/config.go) | [config.go.md](internal/config/config.go.md) | Go |
| [internal/config/config_test.go](../../internal/config/config_test.go) | [config_test.go.md](internal/config/config_test.go.md) | Go 测试 |
| [internal/config/flags.go](../../internal/config/flags.go) | [flags.go.md](internal/config/flags.go.md) | Go |
| [internal/config/flags_test.go](../../internal/config/flags_test.go) | [flags_test.go.md](internal/config/flags_test.go.md) | Go 测试 |
| [internal/config/mode_dev_test.go](../../internal/config/mode_dev_test.go) | [mode_dev_test.go.md](internal/config/mode_dev_test.go.md) | Go 测试 |
| [internal/config/mode_release_test.go](../../internal/config/mode_release_test.go) | [mode_release_test.go.md](internal/config/mode_release_test.go.md) | Go 测试 |
| [internal/config/paths/dev.go](../../internal/config/paths/dev.go) | [dev.go.md](internal/config/paths/dev.go.md) | Go |
| [internal/config/paths/paths.go](../../internal/config/paths/paths.go) | [paths.go.md](internal/config/paths/paths.go.md) | Go |
| [internal/config/paths/release.go](../../internal/config/paths/release.go) | [release.go.md](internal/config/paths/release.go.md) | Go |

### internal/conversation

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/conversation/attachment.go](../../internal/conversation/attachment.go) | [attachment.go.md](internal/conversation/attachment.go.md) | Go |
| [internal/conversation/compaction/plan.go](../../internal/conversation/compaction/plan.go) | [plan.go.md](internal/conversation/compaction/plan.go.md) | Go |
| [internal/conversation/compaction/plan_test.go](../../internal/conversation/compaction/plan_test.go) | [plan_test.go.md](internal/conversation/compaction/plan_test.go.md) | Go 测试 |
| [internal/conversation/compaction/summary.go](../../internal/conversation/compaction/summary.go) | [summary.go.md](internal/conversation/compaction/summary.go.md) | Go |
| [internal/conversation/context.go](../../internal/conversation/context.go) | [context.go.md](internal/conversation/context.go.md) | Go |
| [internal/conversation/errors.go](../../internal/conversation/errors.go) | [errors.go.md](internal/conversation/errors.go.md) | Go |
| [internal/conversation/followup.go](../../internal/conversation/followup.go) | [followup.go.md](internal/conversation/followup.go.md) | Go |
| [internal/conversation/history/context.go](../../internal/conversation/history/context.go) | [context.go.md](internal/conversation/history/context.go.md) | Go |
| [internal/conversation/history/context_test.go](../../internal/conversation/history/context_test.go) | [context_test.go.md](internal/conversation/history/context_test.go.md) | Go 测试 |
| [internal/conversation/history/history.go](../../internal/conversation/history/history.go) | [history.go.md](internal/conversation/history/history.go.md) | Go |
| [internal/conversation/repository.go](../../internal/conversation/repository.go) | [repository.go.md](internal/conversation/repository.go.md) | Go |
| [internal/conversation/service/attachment.go](../../internal/conversation/service/attachment.go) | [attachment.go.md](internal/conversation/service/attachment.go.md) | Go |
| [internal/conversation/service/compaction.go](../../internal/conversation/service/compaction.go) | [compaction.go.md](internal/conversation/service/compaction.go.md) | Go |
| [internal/conversation/service/context_usage.go](../../internal/conversation/service/context_usage.go) | [context_usage.go.md](internal/conversation/service/context_usage.go.md) | Go |
| [internal/conversation/service/execution.go](../../internal/conversation/service/execution.go) | [execution.go.md](internal/conversation/service/execution.go.md) | Go |
| [internal/conversation/service/followup.go](../../internal/conversation/service/followup.go) | [followup.go.md](internal/conversation/service/followup.go.md) | Go |
| [internal/conversation/service/service.go](../../internal/conversation/service/service.go) | [service.go.md](internal/conversation/service/service.go.md) | Go |
| [internal/conversation/service/skills.go](../../internal/conversation/service/skills.go) | [skills.go.md](internal/conversation/service/skills.go.md) | Go |
| [internal/conversation/types.go](../../internal/conversation/types.go) | [types.go.md](internal/conversation/types.go.md) | Go |
| [internal/conversation/usage.go](../../internal/conversation/usage.go) | [usage.go.md](internal/conversation/usage.go.md) | Go |

### internal/credentials

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/credentials/credentialstest/keyring.go](../../internal/credentials/credentialstest/keyring.go) | [keyring.go.md](internal/credentials/credentialstest/keyring.go.md) | Go |
| [internal/credentials/system/store_darwin.go](../../internal/credentials/system/store_darwin.go) | [store_darwin.go.md](internal/credentials/system/store_darwin.go.md) | Go |
| [internal/credentials/system/store_darwin_test.go](../../internal/credentials/system/store_darwin_test.go) | [store_darwin_test.go.md](internal/credentials/system/store_darwin_test.go.md) | Go 测试 |
| [internal/credentials/system/store_linux.go](../../internal/credentials/system/store_linux.go) | [store_linux.go.md](internal/credentials/system/store_linux.go.md) | Go |
| [internal/credentials/system/store_other.go](../../internal/credentials/system/store_other.go) | [store_other.go.md](internal/credentials/system/store_other.go.md) | Go |
| [internal/credentials/system/store_windows.go](../../internal/credentials/system/store_windows.go) | [store_windows.go.md](internal/credentials/system/store_windows.go.md) | Go |
| [internal/credentials/system/system.go](../../internal/credentials/system/system.go) | [system.go.md](internal/credentials/system/system.go.md) | Go |
| [internal/credentials/vault.go](../../internal/credentials/vault.go) | [vault.go.md](internal/credentials/vault.go.md) | Go |
| [internal/credentials/vault_test.go](../../internal/credentials/vault_test.go) | [vault_test.go.md](internal/credentials/vault_test.go.md) | Go 测试 |

### internal/filelock

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/filelock/lock.go](../../internal/filelock/lock.go) | [lock.go.md](internal/filelock/lock.go.md) | Go |
| [internal/filelock/lock_other.go](../../internal/filelock/lock_other.go) | [lock_other.go.md](internal/filelock/lock_other.go.md) | Go |
| [internal/filelock/lock_test.go](../../internal/filelock/lock_test.go) | [lock_test.go.md](internal/filelock/lock_test.go.md) | Go 测试 |
| [internal/filelock/lock_unix.go](../../internal/filelock/lock_unix.go) | [lock_unix.go.md](internal/filelock/lock_unix.go.md) | Go |
| [internal/filelock/lock_windows.go](../../internal/filelock/lock_windows.go) | [lock_windows.go.md](internal/filelock/lock_windows.go.md) | Go |

### internal/id

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/id/id.go](../../internal/id/id.go) | [id.go.md](internal/id/id.go.md) | Go |
| [internal/id/id_test.go](../../internal/id/id_test.go) | [id_test.go.md](internal/id/id_test.go.md) | Go 测试 |

### internal/inference

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/inference/errors.go](../../internal/inference/errors.go) | [errors.go.md](internal/inference/errors.go.md) | Go |
| [internal/inference/executor/execution.go](../../internal/inference/executor/execution.go) | [execution.go.md](internal/inference/executor/execution.go.md) | Go |
| [internal/inference/executor/manager.go](../../internal/inference/executor/manager.go) | [manager.go.md](internal/inference/executor/manager.go.md) | Go |
| [internal/inference/executor/manager_test.go](../../internal/inference/executor/manager_test.go) | [manager_test.go.md](internal/inference/executor/manager_test.go.md) | Go 测试 |
| [internal/inference/notification.go](../../internal/inference/notification.go) | [notification.go.md](internal/inference/notification.go.md) | Go |
| [internal/inference/notification_test.go](../../internal/inference/notification_test.go) | [notification_test.go.md](internal/inference/notification_test.go.md) | Go 测试 |
| [internal/inference/types.go](../../internal/inference/types.go) | [types.go.md](internal/inference/types.go.md) | Go |

### internal/mcp

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/mcp/close_test.go](../../internal/mcp/close_test.go) | [close_test.go.md](internal/mcp/close_test.go.md) | Go 测试 |
| [internal/mcp/config_snapshot_test.go](../../internal/mcp/config_snapshot_test.go) | [config_snapshot_test.go.md](internal/mcp/config_snapshot_test.go.md) | Go 测试 |
| [internal/mcp/oauth.go](../../internal/mcp/oauth.go) | [oauth.go.md](internal/mcp/oauth.go.md) | Go |
| [internal/mcp/oauth_callback.go](../../internal/mcp/oauth_callback.go) | [oauth_callback.go.md](internal/mcp/oauth_callback.go.md) | Go |
| [internal/mcp/oauth_callback.html](../../internal/mcp/oauth_callback.html) | [oauth_callback.html.md](internal/mcp/oauth_callback.html.md) | HTML |
| [internal/mcp/process_other.go](../../internal/mcp/process_other.go) | [process_other.go.md](internal/mcp/process_other.go.md) | Go |
| [internal/mcp/process_unix.go](../../internal/mcp/process_unix.go) | [process_unix.go.md](internal/mcp/process_unix.go.md) | Go |
| [internal/mcp/process_unix_test.go](../../internal/mcp/process_unix_test.go) | [process_unix_test.go.md](internal/mcp/process_unix_test.go.md) | Go 测试 |
| [internal/mcp/process_windows.go](../../internal/mcp/process_windows.go) | [process_windows.go.md](internal/mcp/process_windows.go.md) | Go |
| [internal/mcp/service.go](../../internal/mcp/service.go) | [service.go.md](internal/mcp/service.go.md) | Go |
| [internal/mcp/service_test.go](../../internal/mcp/service_test.go) | [service_test.go.md](internal/mcp/service_test.go.md) | Go 测试 |
| [internal/mcp/tools.go](../../internal/mcp/tools.go) | [tools.go.md](internal/mcp/tools.go.md) | Go |
| [internal/mcp/tools_test.go](../../internal/mcp/tools_test.go) | [tools_test.go.md](internal/mcp/tools_test.go.md) | Go 测试 |
| [internal/mcp/transport.go](../../internal/mcp/transport.go) | [transport.go.md](internal/mcp/transport.go.md) | Go |
| [internal/mcp/transport_test.go](../../internal/mcp/transport_test.go) | [transport_test.go.md](internal/mcp/transport_test.go.md) | Go 测试 |

### internal/model

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/model/adapter/chatcompletions/capture.go](../../internal/model/adapter/chatcompletions/capture.go) | [capture.go.md](internal/model/adapter/chatcompletions/capture.go.md) | Go |
| [internal/model/adapter/chatcompletions/capture_test.go](../../internal/model/adapter/chatcompletions/capture_test.go) | [capture_test.go.md](internal/model/adapter/chatcompletions/capture_test.go.md) | Go 测试 |
| [internal/model/adapter/chatcompletions/client.go](../../internal/model/adapter/chatcompletions/client.go) | [client.go.md](internal/model/adapter/chatcompletions/client.go.md) | Go |
| [internal/model/adapter/chatcompletions/client_test.go](../../internal/model/adapter/chatcompletions/client_test.go) | [client_test.go.md](internal/model/adapter/chatcompletions/client_test.go.md) | Go 测试 |
| [internal/model/adapter/chatcompletions/error.go](../../internal/model/adapter/chatcompletions/error.go) | [error.go.md](internal/model/adapter/chatcompletions/error.go.md) | Go |
| [internal/model/adapter/chatcompletions/error_test.go](../../internal/model/adapter/chatcompletions/error_test.go) | [error_test.go.md](internal/model/adapter/chatcompletions/error_test.go.md) | Go 测试 |
| [internal/model/adapter/chatcompletions/image.go](../../internal/model/adapter/chatcompletions/image.go) | [image.go.md](internal/model/adapter/chatcompletions/image.go.md) | Go |
| [internal/model/adapter/chatcompletions/image_test.go](../../internal/model/adapter/chatcompletions/image_test.go) | [image_test.go.md](internal/model/adapter/chatcompletions/image_test.go.md) | Go 测试 |
| [internal/model/adapter/chatcompletions/input_test.go](../../internal/model/adapter/chatcompletions/input_test.go) | [input_test.go.md](internal/model/adapter/chatcompletions/input_test.go.md) | Go 测试 |
| [internal/model/adapter/chatcompletions/reasoning_test.go](../../internal/model/adapter/chatcompletions/reasoning_test.go) | [reasoning_test.go.md](internal/model/adapter/chatcompletions/reasoning_test.go.md) | Go 测试 |
| [internal/model/adapter/chatcompletions/request.go](../../internal/model/adapter/chatcompletions/request.go) | [request.go.md](internal/model/adapter/chatcompletions/request.go.md) | Go |
| [internal/model/adapter/chatcompletions/response.go](../../internal/model/adapter/chatcompletions/response.go) | [response.go.md](internal/model/adapter/chatcompletions/response.go.md) | Go |
| [internal/model/adapter/chatcompletions/stream.go](../../internal/model/adapter/chatcompletions/stream.go) | [stream.go.md](internal/model/adapter/chatcompletions/stream.go.md) | Go |
| [internal/model/adapter/chatcompletions/stream_test.go](../../internal/model/adapter/chatcompletions/stream_test.go) | [stream_test.go.md](internal/model/adapter/chatcompletions/stream_test.go.md) | Go 测试 |
| [internal/model/adapter/chatcompletions/tools.go](../../internal/model/adapter/chatcompletions/tools.go) | [tools.go.md](internal/model/adapter/chatcompletions/tools.go.md) | Go |
| [internal/model/adapter/chatcompletions/tools_test.go](../../internal/model/adapter/chatcompletions/tools_test.go) | [tools_test.go.md](internal/model/adapter/chatcompletions/tools_test.go.md) | Go 测试 |
| [internal/model/attachment.go](../../internal/model/attachment.go) | [attachment.go.md](internal/model/attachment.go.md) | Go |
| [internal/model/client.go](../../internal/model/client.go) | [client.go.md](internal/model/client.go.md) | Go |
| [internal/model/content.go](../../internal/model/content.go) | [content.go.md](internal/model/content.go.md) | Go |
| [internal/model/error.go](../../internal/model/error.go) | [error.go.md](internal/model/error.go.md) | Go |
| [internal/model/event.go](../../internal/model/event.go) | [event.go.md](internal/model/event.go.md) | Go |
| [internal/model/imageinput/open_other.go](../../internal/model/imageinput/open_other.go) | [open_other.go.md](internal/model/imageinput/open_other.go.md) | Go |
| [internal/model/imageinput/open_unix.go](../../internal/model/imageinput/open_unix.go) | [open_unix.go.md](internal/model/imageinput/open_unix.go.md) | Go |
| [internal/model/imageinput/prepare.go](../../internal/model/imageinput/prepare.go) | [prepare.go.md](internal/model/imageinput/prepare.go.md) | Go |
| [internal/model/imageinput/prepare_test.go](../../internal/model/imageinput/prepare_test.go) | [prepare_test.go.md](internal/model/imageinput/prepare_test.go.md) | Go 测试 |
| [internal/model/imageinput/read.go](../../internal/model/imageinput/read.go) | [read.go.md](internal/model/imageinput/read.go.md) | Go |
| [internal/model/imageinput/read_test.go](../../internal/model/imageinput/read_test.go) | [read_test.go.md](internal/model/imageinput/read_test.go.md) | Go 测试 |
| [internal/model/imageinput/read_unix_test.go](../../internal/model/imageinput/read_unix_test.go) | [read_unix_test.go.md](internal/model/imageinput/read_unix_test.go.md) | Go 测试 |
| [internal/model/input.go](../../internal/model/input.go) | [input.go.md](internal/model/input.go.md) | Go |
| [internal/model/message.go](../../internal/model/message.go) | [message.go.md](internal/model/message.go.md) | Go |
| [internal/model/provider/errors.go](../../internal/model/provider/errors.go) | [errors.go.md](internal/model/provider/errors.go.md) | Go |
| [internal/model/provider/id.go](../../internal/model/provider/id.go) | [id.go.md](internal/model/provider/id.go.md) | Go |
| [internal/model/provider/repository.go](../../internal/model/provider/repository.go) | [repository.go.md](internal/model/provider/repository.go.md) | Go |
| [internal/model/provider/service/id.go](../../internal/model/provider/service/id.go) | [id.go.md](internal/model/provider/service/id.go.md) | Go |
| [internal/model/provider/service/models.go](../../internal/model/provider/service/models.go) | [models.go.md](internal/model/provider/service/models.go.md) | Go |
| [internal/model/provider/service/providers.go](../../internal/model/provider/service/providers.go) | [providers.go.md](internal/model/provider/service/providers.go.md) | Go |
| [internal/model/provider/service/service.go](../../internal/model/provider/service/service.go) | [service.go.md](internal/model/provider/service/service.go.md) | Go |
| [internal/model/provider/service/service_test.go](../../internal/model/provider/service/service_test.go) | [service_test.go.md](internal/model/provider/service/service_test.go.md) | Go 测试 |
| [internal/model/provider/service/validate.go](../../internal/model/provider/service/validate.go) | [validate.go.md](internal/model/provider/service/validate.go.md) | Go |
| [internal/model/provider/service/validate_test.go](../../internal/model/provider/service/validate_test.go) | [validate_test.go.md](internal/model/provider/service/validate_test.go.md) | Go 测试 |
| [internal/model/provider/types.go](../../internal/model/provider/types.go) | [types.go.md](internal/model/provider/types.go.md) | Go |
| [internal/model/request.go](../../internal/model/request.go) | [request.go.md](internal/model/request.go.md) | Go |
| [internal/model/response.go](../../internal/model/response.go) | [response.go.md](internal/model/response.go.md) | Go |
| [internal/model/tokens/estimate.go](../../internal/model/tokens/estimate.go) | [estimate.go.md](internal/model/tokens/estimate.go.md) | Go |
| [internal/model/tokens/estimate_test.go](../../internal/model/tokens/estimate_test.go) | [estimate_test.go.md](internal/model/tokens/estimate_test.go.md) | Go 测试 |
| [internal/model/tokens/meter.go](../../internal/model/tokens/meter.go) | [meter.go.md](internal/model/tokens/meter.go.md) | Go |
| [internal/model/tool.go](../../internal/model/tool.go) | [tool.go.md](internal/model/tool.go.md) | Go |

### internal/processgroup

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/processgroup/stop_unix.go](../../internal/processgroup/stop_unix.go) | [stop_unix.go.md](internal/processgroup/stop_unix.go.md) | Go |
| [internal/processgroup/stop_unix_test.go](../../internal/processgroup/stop_unix_test.go) | [stop_unix_test.go.md](internal/processgroup/stop_unix_test.go.md) | Go 测试 |

### internal/runtimeassets

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/runtimeassets/bundle.go](../../internal/runtimeassets/bundle.go) | [bundle.go.md](internal/runtimeassets/bundle.go.md) | Go |
| [internal/runtimeassets/bundle_test.go](../../internal/runtimeassets/bundle_test.go) | [bundle_test.go.md](internal/runtimeassets/bundle_test.go.md) | Go 测试 |
| [internal/runtimeassets/manifest.go](../../internal/runtimeassets/manifest.go) | [manifest.go.md](internal/runtimeassets/manifest.go.md) | Go |
| [internal/runtimeassets/prepare/archive.go](../../internal/runtimeassets/prepare/archive.go) | [archive.go.md](internal/runtimeassets/prepare/archive.go.md) | Go |
| [internal/runtimeassets/prepare/archive_test.go](../../internal/runtimeassets/prepare/archive_test.go) | [archive_test.go.md](internal/runtimeassets/prepare/archive_test.go.md) | Go 测试 |
| [internal/runtimeassets/prepare/command.go](../../internal/runtimeassets/prepare/command.go) | [command.go.md](internal/runtimeassets/prepare/command.go.md) | Go |
| [internal/runtimeassets/prepare/command_test.go](../../internal/runtimeassets/prepare/command_test.go) | [command_test.go.md](internal/runtimeassets/prepare/command_test.go.md) | Go 测试 |
| [internal/runtimeassets/prepare/lock_test.go](../../internal/runtimeassets/prepare/lock_test.go) | [lock_test.go.md](internal/runtimeassets/prepare/lock_test.go.md) | Go 测试 |
| [internal/runtimeassets/prepare/packages.go](../../internal/runtimeassets/prepare/packages.go) | [packages.go.md](internal/runtimeassets/prepare/packages.go.md) | Go |
| [internal/runtimeassets/prepare/packages_test.go](../../internal/runtimeassets/prepare/packages_test.go) | [packages_test.go.md](internal/runtimeassets/prepare/packages_test.go.md) | Go 测试 |
| [internal/runtimeassets/prepare/prepare.go](../../internal/runtimeassets/prepare/prepare.go) | [prepare.go.md](internal/runtimeassets/prepare/prepare.go.md) | Go |
| [internal/runtimeassets/prepare/workspace.go](../../internal/runtimeassets/prepare/workspace.go) | [workspace.go.md](internal/runtimeassets/prepare/workspace.go.md) | Go |

### internal/settings

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/settings/compaction.go](../../internal/settings/compaction.go) | [compaction.go.md](internal/settings/compaction.go.md) | Go |
| [internal/settings/compaction_snapshot.go](../../internal/settings/compaction_snapshot.go) | [compaction_snapshot.go.md](internal/settings/compaction_snapshot.go.md) | Go |
| [internal/settings/compaction_test.go](../../internal/settings/compaction_test.go) | [compaction_test.go.md](internal/settings/compaction_test.go.md) | Go 测试 |
| [internal/settings/general.go](../../internal/settings/general.go) | [general.go.md](internal/settings/general.go.md) | Go |
| [internal/settings/mcp.go](../../internal/settings/mcp.go) | [mcp.go.md](internal/settings/mcp.go.md) | Go |
| [internal/settings/service/service.go](../../internal/settings/service/service.go) | [service.go.md](internal/settings/service/service.go.md) | Go |
| [internal/settings/types.go](../../internal/settings/types.go) | [types.go.md](internal/settings/types.go.md) | Go |
| [internal/settings/types_test.go](../../internal/settings/types_test.go) | [types_test.go.md](internal/settings/types_test.go.md) | Go 测试 |

### internal/skills

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/skills/read_unix.go](../../internal/skills/read_unix.go) | [read_unix.go.md](internal/skills/read_unix.go.md) | Go |
| [internal/skills/read_windows.go](../../internal/skills/read_windows.go) | [read_windows.go.md](internal/skills/read_windows.go.md) | Go |
| [internal/skills/skills.go](../../internal/skills/skills.go) | [skills.go.md](internal/skills/skills.go.md) | Go |
| [internal/skills/skills_test.go](../../internal/skills/skills_test.go) | [skills_test.go.md](internal/skills/skills_test.go.md) | Go 测试 |

### internal/stdio

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/stdio/approval/handler.go](../../internal/stdio/approval/handler.go) | [handler.go.md](internal/stdio/approval/handler.go.md) | Go |
| [internal/stdio/catalog/dto.go](../../internal/stdio/catalog/dto.go) | [dto.go.md](internal/stdio/catalog/dto.go.md) | Go |
| [internal/stdio/catalog/errors.go](../../internal/stdio/catalog/errors.go) | [errors.go.md](internal/stdio/catalog/errors.go.md) | Go |
| [internal/stdio/catalog/handler.go](../../internal/stdio/catalog/handler.go) | [handler.go.md](internal/stdio/catalog/handler.go.md) | Go |
| [internal/stdio/catalog/pagination_test.go](../../internal/stdio/catalog/pagination_test.go) | [pagination_test.go.md](internal/stdio/catalog/pagination_test.go.md) | Go 测试 |
| [internal/stdio/conversation/compaction.go](../../internal/stdio/conversation/compaction.go) | [compaction.go.md](internal/stdio/conversation/compaction.go.md) | Go |
| [internal/stdio/conversation/errors.go](../../internal/stdio/conversation/errors.go) | [errors.go.md](internal/stdio/conversation/errors.go.md) | Go |
| [internal/stdio/conversation/errors_test.go](../../internal/stdio/conversation/errors_test.go) | [errors_test.go.md](internal/stdio/conversation/errors_test.go.md) | Go 测试 |
| [internal/stdio/conversation/handler.go](../../internal/stdio/conversation/handler.go) | [handler.go.md](internal/stdio/conversation/handler.go.md) | Go |
| [internal/stdio/execution/dto.go](../../internal/stdio/execution/dto.go) | [dto.go.md](internal/stdio/execution/dto.go.md) | Go |
| [internal/stdio/execution/errors.go](../../internal/stdio/execution/errors.go) | [errors.go.md](internal/stdio/execution/errors.go.md) | Go |
| [internal/stdio/execution/events.go](../../internal/stdio/execution/events.go) | [events.go.md](internal/stdio/execution/events.go.md) | Go |
| [internal/stdio/execution/handler.go](../../internal/stdio/execution/handler.go) | [handler.go.md](internal/stdio/execution/handler.go.md) | Go |
| [internal/stdio/execution/persistent_events.go](../../internal/stdio/execution/persistent_events.go) | [persistent_events.go.md](internal/stdio/execution/persistent_events.go.md) | Go |
| [internal/stdio/execution/persistent_events_test.go](../../internal/stdio/execution/persistent_events_test.go) | [persistent_events_test.go.md](internal/stdio/execution/persistent_events_test.go.md) | Go 测试 |
| [internal/stdio/execution/queries.go](../../internal/stdio/execution/queries.go) | [queries.go.md](internal/stdio/execution/queries.go.md) | Go |
| [internal/stdio/mcp/handler.go](../../internal/stdio/mcp/handler.go) | [handler.go.md](internal/stdio/mcp/handler.go.md) | Go |
| [internal/stdio/protocol/errors.go](../../internal/stdio/protocol/errors.go) | [errors.go.md](internal/stdio/protocol/errors.go.md) | Go |
| [internal/stdio/protocol/request.go](../../internal/stdio/protocol/request.go) | [request.go.md](internal/stdio/protocol/request.go.md) | Go |
| [internal/stdio/protocol/response.go](../../internal/stdio/protocol/response.go) | [response.go.md](internal/stdio/protocol/response.go.md) | Go |
| [internal/stdio/repository_test.go](../../internal/stdio/repository_test.go) | [repository_test.go.md](internal/stdio/repository_test.go.md) | Go 测试 |
| [internal/stdio/router.go](../../internal/stdio/router.go) | [router.go.md](internal/stdio/router.go.md) | Go |
| [internal/stdio/server.go](../../internal/stdio/server.go) | [server.go.md](internal/stdio/server.go.md) | Go |
| [internal/stdio/server_test.go](../../internal/stdio/server_test.go) | [server_test.go.md](internal/stdio/server_test.go.md) | Go 测试 |
| [internal/stdio/settings/handler.go](../../internal/stdio/settings/handler.go) | [handler.go.md](internal/stdio/settings/handler.go.md) | Go |
| [internal/stdio/skills/handler.go](../../internal/stdio/skills/handler.go) | [handler.go.md](internal/stdio/skills/handler.go.md) | Go |
| [internal/stdio/transport/connection.go](../../internal/stdio/transport/connection.go) | [connection.go.md](internal/stdio/transport/connection.go.md) | Go |
| [internal/stdio/transport/connection_test.go](../../internal/stdio/transport/connection_test.go) | [connection_test.go.md](internal/stdio/transport/connection_test.go.md) | Go 测试 |
| [internal/stdio/transport/streams_other.go](../../internal/stdio/transport/streams_other.go) | [streams_other.go.md](internal/stdio/transport/streams_other.go.md) | Go |
| [internal/stdio/transport/streams_unix.go](../../internal/stdio/transport/streams_unix.go) | [streams_unix.go.md](internal/stdio/transport/streams_unix.go.md) | Go |
| [internal/stdio/transport/streams_unix_test.go](../../internal/stdio/transport/streams_unix_test.go) | [streams_unix_test.go.md](internal/stdio/transport/streams_unix_test.go.md) | Go 测试 |
| [internal/stdio/userinput/handler.go](../../internal/stdio/userinput/handler.go) | [handler.go.md](internal/stdio/userinput/handler.go.md) | Go |
| [internal/stdio/workspace/errors.go](../../internal/stdio/workspace/errors.go) | [errors.go.md](internal/stdio/workspace/errors.go.md) | Go |
| [internal/stdio/workspace/handler.go](../../internal/stdio/workspace/handler.go) | [handler.go.md](internal/stdio/workspace/handler.go.md) | Go |

### internal/storage

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/storage/sqlite/agent/approvals.go](../../internal/storage/sqlite/agent/approvals.go) | [approvals.go.md](internal/storage/sqlite/agent/approvals.go.md) | Go |
| [internal/storage/sqlite/agent/approvals_test.go](../../internal/storage/sqlite/agent/approvals_test.go) | [approvals_test.go.md](internal/storage/sqlite/agent/approvals_test.go.md) | Go 测试 |
| [internal/storage/sqlite/agent/artifacts.go](../../internal/storage/sqlite/agent/artifacts.go) | [artifacts.go.md](internal/storage/sqlite/agent/artifacts.go.md) | Go |
| [internal/storage/sqlite/agent/artifacts_test.go](../../internal/storage/sqlite/agent/artifacts_test.go) | [artifacts_test.go.md](internal/storage/sqlite/agent/artifacts_test.go.md) | Go 测试 |
| [internal/storage/sqlite/agent/checkpoint.go](../../internal/storage/sqlite/agent/checkpoint.go) | [checkpoint.go.md](internal/storage/sqlite/agent/checkpoint.go.md) | Go |
| [internal/storage/sqlite/agent/checkpoint_test.go](../../internal/storage/sqlite/agent/checkpoint_test.go) | [checkpoint_test.go.md](internal/storage/sqlite/agent/checkpoint_test.go.md) | Go 测试 |
| [internal/storage/sqlite/agent/context.go](../../internal/storage/sqlite/agent/context.go) | [context.go.md](internal/storage/sqlite/agent/context.go.md) | Go |
| [internal/storage/sqlite/agent/context_test.go](../../internal/storage/sqlite/agent/context_test.go) | [context_test.go.md](internal/storage/sqlite/agent/context_test.go.md) | Go 测试 |
| [internal/storage/sqlite/agent/directory.go](../../internal/storage/sqlite/agent/directory.go) | [directory.go.md](internal/storage/sqlite/agent/directory.go.md) | Go |
| [internal/storage/sqlite/agent/directory_test.go](../../internal/storage/sqlite/agent/directory_test.go) | [directory_test.go.md](internal/storage/sqlite/agent/directory_test.go.md) | Go 测试 |
| [internal/storage/sqlite/agent/mapping.go](../../internal/storage/sqlite/agent/mapping.go) | [mapping.go.md](internal/storage/sqlite/agent/mapping.go.md) | Go |
| [internal/storage/sqlite/agent/repository.go](../../internal/storage/sqlite/agent/repository.go) | [repository.go.md](internal/storage/sqlite/agent/repository.go.md) | Go |
| [internal/storage/sqlite/agent/repository_test.go](../../internal/storage/sqlite/agent/repository_test.go) | [repository_test.go.md](internal/storage/sqlite/agent/repository_test.go.md) | Go 测试 |
| [internal/storage/sqlite/agent/transcript.go](../../internal/storage/sqlite/agent/transcript.go) | [transcript.go.md](internal/storage/sqlite/agent/transcript.go.md) | Go |
| [internal/storage/sqlite/agent/transcript_test.go](../../internal/storage/sqlite/agent/transcript_test.go) | [transcript_test.go.md](internal/storage/sqlite/agent/transcript_test.go.md) | Go 测试 |
| [internal/storage/sqlite/agent/user_input.go](../../internal/storage/sqlite/agent/user_input.go) | [user_input.go.md](internal/storage/sqlite/agent/user_input.go.md) | Go |
| [internal/storage/sqlite/agent/user_input_test.go](../../internal/storage/sqlite/agent/user_input_test.go) | [user_input_test.go.md](internal/storage/sqlite/agent/user_input_test.go.md) | Go 测试 |
| [internal/storage/sqlite/catalog/catalog_test.go](../../internal/storage/sqlite/catalog/catalog_test.go) | [catalog_test.go.md](internal/storage/sqlite/catalog/catalog_test.go.md) | Go 测试 |
| [internal/storage/sqlite/catalog/credentials.go](../../internal/storage/sqlite/catalog/credentials.go) | [credentials.go.md](internal/storage/sqlite/catalog/credentials.go.md) | Go |
| [internal/storage/sqlite/catalog/credentials_test.go](../../internal/storage/sqlite/catalog/credentials_test.go) | [credentials_test.go.md](internal/storage/sqlite/catalog/credentials_test.go.md) | Go 测试 |
| [internal/storage/sqlite/catalog/errors.go](../../internal/storage/sqlite/catalog/errors.go) | [errors.go.md](internal/storage/sqlite/catalog/errors.go.md) | Go |
| [internal/storage/sqlite/catalog/gorm_test.go](../../internal/storage/sqlite/catalog/gorm_test.go) | [gorm_test.go.md](internal/storage/sqlite/catalog/gorm_test.go.md) | Go 测试 |
| [internal/storage/sqlite/catalog/id_test.go](../../internal/storage/sqlite/catalog/id_test.go) | [id_test.go.md](internal/storage/sqlite/catalog/id_test.go.md) | Go 测试 |
| [internal/storage/sqlite/catalog/mapping.go](../../internal/storage/sqlite/catalog/mapping.go) | [mapping.go.md](internal/storage/sqlite/catalog/mapping.go.md) | Go |
| [internal/storage/sqlite/catalog/models.go](../../internal/storage/sqlite/catalog/models.go) | [models.go.md](internal/storage/sqlite/catalog/models.go.md) | Go |
| [internal/storage/sqlite/catalog/providers.go](../../internal/storage/sqlite/catalog/providers.go) | [providers.go.md](internal/storage/sqlite/catalog/providers.go.md) | Go |
| [internal/storage/sqlite/catalog/repository.go](../../internal/storage/sqlite/catalog/repository.go) | [repository.go.md](internal/storage/sqlite/catalog/repository.go.md) | Go |
| [internal/storage/sqlite/catalog/service_test.go](../../internal/storage/sqlite/catalog/service_test.go) | [service_test.go.md](internal/storage/sqlite/catalog/service_test.go.md) | Go 测试 |
| [internal/storage/sqlite/conversation/begin.go](../../internal/storage/sqlite/conversation/begin.go) | [begin.go.md](internal/storage/sqlite/conversation/begin.go.md) | Go |
| [internal/storage/sqlite/conversation/compaction.go](../../internal/storage/sqlite/conversation/compaction.go) | [compaction.go.md](internal/storage/sqlite/conversation/compaction.go.md) | Go |
| [internal/storage/sqlite/conversation/context.go](../../internal/storage/sqlite/conversation/context.go) | [context.go.md](internal/storage/sqlite/conversation/context.go.md) | Go |
| [internal/storage/sqlite/conversation/context_test.go](../../internal/storage/sqlite/conversation/context_test.go) | [context_test.go.md](internal/storage/sqlite/conversation/context_test.go.md) | Go 测试 |
| [internal/storage/sqlite/conversation/followup.go](../../internal/storage/sqlite/conversation/followup.go) | [followup.go.md](internal/storage/sqlite/conversation/followup.go.md) | Go |
| [internal/storage/sqlite/conversation/history.go](../../internal/storage/sqlite/conversation/history.go) | [history.go.md](internal/storage/sqlite/conversation/history.go.md) | Go |
| [internal/storage/sqlite/conversation/history_test.go](../../internal/storage/sqlite/conversation/history_test.go) | [history_test.go.md](internal/storage/sqlite/conversation/history_test.go.md) | Go 测试 |
| [internal/storage/sqlite/conversation/repository.go](../../internal/storage/sqlite/conversation/repository.go) | [repository.go.md](internal/storage/sqlite/conversation/repository.go.md) | Go |
| [internal/storage/sqlite/conversation/repository_test.go](../../internal/storage/sqlite/conversation/repository_test.go) | [repository_test.go.md](internal/storage/sqlite/conversation/repository_test.go.md) | Go 测试 |
| [internal/storage/sqlite/conversation/usage.go](../../internal/storage/sqlite/conversation/usage.go) | [usage.go.md](internal/storage/sqlite/conversation/usage.go.md) | Go |
| [internal/storage/sqlite/files.go](../../internal/storage/sqlite/files.go) | [files.go.md](internal/storage/sqlite/files.go.md) | Go |
| [internal/storage/sqlite/mcp/repository.go](../../internal/storage/sqlite/mcp/repository.go) | [repository.go.md](internal/storage/sqlite/mcp/repository.go.md) | Go |
| [internal/storage/sqlite/migrations/000001_01_create_catalog.go](../../internal/storage/sqlite/migrations/000001_01_create_catalog.go) | [000001_01_create_catalog.go.md](internal/storage/sqlite/migrations/000001_01_create_catalog.go.md) | Go |
| [internal/storage/sqlite/migrations/000001_02_create_conversations.go](../../internal/storage/sqlite/migrations/000001_02_create_conversations.go) | [000001_02_create_conversations.go.md](internal/storage/sqlite/migrations/000001_02_create_conversations.go.md) | Go |
| [internal/storage/sqlite/migrations/000001_03_create_approvals.go](../../internal/storage/sqlite/migrations/000001_03_create_approvals.go) | [000001_03_create_approvals.go.md](internal/storage/sqlite/migrations/000001_03_create_approvals.go.md) | Go |
| [internal/storage/sqlite/migrations/000001_04_create_settings.go](../../internal/storage/sqlite/migrations/000001_04_create_settings.go) | [000001_04_create_settings.go.md](internal/storage/sqlite/migrations/000001_04_create_settings.go.md) | Go |
| [internal/storage/sqlite/migrations/000002_01_optional_workspace_root.go](../../internal/storage/sqlite/migrations/000002_01_optional_workspace_root.go) | [000002_01_optional_workspace_root.go.md](internal/storage/sqlite/migrations/000002_01_optional_workspace_root.go.md) | Go |
| [internal/storage/sqlite/migrations/000003_01_allow_auto_reviewer.go](../../internal/storage/sqlite/migrations/000003_01_allow_auto_reviewer.go) | [000003_01_allow_auto_reviewer.go.md](internal/storage/sqlite/migrations/000003_01_allow_auto_reviewer.go.md) | Go |
| [internal/storage/sqlite/migrations/000007_01_create_mcp.go](../../internal/storage/sqlite/migrations/000007_01_create_mcp.go) | [000007_01_create_mcp.go.md](internal/storage/sqlite/migrations/000007_01_create_mcp.go.md) | Go |
| [internal/storage/sqlite/migrations/auto_reviewer_test.go](../../internal/storage/sqlite/migrations/auto_reviewer_test.go) | [auto_reviewer_test.go.md](internal/storage/sqlite/migrations/auto_reviewer_test.go.md) | Go 测试 |
| [internal/storage/sqlite/migrations/context_test.go](../../internal/storage/sqlite/migrations/context_test.go) | [context_test.go.md](internal/storage/sqlite/migrations/context_test.go.md) | Go 测试 |
| [internal/storage/sqlite/migrations/credentials_test.go](../../internal/storage/sqlite/migrations/credentials_test.go) | [credentials_test.go.md](internal/storage/sqlite/migrations/credentials_test.go.md) | Go 测试 |
| [internal/storage/sqlite/migrations/mcp_test.go](../../internal/storage/sqlite/migrations/mcp_test.go) | [mcp_test.go.md](internal/storage/sqlite/migrations/mcp_test.go.md) | Go 测试 |
| [internal/storage/sqlite/migrations/migrate.go](../../internal/storage/sqlite/migrations/migrate.go) | [migrate.go.md](internal/storage/sqlite/migrations/migrate.go.md) | Go |
| [internal/storage/sqlite/migrations/migrate_test.go](../../internal/storage/sqlite/migrations/migrate_test.go) | [migrate_test.go.md](internal/storage/sqlite/migrations/migrate_test.go.md) | Go 测试 |
| [internal/storage/sqlite/migrations/optional_workspace_root_test.go](../../internal/storage/sqlite/migrations/optional_workspace_root_test.go) | [optional_workspace_root_test.go.md](internal/storage/sqlite/migrations/optional_workspace_root_test.go.md) | Go 测试 |
| [internal/storage/sqlite/migrations/rebuild_table.go](../../internal/storage/sqlite/migrations/rebuild_table.go) | [rebuild_table.go.md](internal/storage/sqlite/migrations/rebuild_table.go.md) | Go |
| [internal/storage/sqlite/migrations/settings_test.go](../../internal/storage/sqlite/migrations/settings_test.go) | [settings_test.go.md](internal/storage/sqlite/migrations/settings_test.go.md) | Go 测试 |
| [internal/storage/sqlite/migrations/validation_test.go](../../internal/storage/sqlite/migrations/validation_test.go) | [validation_test.go.md](internal/storage/sqlite/migrations/validation_test.go.md) | Go 测试 |
| [internal/storage/sqlite/ownership.go](../../internal/storage/sqlite/ownership.go) | [ownership.go.md](internal/storage/sqlite/ownership.go.md) | Go |
| [internal/storage/sqlite/ownership_test.go](../../internal/storage/sqlite/ownership_test.go) | [ownership_test.go.md](internal/storage/sqlite/ownership_test.go.md) | Go 测试 |
| [internal/storage/sqlite/schema/approval.go](../../internal/storage/sqlite/schema/approval.go) | [approval.go.md](internal/storage/sqlite/schema/approval.go.md) | Go |
| [internal/storage/sqlite/schema/context.go](../../internal/storage/sqlite/schema/context.go) | [context.go.md](internal/storage/sqlite/schema/context.go.md) | Go |
| [internal/storage/sqlite/schema/conversation.go](../../internal/storage/sqlite/schema/conversation.go) | [conversation.go.md](internal/storage/sqlite/schema/conversation.go.md) | Go |
| [internal/storage/sqlite/schema/directory.go](../../internal/storage/sqlite/schema/directory.go) | [directory.go.md](internal/storage/sqlite/schema/directory.go.md) | Go |
| [internal/storage/sqlite/schema/followup.go](../../internal/storage/sqlite/schema/followup.go) | [followup.go.md](internal/storage/sqlite/schema/followup.go.md) | Go |
| [internal/storage/sqlite/schema/mcp_credentials.go](../../internal/storage/sqlite/schema/mcp_credentials.go) | [mcp_credentials.go.md](internal/storage/sqlite/schema/mcp_credentials.go.md) | Go |
| [internal/storage/sqlite/schema/message.go](../../internal/storage/sqlite/schema/message.go) | [message.go.md](internal/storage/sqlite/schema/message.go.md) | Go |
| [internal/storage/sqlite/schema/model.go](../../internal/storage/sqlite/schema/model.go) | [model.go.md](internal/storage/sqlite/schema/model.go.md) | Go |
| [internal/storage/sqlite/schema/provider.go](../../internal/storage/sqlite/schema/provider.go) | [provider.go.md](internal/storage/sqlite/schema/provider.go.md) | Go |
| [internal/storage/sqlite/schema/run.go](../../internal/storage/sqlite/schema/run.go) | [run.go.md](internal/storage/sqlite/schema/run.go.md) | Go |
| [internal/storage/sqlite/schema/settings.go](../../internal/storage/sqlite/schema/settings.go) | [settings.go.md](internal/storage/sqlite/schema/settings.go.md) | Go |
| [internal/storage/sqlite/schema/step.go](../../internal/storage/sqlite/schema/step.go) | [step.go.md](internal/storage/sqlite/schema/step.go.md) | Go |
| [internal/storage/sqlite/schema/workspace.go](../../internal/storage/sqlite/schema/workspace.go) | [workspace.go.md](internal/storage/sqlite/schema/workspace.go.md) | Go |
| [internal/storage/sqlite/settings/repository.go](../../internal/storage/sqlite/settings/repository.go) | [repository.go.md](internal/storage/sqlite/settings/repository.go.md) | Go |
| [internal/storage/sqlite/settings/repository_test.go](../../internal/storage/sqlite/settings/repository_test.go) | [repository_test.go.md](internal/storage/sqlite/settings/repository_test.go.md) | Go 测试 |
| [internal/storage/sqlite/sqlite.go](../../internal/storage/sqlite/sqlite.go) | [sqlite.go.md](internal/storage/sqlite/sqlite.go.md) | Go |
| [internal/storage/sqlite/sqlite_test.go](../../internal/storage/sqlite/sqlite_test.go) | [sqlite_test.go.md](internal/storage/sqlite/sqlite_test.go.md) | Go 测试 |
| [internal/storage/sqlite/workspace/repository.go](../../internal/storage/sqlite/workspace/repository.go) | [repository.go.md](internal/storage/sqlite/workspace/repository.go.md) | Go |
| [internal/storage/sqlite/workspace/repository_test.go](../../internal/storage/sqlite/workspace/repository_test.go) | [repository_test.go.md](internal/storage/sqlite/workspace/repository_test.go.md) | Go 测试 |

### internal/tool

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/tool/arguments.go](../../internal/tool/arguments.go) | [arguments.go.md](internal/tool/arguments.go.md) | Go |
| [internal/tool/builtin/attachment/open/open.go](../../internal/tool/builtin/attachment/open/open.go) | [open.go.md](internal/tool/builtin/attachment/open/open.go.md) | Go |
| [internal/tool/builtin/command/command.go](../../internal/tool/builtin/command/command.go) | [command.go.md](internal/tool/builtin/command/command.go.md) | Go |
| [internal/tool/builtin/command/run/arguments.go](../../internal/tool/builtin/command/run/arguments.go) | [arguments.go.md](internal/tool/builtin/command/run/arguments.go.md) | Go |
| [internal/tool/builtin/command/run/environment.go](../../internal/tool/builtin/command/run/environment.go) | [environment.go.md](internal/tool/builtin/command/run/environment.go.md) | Go |
| [internal/tool/builtin/command/run/environment_test.go](../../internal/tool/builtin/command/run/environment_test.go) | [environment_test.go.md](internal/tool/builtin/command/run/environment_test.go.md) | Go 测试 |
| [internal/tool/builtin/command/run/invocation.go](../../internal/tool/builtin/command/run/invocation.go) | [invocation.go.md](internal/tool/builtin/command/run/invocation.go.md) | Go |
| [internal/tool/builtin/command/run/output.go](../../internal/tool/builtin/command/run/output.go) | [output.go.md](internal/tool/builtin/command/run/output.go.md) | Go |
| [internal/tool/builtin/command/run/output_test.go](../../internal/tool/builtin/command/run/output_test.go) | [output_test.go.md](internal/tool/builtin/command/run/output_test.go.md) | Go 测试 |
| [internal/tool/builtin/command/run/process.go](../../internal/tool/builtin/command/run/process.go) | [process.go.md](internal/tool/builtin/command/run/process.go.md) | Go |
| [internal/tool/builtin/command/run/process_other.go](../../internal/tool/builtin/command/run/process_other.go) | [process_other.go.md](internal/tool/builtin/command/run/process_other.go.md) | Go |
| [internal/tool/builtin/command/run/process_test.go](../../internal/tool/builtin/command/run/process_test.go) | [process_test.go.md](internal/tool/builtin/command/run/process_test.go.md) | Go 测试 |
| [internal/tool/builtin/command/run/process_unix.go](../../internal/tool/builtin/command/run/process_unix.go) | [process_unix.go.md](internal/tool/builtin/command/run/process_unix.go.md) | Go |
| [internal/tool/builtin/command/run/process_unix_test.go](../../internal/tool/builtin/command/run/process_unix_test.go) | [process_unix_test.go.md](internal/tool/builtin/command/run/process_unix_test.go.md) | Go 测试 |
| [internal/tool/builtin/command/run/process_windows.go](../../internal/tool/builtin/command/run/process_windows.go) | [process_windows.go.md](internal/tool/builtin/command/run/process_windows.go.md) | Go |
| [internal/tool/builtin/command/run/process_windows_test.go](../../internal/tool/builtin/command/run/process_windows_test.go) | [process_windows_test.go.md](internal/tool/builtin/command/run/process_windows_test.go.md) | Go 测试 |
| [internal/tool/builtin/command/run/protection_darwin.go](../../internal/tool/builtin/command/run/protection_darwin.go) | [protection_darwin.go.md](internal/tool/builtin/command/run/protection_darwin.go.md) | Go |
| [internal/tool/builtin/command/run/protection_other.go](../../internal/tool/builtin/command/run/protection_other.go) | [protection_other.go.md](internal/tool/builtin/command/run/protection_other.go.md) | Go |
| [internal/tool/builtin/command/run/protection_windows.go](../../internal/tool/builtin/command/run/protection_windows.go) | [protection_windows.go.md](internal/tool/builtin/command/run/protection_windows.go.md) | Go |
| [internal/tool/builtin/command/run/risk.go](../../internal/tool/builtin/command/run/risk.go) | [risk.go.md](internal/tool/builtin/command/run/risk.go.md) | Go |
| [internal/tool/builtin/command/run/run.go](../../internal/tool/builtin/command/run/run.go) | [run.go.md](internal/tool/builtin/command/run/run.go.md) | Go |
| [internal/tool/builtin/command/run/run_test.go](../../internal/tool/builtin/command/run/run_test.go) | [run_test.go.md](internal/tool/builtin/command/run/run_test.go.md) | Go 测试 |
| [internal/tool/builtin/command/run/runtime_test.go](../../internal/tool/builtin/command/run/runtime_test.go) | [runtime_test.go.md](internal/tool/builtin/command/run/runtime_test.go.md) | Go 测试 |
| [internal/tool/builtin/command/run/sandbox.go](../../internal/tool/builtin/command/run/sandbox.go) | [sandbox.go.md](internal/tool/builtin/command/run/sandbox.go.md) | Go |
| [internal/tool/builtin/command/run/sandbox_darwin.go](../../internal/tool/builtin/command/run/sandbox_darwin.go) | [sandbox_darwin.go.md](internal/tool/builtin/command/run/sandbox_darwin.go.md) | Go |
| [internal/tool/builtin/command/run/sandbox_darwin_test.go](../../internal/tool/builtin/command/run/sandbox_darwin_test.go) | [sandbox_darwin_test.go.md](internal/tool/builtin/command/run/sandbox_darwin_test.go.md) | Go 测试 |
| [internal/tool/builtin/command/run/sandbox_other.go](../../internal/tool/builtin/command/run/sandbox_other.go) | [sandbox_other.go.md](internal/tool/builtin/command/run/sandbox_other.go.md) | Go |
| [internal/tool/builtin/command/run/sandbox_recovery_windows.go](../../internal/tool/builtin/command/run/sandbox_recovery_windows.go) | [sandbox_recovery_windows.go.md](internal/tool/builtin/command/run/sandbox_recovery_windows.go.md) | Go |
| [internal/tool/builtin/command/run/sandbox_security_windows.go](../../internal/tool/builtin/command/run/sandbox_security_windows.go) | [sandbox_security_windows.go.md](internal/tool/builtin/command/run/sandbox_security_windows.go.md) | Go |
| [internal/tool/builtin/command/run/sandbox_state_other.go](../../internal/tool/builtin/command/run/sandbox_state_other.go) | [sandbox_state_other.go.md](internal/tool/builtin/command/run/sandbox_state_other.go.md) | Go |
| [internal/tool/builtin/command/run/sandbox_windows.go](../../internal/tool/builtin/command/run/sandbox_windows.go) | [sandbox_windows.go.md](internal/tool/builtin/command/run/sandbox_windows.go.md) | Go |
| [internal/tool/builtin/command/run/sandbox_windows_test.go](../../internal/tool/builtin/command/run/sandbox_windows_test.go) | [sandbox_windows_test.go.md](internal/tool/builtin/command/run/sandbox_windows_test.go.md) | Go 测试 |
| [internal/tool/builtin/command/run/shell.go](../../internal/tool/builtin/command/run/shell.go) | [shell.go.md](internal/tool/builtin/command/run/shell.go.md) | Go |
| [internal/tool/builtin/command/run/shell_test.go](../../internal/tool/builtin/command/run/shell_test.go) | [shell_test.go.md](internal/tool/builtin/command/run/shell_test.go.md) | Go 测试 |
| [internal/tool/builtin/command/run/target.go](../../internal/tool/builtin/command/run/target.go) | [target.go.md](internal/tool/builtin/command/run/target.go.md) | Go |
| [internal/tool/builtin/command/run/testdata/process/acl_other.go](../../internal/tool/builtin/command/run/testdata/process/acl_other.go) | [acl_other.go.md](internal/tool/builtin/command/run/testdata/process/acl_other.go.md) | Go 测试辅助程序 |
| [internal/tool/builtin/command/run/testdata/process/acl_windows.go](../../internal/tool/builtin/command/run/testdata/process/acl_windows.go) | [acl_windows.go.md](internal/tool/builtin/command/run/testdata/process/acl_windows.go.md) | Go 测试辅助程序 |
| [internal/tool/builtin/command/run/testdata/process/main.go](../../internal/tool/builtin/command/run/testdata/process/main.go) | [main.go.md](internal/tool/builtin/command/run/testdata/process/main.go.md) | Go 测试辅助程序 |
| [internal/tool/builtin/filesystem/artifact/artifact.go](../../internal/tool/builtin/filesystem/artifact/artifact.go) | [artifact.go.md](internal/tool/builtin/filesystem/artifact/artifact.go.md) | Go |
| [internal/tool/builtin/filesystem/filesystem.go](../../internal/tool/builtin/filesystem/filesystem.go) | [filesystem.go.md](internal/tool/builtin/filesystem/filesystem.go.md) | Go |
| [internal/tool/builtin/filesystem/filesystem_test.go](../../internal/tool/builtin/filesystem/filesystem_test.go) | [filesystem_test.go.md](internal/tool/builtin/filesystem/filesystem_test.go.md) | Go 测试 |
| [internal/tool/builtin/filesystem/internal/fileaccess/io.go](../../internal/tool/builtin/filesystem/internal/fileaccess/io.go) | [io.go.md](internal/tool/builtin/filesystem/internal/fileaccess/io.go.md) | Go |
| [internal/tool/builtin/filesystem/internal/fileaccess/open_other.go](../../internal/tool/builtin/filesystem/internal/fileaccess/open_other.go) | [open_other.go.md](internal/tool/builtin/filesystem/internal/fileaccess/open_other.go.md) | Go |
| [internal/tool/builtin/filesystem/internal/fileaccess/open_unix.go](../../internal/tool/builtin/filesystem/internal/fileaccess/open_unix.go) | [open_unix.go.md](internal/tool/builtin/filesystem/internal/fileaccess/open_unix.go.md) | Go |
| [internal/tool/builtin/filesystem/internal/fileaccess/path.go](../../internal/tool/builtin/filesystem/internal/fileaccess/path.go) | [path.go.md](internal/tool/builtin/filesystem/internal/fileaccess/path.go.md) | Go |
| [internal/tool/builtin/filesystem/internal/fileaccess/target.go](../../internal/tool/builtin/filesystem/internal/fileaccess/target.go) | [target.go.md](internal/tool/builtin/filesystem/internal/fileaccess/target.go.md) | Go |
| [internal/tool/builtin/filesystem/internal/fileaccess/target_test.go](../../internal/tool/builtin/filesystem/internal/fileaccess/target_test.go) | [target_test.go.md](internal/tool/builtin/filesystem/internal/fileaccess/target_test.go.md) | Go 测试 |
| [internal/tool/builtin/filesystem/list/arguments.go](../../internal/tool/builtin/filesystem/list/arguments.go) | [arguments.go.md](internal/tool/builtin/filesystem/list/arguments.go.md) | Go |
| [internal/tool/builtin/filesystem/list/list.go](../../internal/tool/builtin/filesystem/list/list.go) | [list.go.md](internal/tool/builtin/filesystem/list/list.go.md) | Go |
| [internal/tool/builtin/filesystem/list/list_test.go](../../internal/tool/builtin/filesystem/list/list_test.go) | [list_test.go.md](internal/tool/builtin/filesystem/list/list_test.go.md) | Go 测试 |
| [internal/tool/builtin/filesystem/patch/patch.go](../../internal/tool/builtin/filesystem/patch/patch.go) | [patch.go.md](internal/tool/builtin/filesystem/patch/patch.go.md) | Go |
| [internal/tool/builtin/filesystem/patch/patch_commit_other.go](../../internal/tool/builtin/filesystem/patch/patch_commit_other.go) | [patch_commit_other.go.md](internal/tool/builtin/filesystem/patch/patch_commit_other.go.md) | Go |
| [internal/tool/builtin/filesystem/patch/patch_commit_unix.go](../../internal/tool/builtin/filesystem/patch/patch_commit_unix.go) | [patch_commit_unix.go.md](internal/tool/builtin/filesystem/patch/patch_commit_unix.go.md) | Go |
| [internal/tool/builtin/filesystem/patch/patch_commit_windows.go](../../internal/tool/builtin/filesystem/patch/patch_commit_windows.go) | [patch_commit_windows.go.md](internal/tool/builtin/filesystem/patch/patch_commit_windows.go.md) | Go |
| [internal/tool/builtin/filesystem/patch/patch_parse.go](../../internal/tool/builtin/filesystem/patch/patch_parse.go) | [patch_parse.go.md](internal/tool/builtin/filesystem/patch/patch_parse.go.md) | Go |
| [internal/tool/builtin/filesystem/patch/patch_parse_test.go](../../internal/tool/builtin/filesystem/patch/patch_parse_test.go) | [patch_parse_test.go.md](internal/tool/builtin/filesystem/patch/patch_parse_test.go.md) | Go 测试 |
| [internal/tool/builtin/filesystem/patch/patch_target.go](../../internal/tool/builtin/filesystem/patch/patch_target.go) | [patch_target.go.md](internal/tool/builtin/filesystem/patch/patch_target.go.md) | Go |
| [internal/tool/builtin/filesystem/patch/patch_test.go](../../internal/tool/builtin/filesystem/patch/patch_test.go) | [patch_test.go.md](internal/tool/builtin/filesystem/patch/patch_test.go.md) | Go 测试 |
| [internal/tool/builtin/filesystem/patch/patch_unix_test.go](../../internal/tool/builtin/filesystem/patch/patch_unix_test.go) | [patch_unix_test.go.md](internal/tool/builtin/filesystem/patch/patch_unix_test.go.md) | Go 测试 |
| [internal/tool/builtin/filesystem/patch/patch_windows_test.go](../../internal/tool/builtin/filesystem/patch/patch_windows_test.go) | [patch_windows_test.go.md](internal/tool/builtin/filesystem/patch/patch_windows_test.go.md) | Go 测试 |
| [internal/tool/builtin/filesystem/patch/patch_write.go](../../internal/tool/builtin/filesystem/patch/patch_write.go) | [patch_write.go.md](internal/tool/builtin/filesystem/patch/patch_write.go.md) | Go |
| [internal/tool/builtin/filesystem/read/arguments.go](../../internal/tool/builtin/filesystem/read/arguments.go) | [arguments.go.md](internal/tool/builtin/filesystem/read/arguments.go.md) | Go |
| [internal/tool/builtin/filesystem/read/read.go](../../internal/tool/builtin/filesystem/read/read.go) | [read.go.md](internal/tool/builtin/filesystem/read/read.go.md) | Go |
| [internal/tool/builtin/filesystem/read/read_test.go](../../internal/tool/builtin/filesystem/read/read_test.go) | [read_test.go.md](internal/tool/builtin/filesystem/read/read_test.go.md) | Go 测试 |
| [internal/tool/builtin/filesystem/search/arguments.go](../../internal/tool/builtin/filesystem/search/arguments.go) | [arguments.go.md](internal/tool/builtin/filesystem/search/arguments.go.md) | Go |
| [internal/tool/builtin/filesystem/search/search.go](../../internal/tool/builtin/filesystem/search/search.go) | [search.go.md](internal/tool/builtin/filesystem/search/search.go.md) | Go |
| [internal/tool/builtin/filesystem/search/search_test.go](../../internal/tool/builtin/filesystem/search/search_test.go) | [search_test.go.md](internal/tool/builtin/filesystem/search/search_test.go.md) | Go 测试 |
| [internal/tool/builtin/filesystem/skills_test.go](../../internal/tool/builtin/filesystem/skills_test.go) | [skills_test.go.md](internal/tool/builtin/filesystem/skills_test.go.md) | Go 测试 |
| [internal/tool/builtin/internal/arguments/decode.go](../../internal/tool/builtin/internal/arguments/decode.go) | [decode.go.md](internal/tool/builtin/internal/arguments/decode.go.md) | Go |
| [internal/tool/builtin/internal/arguments/decode_test.go](../../internal/tool/builtin/internal/arguments/decode_test.go) | [decode_test.go.md](internal/tool/builtin/internal/arguments/decode_test.go.md) | Go 测试 |
| [internal/tool/builtin/runtime/dependencies.go](../../internal/tool/builtin/runtime/dependencies.go) | [dependencies.go.md](internal/tool/builtin/runtime/dependencies.go.md) | Go |
| [internal/tool/directory.go](../../internal/tool/directory.go) | [directory.go.md](internal/tool/directory.go.md) | Go |
| [internal/tool/directory_test.go](../../internal/tool/directory_test.go) | [directory_test.go.md](internal/tool/directory_test.go.md) | Go 测试 |
| [internal/tool/executor/directory.go](../../internal/tool/executor/directory.go) | [directory.go.md](internal/tool/executor/directory.go.md) | Go |
| [internal/tool/executor/executor.go](../../internal/tool/executor/executor.go) | [executor.go.md](internal/tool/executor/executor.go.md) | Go |
| [internal/tool/executor/executor_test.go](../../internal/tool/executor/executor_test.go) | [executor_test.go.md](internal/tool/executor/executor_test.go.md) | Go 测试 |
| [internal/tool/policy/directory.go](../../internal/tool/policy/directory.go) | [directory.go.md](internal/tool/policy/directory.go.md) | Go |
| [internal/tool/policy/operation.go](../../internal/tool/policy/operation.go) | [operation.go.md](internal/tool/policy/operation.go.md) | Go |
| [internal/tool/policy/operation_test.go](../../internal/tool/policy/operation_test.go) | [operation_test.go.md](internal/tool/policy/operation_test.go.md) | Go 测试 |
| [internal/tool/policy/policy.go](../../internal/tool/policy/policy.go) | [policy.go.md](internal/tool/policy/policy.go.md) | Go |
| [internal/tool/policy/policy_test.go](../../internal/tool/policy/policy_test.go) | [policy_test.go.md](internal/tool/policy/policy_test.go.md) | Go 测试 |
| [internal/tool/protected.go](../../internal/tool/protected.go) | [protected.go.md](internal/tool/protected.go.md) | Go |
| [internal/tool/registry/registry.go](../../internal/tool/registry/registry.go) | [registry.go.md](internal/tool/registry/registry.go.md) | Go |
| [internal/tool/types.go](../../internal/tool/types.go) | [types.go.md](internal/tool/types.go.md) | Go |

### internal/workspace

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [internal/workspace/errors.go](../../internal/workspace/errors.go) | [errors.go.md](internal/workspace/errors.go.md) | Go |
| [internal/workspace/repository.go](../../internal/workspace/repository.go) | [repository.go.md](internal/workspace/repository.go.md) | Go |
| [internal/workspace/service/service.go](../../internal/workspace/service/service.go) | [service.go.md](internal/workspace/service/service.go.md) | Go |
| [internal/workspace/types.go](../../internal/workspace/types.go) | [types.go.md](internal/workspace/types.go.md) | Go |

### scripts

| 原代码 | 中文对照 | 类型 |
| --- | --- | --- |
| [scripts/check-race.sh](../../scripts/check-race.sh) | [check-race.sh.md](scripts/check-race.sh.md) | Shell |
