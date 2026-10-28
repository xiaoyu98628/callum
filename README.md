# Callum

Go 服务的启动装配、SQLite、日志、工作空间管理和 stdio 通信。

## 启动

需要 Go 1.27.3 或更高版本。

开发时从 Callum 仓库根目录运行：

```sh
go run ./cmd/callum
```

默认应用数据目录按构建模式选择：

| 构建方式 | 默认数据目录 |
| --- | --- |
| 普通构建，未指定 `release` 标签 | 启动工作目录下的 `.callum/` |
| 发布构建，指定 `-tags release` | 用户家目录下的 `~/.callum/` |

GoLand 开发运行时，将 Working directory 设为 Callum 仓库根目录，Go tool arguments 不添加 `-tags release`。
这样开发数据保存在仓库的 `.callum/` 下，该目录已加入 `.gitignore`。
这个目录用于保存 Callum 自身的数据，各个工作空间的项目目录仍然单独管理。

显式传入 `--data-dir` 时，以指定目录为准，覆盖两种模式的默认值。
Electron 后续也可以通过这个参数传入客户端选定的应用数据目录：

```sh
go run ./cmd/callum --data-dir /absolute/path/to/callum-data --log-level debug
```

支持的参数：

| 参数 | 默认值 | 用途 |
| --- | --- | --- |
| `--data-dir` | 开发为 `./.callum`，发布为 `~/.callum` | 数据库和日志保存位置 |
| `--log-level` | `info` | `debug`、`info`、`warn`、`error` |

应用启动后通过 stdin/stdout 处理逐行 JSON 请求，按 Ctrl+C 正常停止。
客户端关闭 stdin 时，服务也会退出并关闭数据库和日志。
原来系统用户配置目录下的 `callum` 数据会保留，不会自动迁移到新的默认目录。

## 数据库

SQLite 文件保存为 `<data-dir>/callum.db`，由 Go 服务持有。
驱动使用 [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite@v1.61.0)，构建不需要 C 工具链。

