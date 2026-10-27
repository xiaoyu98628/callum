# Callum

Go 应用的启动装配、SQLite 连接和日志管理。

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

应用启动后等待退出信号，按 Ctrl+C 正常停止。
原来系统用户配置目录下的 `callum` 数据会保留，不会自动迁移到新的默认目录。

## 数据库

SQLite 文件保存为 `<data-dir>/callum.db`，由 Go 服务持有。
驱动使用 [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite@v1.61.0)，构建不需要 C 工具链。

连接初始化统一配置：

- WAL 模式、外键检查和 FULL 同步级别。
- 锁等待时间 5 秒。
- 每个进程的连接池最大打开连接数和空闲连接数均为 1。
- 连接参数应用到每个新物理连接，连接重建后仍然生效。

启动装配有 10 秒超时，数据库初始化失败会中止启动。
这里只建立数据库连接，不创建工作空间、对话或消息表。
WAL 模式下 SQLite 可能在数据库旁创建 `-wal`、`-shm` 文件，由 SQLite 自行管理。
同一台机器上的多个 Go 服务进程可以使用同一个数据目录，SQLite 自行协调跨进程读写。
写事务仍然逐个提交；等待超过 5 秒会返回锁忙错误，调用方需要处理。

## 日志

使用 `log/slog` 输出 JSON 日志，同时写入 stderr 和 `<data-dir>/logs/callum.log`。
stdout 保留给后续的进程通信协议。

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

业务组件以后通过注入的 `*slog.Logger` 记录日志，可用 `With` 添加模块和请求标识。
运行错误在 bootstrap 边界统一记录，main 负责错误退出码。

## 装配与关闭

```text
main
  → bootstrap：解析配置、创建日志
  → app.Compose：打开数据库
  → App.Run：等待取消
  → App.Close：关闭数据库
  → bootstrap：记录停止日志、关闭日志文件
```

`internal/config` 管理配置与路径，`internal/logging` 管理日志，
`internal/storage/sqlite` 管理连接，`internal/app` 负责业务装配和资源所有权。

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
