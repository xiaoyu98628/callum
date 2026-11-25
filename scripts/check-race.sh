#!/bin/sh
set -eu
set -f

project_root=$(CDPATH='' cd "$(dirname "$0")/.." && pwd)
cd "$project_root"

# 这些用例有长工具链、大响应或旧轨迹恢复，分别运行以减少相互争抢资源。
isolated_tests='
TestLargeModelResultUsesBoundedUTF8Fragments
TestSingleExecutionCompactsMessageBoundaryAndRestoresAfterRestart
TestLegacyOversizedTraceIsCompactedBeforeRestore
TestAutomaticCompactionSurvivesLongHistoryAndKeepsConversationsIsolated
'

app_package=$(go list -mod=readonly ./internal/app)
packages=$(go list -mod=readonly ./...)
app_tests=$(go test -mod=readonly -race -list '^Test' ./internal/app)
isolated_pattern=''
for test_name in $isolated_tests; do
    if ! printf '%s\n' "$app_tests" | grep -Fxq "$test_name"; then
        printf '隔离用例不存在：%s\n' "$test_name" >&2
        exit 1
    fi
    isolated_pattern="${isolated_pattern:+$isolated_pattern|}$test_name"
done

run_group() {
    group=$1
    shift
    started=$(date +%s)
    printf '\n开始 %s\n' "$group"
    if go test -mod=readonly -race -count=1 "$@"; then
        printf '通过 %s (%ss)\n' "$group" "$(( $(date +%s) - started ))"
    else
        status=$?
        printf '失败 %s (%ss)\n' "$group" "$(( $(date +%s) - started ))" >&2
        return "$status"
    fi
}

set --
for package in $packages; do
    if [ "$package" != "$app_package" ]; then
        set -- "$@" "$package"
    fi
done
run_group '其他 Go 包' -p=4 -parallel=4 -timeout=60s -run=. -skip= "$@"
run_group '常规应用回归' -parallel=3 -timeout=120s -run=. -skip="^($isolated_pattern)$" ./internal/app
for test_name in $isolated_tests; do
    run_group "$test_name" -parallel=1 -timeout=60s -run="^$test_name$" -skip= ./internal/app
done
