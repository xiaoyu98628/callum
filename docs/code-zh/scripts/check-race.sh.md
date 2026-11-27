```sh
#!/bin/sh
set -eu
set -f

项目根目录=$(CDPATH='' cd "$(dirname "$0")/.." && pwd)
cd "$项目根目录"

# 这些用例有长工具链、大响应或旧轨迹恢复，分别运行以减少相互争抢资源。
独立用例='
TestLargeModelResultUsesBoundedUTF8Fragments
TestSingleExecutionCompactsMessageBoundaryAndRestoresAfterRestart
TestLegacyOversizedTraceIsCompactedBeforeRestore
TestAutomaticCompactionSurvivesLongHistoryAndKeepsConversationsIsolated
'

应用包=$(go list -mod=readonly ./internal/app)
包列表=$(go list -mod=readonly ./...)
应用测试=$(go test -mod=readonly -race -list '^Test' ./internal/app)
独立用例模式=''
for 测试名称 in $独立用例; do
    if ! printf '%s\n' "$应用测试" | grep -Fxq "$测试名称"; then
        printf '隔离用例不存在：%s\n' "$测试名称" >&2
        exit 1
    fi
    独立用例模式="${独立用例模式:+$独立用例模式|}$测试名称"
done

运行分组() {
    分组=$1
    shift
    开始时间=$(date +%s)
    printf '\n开始 %s\n' "$分组"
    if go test -mod=readonly -race -count=1 "$@"; then
        printf '通过 %s (%ss)\n' "$分组" "$(( $(date +%s) - 开始时间 ))"
    else
        退出状态=$?
        printf '失败 %s (%ss)\n' "$分组" "$(( $(date +%s) - 开始时间 ))" >&2
        return "$退出状态"
    fi
}

set --
for 包名 in $包列表; do
    if [ "$包名" != "$应用包" ]; then
        set -- "$@" "$包名"
    fi
done
运行分组 '其他 Go 包' -p=4 -parallel=4 -timeout=60s -run=. -skip= "$@"
运行分组 '常规应用回归' -parallel=3 -timeout=120s -run=. -skip="^($独立用例模式)$" ./internal/app
for 测试名称 in $独立用例; do
    运行分组 "$测试名称" -parallel=1 -timeout=60s -run="^$测试名称$" -skip= ./internal/app
done
```
