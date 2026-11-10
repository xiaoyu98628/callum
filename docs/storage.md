# 存储与模型配置

本文件说明后端的数据路径、配置存储、结构校验、迁移和并发约束。命令从仓库根目录执行；启动和构建见 [运行与开发](development.md)，运行时资源所有权见 [后端架构](architecture.md#运行时与资源所有权)。

持久化使用 GORM v1.31.2、官方 SQLite 方言 v1.6.0 和 `modernc.org/sqlite` v1.61.0，
具体依赖以 `go.mod` 为准。标准库和原有模型 SDK 不提供所需的 ORM 与 SQLite 引擎，
因此新增这些依赖。官方方言接收显式创建的 modernc 连接池，支持 `CGO_ENABLED=0`；
`mattn/go-sqlite3` 是方言的间接依赖，本模块不使用它建立连接。

业务仓储通过 GORM 组合计数、关联查询、`IN` / `EXISTS` 子查询和窗口查询，JSON 提取及 `ROW_NUMBER()` 等 SQLite 表达式在数据库内执行。待核实操作先按固定操作取最新结果，再筛选需要确认的状态。最新检查点仍逐行读取并检查累计容量。初始化的 `PRAGMA` 和结构元数据检查使用专用 SQL。底层连接池及流式结果由存储层管理生命周期。

配置存储的装配顺序为 `config.LoadArgs(args)` / `config.Load(dataDir)` →
`sqlite.Open(ctx, cfg.DatabasePath)` → 创建凭据 Vault 并执行 `catalog.MigrateCredentials` →
`catalog.NewRepository(db.DB, vault)` → `providerservice.New(repository)`。应用在打开数据库前取得实例锁。
这里的 `catalog` 为 `internal/storage/sqlite/catalog`，`providerservice` 为 `internal/model/provider/service` 的导入别名。
数据库打开成功后由创建者调用 `db.Close()`；存储实现使用传入的 GORM 实例，
业务服务使用传入的存储接口，二者均不关闭数据库。数据库 `Close` 可以重复调用。
`sqlite.Open` 将 GORM 连接传给 `migrations.Run`。迁移包复用 `schema` 的表映射，按编号和业务分组维护建表及校验。
`000001_01_create_catalog.go` 管理厂商和模型。`000001_02_create_conversations.go` 管理工作区及其其他目录、会话、消息、执行、步骤及上下文版本。
`000001_03_create_approvals.go` 管理审批和目录授权。`000001_04_create_settings.go` 初始化应用设置与默认的执行、压缩设置组。
`migrate.go` 统一管理事务、校验与结构版本。空库按当前表映射直接初始化为版本 `4`。`000002_*` 将主目录改为可选，`000003_*` 扩展执行快照的审批人约束。
初始化失败会关闭此前创建的连接池；初始化上下文不绑定到运行时实例，
每次配置操作都接收自己的 `context.Context`。

## 数据路径与访问权限

| 使用场景 | 模块配置约定 |
| --- | --- |
| 开发构建、GoLand 默认调试及 `go run` | `config.Load("")` 自动解析为 Callum 项目根目录下 `.callum/dev1` |
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

API Key 使用 AES-256-GCM 加密后保存在 SQLite；数据库中的会话、步骤等其他内容并未整体加密。
厂商查询仅返回 `HasAPIKey`，不返回凭据；`Resolve` 在内部解析运行配置时才解密凭据，
运行配置不得整体写入日志或作为普通配置快照导出。GORM 的默认 SQL 日志已关闭，
存储错误由调用边界统一处理。数据库文件及可能出现的 SQLite 辅助文件都应作为敏感数据管理。

## 系统凭据与迁移

`internal/credentials` 提供加密契约和 Vault；`system` 实现系统主密钥存储。每个数据目录使用随机 32 字节主密钥，系统条目命名空间为 `org.callum.credentials.v1`，条目标识是规范化数据库绝对路径的 SHA-256。macOS 使用登录钥匙串（固定 `/usr/bin/security`，密钥从标准输入传入），Windows 使用当前用户的 [Credential Manager](https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credreadw)，Linux 使用当前用户本地 Unix D-Bus 会话上的 [Secret Service](https://specifications.freedesktop.org/secret-service/latest/ch03.html) 默认集合。Linux 新增纯 Go 的 `github.com/godbus/dbus/v5`，用于已有依赖未提供的 D-Bus 协议；不依赖 CGo，不创建或解锁系统集合。

密文保存格式版本、非秘密主密钥标识、随机 nonce 和认证标签，并将厂商 ID 绑定为认证数据，防止跨记录替换。空 API Key 无需系统凭据条目；首次保存非空值才创建主密钥，并读取核对后再写数据库。主密钥不可用、锁定、缺失或密文校验失败时明确拒绝操作，不回退明文、不自动替换丢失的主密钥。配置接口返回 `credentials_unavailable`（错误码 `1012`）。API Key 保留、替换和清空语义不变。

版本 `4` 的 `credential_format` 仅允许 `0`（待迁移明文）和 `1`（密文或空值）。启动在实例锁下先检查已有密文，再于事务外取得主密钥，随后以单个事务加密全部旧凭据；任一失败整体回滚凭据更新并阻止运行时启动，下次启动可以重试。表结构升级与凭据迁移分别提交，迁移失败的旧值仍由格式列明确识别，不会被误当密文。迁移使用有界批次读取，启用 SQLite `secure_delete`，成功后截断 WAL；清理失败也阻止启动。这只清除 SQLite 可控制的逻辑旧页，不保证删除已有备份、文件系统快照或物理磁盘残留。

密文保留原主密钥标识，因此同一系统用户移动数据库目录后仍可读取旧条目；后续新密钥值使用新目录的主密钥。跨系统或账户复制数据库不会同时复制系统主密钥；数据库文件本身不能作为完整的凭据备份。启动因主密钥缺失而失败时，需恢复对应系统凭据；当前没有绕过该检查或在界面重置丢失主密钥的入口。删除厂商不自动删除共享主密钥条目。备份数据库仍需按敏感数据管理。运行时等待任务结束后清空持有的主密钥缓存，Go/系统调用中的短期明文不能视为可保证的全内存擦除。

## 表结构与校验

| 表 | 字段 |
| --- | --- |
| `model_providers` | `id`、`name`、`protocol`、`base_url`、`api_key`、`credential_format`、`created_at`、`updated_at` |
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

## 迁移与并发

数据库使用 STRICT 表、外键约束、联合唯一索引及字段检查约束。
结构版本保存在 SQLite `user_version` 中，当前版本为 `4`。
空库在同一写事务中创建 12 张业务表及其约束和索引，初始化两个默认设置组。校验通过后记录版本 `4`，失败时回滚。
打开已有库时同时校验版本、表结构、约束及设置组。版本 `1` 先将主目录改为可选，版本 `1` 和 `2` 都会升级执行快照的审批人检查约束。服务在实例锁下固定初始化连接并临时关闭外键，在同一事务中重建所需表，保留项目、消息、执行、审批、索引和触发器。版本 `1`、`2`、`3` 同时补充凭据格式列，旧值标记为待加密。通过外键与结构校验后提交版本 `4`，失败回滚并恢复外键设置。其他未支持版本和不完整的旧结构保留原数据并拒绝打开。
开发时可通过 `--data-dir` 指向独立的新数据目录。明确需要保留的旧开发数据可离线备份到独立目录，并用当前程序校验转换后的副本。
启动使用显式建表和结构校验。开发阶段调整数据库结构时，优先更新建表迁移和表结构定义，并用空库验证初始化。明确可丢弃的开发数据库可以删除重建。
只有确认需要保留既有数据时，才考虑新增 `ALTER TABLE`、数据迁移或旧结构兼容逻辑，并验证升级和失败回滚。

连接池上限为一个连接，外键检查和 5 秒锁等待对每个连接生效。
事务使用 `BEGIN IMMEDIATE`，保证并发初始化时串行检查和更新结构版本。
每次配置操作传递调用方上下文；查询结果读取完成后释放连接，
`Resolve` 通过一次关联查询读取厂商和模型，不在等待模型响应期间持有连接或事务。

应用在迁移和恢复之前取得数据库旁的 `callum.db.lock` 文件锁，并持有到资源关闭。同一数据库只允许一个运行中的服务实例，多个客户端窗口共享这个 Go 进程。
取得实例锁后，将遗留活动执行及步骤标记为 `interrupted`，将待审批记录标记为 `cancelled`，保留已保存内容及已经作出的决定；终止确认时间为本次恢复时间，不推测实际崩溃时刻，也不自动重试。新契约的未完成工具步骤同时保存独立的操作事实，已启动但无法确认的修改或外发标记为 `unknown`；旧记录不补造历史事实。
关闭时释放锁，锁文件可以保留。初始化失败和重复关闭均释放本进程拥有的资源。