工作空间读写使用 [GORM](https://gorm.io/) `v1.31.2`，通过
[libtnb/sqlite](https://github.com/libtnb/sqlite) `v1.2.2` 适配现有的 modernc 驱动。
`internal/storage/sqlite/orm.go` 在数据库迁移完成后初始化 ORM，复用同一个连接池，
连接池仍由 App 统一关闭。ORM 初始化的版本查询使用启动上下文，
后续读写使用各个请求的上下文，不受已结束的启动上下文影响。
ORM 默认日志关闭，业务错误继续由现有的 slog 日志边界记录，stdout 仅用于 JSON 响应。
模型映射放在 SQLite 存储层，时间由业务层以 Unix 毫秒写入，关闭 ORM 自动时间赋值。
表结构继续使用下述 SQL 迁移管理，不执行 GORM 的 `AutoMigrate`。

连接初始化统一配置：

- WAL 模式、外键检查和 FULL 同步级别。
- 锁等待时间 5 秒。
- 每个进程的连接池最大打开连接数和空闲连接数均为 1。
- 连接参数应用到每个新物理连接，连接重建后仍然生效。

启动装配有 10 秒超时，数据库初始化或迁移失败会中止启动并关闭已打开的数据库。
WAL 模式下 SQLite 可能在数据库旁创建 `-wal`、`-shm` 文件，由 SQLite 自行管理。
同一台机器上的多个 Go 服务进程可以使用同一个数据目录，SQLite 自行协调跨进程读写。
写事务仍然逐个提交；等待超过 5 秒会返回锁忙错误，调用方需要处理。

## 数据库迁移

`internal/storage/sqlite/migrations/` 下的 SQL 通过 `go:embed` 嵌入程序，
打包后不需要额外分发 SQL 文件。应用装配顺序为打开连接、执行迁移、持有数据库资源。

当前迁移创建以下表：

| 表 | 用途 |
| --- | --- |
| `workspaces` | 工作空间，7 个字段，包含项目路径及置顶、归档时间 |
| `conversations` | 会话，10 个字段，包含当前模型、标题来源及消息、置顶、归档时间 |
| `schema_migrations` | 内部迁移记录，保存编号、文件名、SHA-256 校验值及执行时间 |

会话的 `workspace_id` 可以为空，支持独立聊天。删除工作空间会解除会话关联，保留会话。
`title_source` 允许为空或取 `auto`、`manual`。时间字段统一由 Go 使用 Unix 毫秒值写入；
SQL 不会自动生成标题或更新时间。会话创建、模型切换及标题生成等业务逻辑后续接入。

迁移先取得 SQLite 写锁再检查版本，使多个进程依次检查并执行。
WAL 快照升级等锁冲突会回滚整笔事务后重试，迁移最多等待 10 秒，同时受启动上下文限制。
所有待执行的 SQL 和版本记录在同一个事务中提交，失败全部回滚；
重复启动会校验已执行的记录，不会重复执行 SQL，也不会清空业务数据。

迁移使用从 bootstrap 经 app 显式传入的日志实例，输出到同一控制台和日志文件，
附带 `component=sqlite.migration`、数据库路径和进程 ID。
默认 INFO 级别记录开始检查、事务提交后的迁移文件、完成或无需更新的结果，
结果包含 `current_version`、`applied_count`、`duration_ms`。
锁冲突重试记录为 DEBUG，使用 `--log-level debug` 查看。
失败继续返回启动边界统一记录一次 ERROR，回滚的迁移不会输出提交成功日志。

增加表或字段时，新增 `002_description.sql`、`003_description.sql` 等文件，
编号从 `001` 开始连续递增，不允许重复或缺失。已经执行的 SQL 不应修改或重命名，
文件名、校验值不一致或数据库版本高于当前程序时会拒绝启动。
SQL 文件不自行执行 `BEGIN`、`COMMIT`、`ROLLBACK`，事务由迁移执行器统一管理。
同名业务表若未登记迁移记录，程序会报错并保留原数据，不自动删除或接管这些表。

## 日志

使用 `log/slog` 输出 JSON 日志，同时写入 stderr 和 `<data-dir>/logs/callum.log`。
stdout 用于进程通信的 JSON 响应。

多个 Go 进程可以共同写入这个日志文件，每条日志带有 `pid`，用于区分进程。
文件锁使用 [gofrs/flock](https://pkg.go.dev/github.com/gofrs/flock@v0.13.1)，
写入、轮转和清理都在同一把跨进程锁内执行。每次写入重新打开当前日志文件，
避免继续写入其他进程已经轮转的旧文件。

轮转规则：

- 单个文件最大 10 MiB。
- 最多保留 5 份历史文件。
- 历史文件保留最多 14 天，清理在日志文件创建或轮转时触发。

锁文件保存为 `<data-dir>/logs/callum.log.lock`，退出时保留，不要手动删除。
进程正常退出或被终止后，操作系统会释放锁，不需要清理锁文件。
获取锁最多等待 5 秒，超时或文件写入失败会向 stderr 报告，原日志也会写入 stderr。
共享目录应位于同一台机器的本地磁盘，所有写入者都需要遵守这套文件锁协议。

业务组件通过注入的 `*slog.Logger` 记录日志，用 `With` 添加模块和请求标识。
工作空间创建记录为 INFO，重复添加和列表查询记录为 DEBUG。
请求内部错误在 stdio 边界记录一次，响应不暴露数据库内部错误细节；
参数错误直接返回错误响应。进程运行错误在 bootstrap 边界记录，main 负责错误退出码。

## 装配与关闭

```text
main
  → bootstrap：解析配置、创建日志
  → app.Compose：打开数据库、执行迁移、初始化 ORM、装配工作空间服务和 stdio 处理
  → App.Run：处理请求，等待 stdin 关闭或上下文取消
  → stdio：关闭输入输出，结束正在进行的读写
  → App.Close：关闭数据库
  → bootstrap：记录停止日志、关闭日志文件
```

`internal/config` 管理配置与路径，`internal/logging` 管理日志，
`internal/storage/sqlite` 管理连接、迁移、ORM 初始化和工作空间读写，
`internal/workspace` 管理工作空间业务规则，`internal/transport/stdio` 管理通信请求和响应，
`internal/app` 负责业务装配和资源所有权。

## 工作空间与 stdio 协议

服务使用逐行 JSON 协议，每个请求和响应各占一行，当前按接收顺序处理。
请求必须是 JSON 对象，包含字符串或数字 `id`、非空字符串 `method`；
`params` 可以省略，省略时按 `{}` 处理，提供时必须是对象。
这是一套本项目的请求协议，不要求 `jsonrpc` 字段，不支持批量请求和通知。
空白行忽略；每行请求最多 1 MiB，超过限制会结束服务，避免无界读取。

| 方法 | params | 返回结果 |
| --- | --- | --- |
| `workspace.create` | 必填 `root_path`，可选 `name` | 一个工作空间对象 |
| `workspace.list` | 可选 `include_archived`，默认 `false` | 工作空间对象数组，空列表为 `[]` |

方法参数不接受未知字段。`root_path` 必须是已存在目录的绝对路径；
服务清理路径并解析符号链接，以规范化后的路径字符串去重，不创建或修改项目目录。
名称省略或仅有空白时使用目录名称。重复添加返回已有记录，不覆盖名称、置顶或归档状态。
数据库唯一约束协调多个进程的重复创建，服务不会切换全局工作目录。

列表默认隐藏归档记录，`include_archived: true` 时包含归档记录。
置顶记录优先，按置顶时间倒序，再按更新时间倒序、ID 升序排列。
当前提供创建和列表查询；置顶、归档、重命名和删除尚未提供操作入口。

### 命令行手动测试

从仓库根目录运行 `go run ./cmd/callum`，或在 GoLand 中启动服务后，
将下面的 JSON 粘贴到程序的标准输入并按回车。每条请求各占一行，服务通过标准输出返回响应。

查询工作空间列表：

```json
{"id":"2","method":"workspace.list","params":{}}
```

创建工作空间，将当前 Callum 仓库注册为工作空间：

```json
{"id":"1","method":"workspace.create","params":{"root_path":"/Users/xiaoyu/www/go/works/callum"}}
```

`root_path` 必须是本机已存在目录的绝对路径，测试其他项目时替换为对应路径。
创建会写入当前配置的数据目录中的数据库；重复提交同一路径会返回已有工作空间。
创建后再次发送上面的列表请求，即可查看结果。按 Ctrl+C 停止服务。

### 响应与错误

成功响应包含相同 `id` 和 `result`：

```json
{"id":"list-1","result":[]}
```

工作空间对象包含 `id`、`name`、`root_path`、`pinned_at`、`archived_at`、`created_at`、`updated_at`。
时间使用 Unix 毫秒，未置顶和未归档的时间字段为 `null`。

失败响应包含 `id` 和 `error`，例如：

```json
{"id":"request-1","error":{"code":"method_not_found","message":"未知方法: unknown"}}
```

错误码为 `parse_error`（JSON 无效）、`invalid_request`（请求结构无效）、
`method_not_found`（方法不存在）、`invalid_params`（业务参数无效）和 `internal_error`（内部错误）。
无法解析 JSON 或 `id` 无效时，响应的 `id` 为 `null`。
单条请求失败后服务继续处理下一条；管道读写失败则结束服务并清理资源。
没有请求级超时或取消指令；进程退出信号会取消正在处理的操作，并关闭标准输入输出以中断等待。

## 检查与构建

```sh
go vet -mod=readonly ./...
go vet -mod=readonly -tags release ./...
go test -mod=readonly -race -timeout 60s ./...
go test -mod=readonly -tags release -race -timeout 60s ./...
CGO_ENABLED=0 go build -mod=readonly -o bin/callum-dev ./cmd/callum
CGO_ENABLED=0 go build -mod=readonly -tags release -o bin/callum ./cmd/callum
```

打包流程使用带 `-tags release` 的构建命令。构建模式在编译时确定，
生成的发布程序启动时无需再传入模式参数。
