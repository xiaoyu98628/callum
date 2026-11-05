// Package settings 定义应用设置、执行快照及持久化契约。
package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool/policy"
)

const ExecutionKey = "execution"
const MaxValueBytes = 16 << 10

var (
	ErrInvalid  = errors.New("应用设置无效")
	ErrNotFound = errors.New("应用设置不存在")
	ErrConflict = errors.New("应用设置版本已变化")
	ErrStorage  = errors.New("应用设置存储失败")
)

// Record 保存一个配置组；Value 不包含凭据，不能直接接受任意未注册配置。
type Record struct {
	ID        string          `json:"id"`
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	Revision  int64           `json:"revision"`
	CreatedAt int64           `json:"created_at"`
	UpdatedAt int64           `json:"updated_at"`
}

// Execution 是当前支持的执行设置，新增 JSON 字段时同步定义默认值和校验。
type Execution struct {
	ApprovalPolicy    policy.ApprovalPolicy `json:"approval_policy"`
	ApprovalsReviewer policy.Reviewer       `json:"approvals_reviewer"`
}

// DefaultExecution 只在创建设置记录时使用，读取失败不能回退到默认权限。
func DefaultExecution() Execution {
	return Execution{ApprovalPolicy: policy.OnRequest, ApprovalsReviewer: policy.User}
}

// Snapshot 固定一次执行实际采用的配置及版本，不能跟随全局设置更新。
type Snapshot struct {
	Revision int64 `json:"revision"`
	Execution
	Compaction *CompactionSnapshot `json:"context_compaction,omitempty"`
}

// Valid 判断快照是否具有有效版本和当前服务支持的独立配置值。
func (s Snapshot) Valid() bool {
	return s.Revision > 0 && s.ApprovalPolicy.Valid() && s.ApprovalsReviewer.Valid() &&
		(s.Compaction == nil || s.Compaction.Valid())
}

// Clone 复制可变嵌套快照，受理回调不得修改即将执行的配置。
func (s Snapshot) Clone() Snapshot {
	if s.Compaction != nil {
		value := *s.Compaction
		s.Compaction = &value
	}
	return s
}

// DecodeExecution 严格校验客户端或存储中的 JSON；不接受重复键、未知字段或尾随内容。
func DecodeExecution(raw json.RawMessage) (Execution, error) {
	var value Execution
	if len(raw) == 0 || len(raw) > MaxValueBytes || !utf8.Valid(raw) {
		return value, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return value, ErrInvalid
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return value, ErrInvalid
		}
		seen[key] = true
		switch key {
		case "approval_policy":
			err = decoder.Decode(&value.ApprovalPolicy)
		case "approvals_reviewer":
			err = decoder.Decode(&value.ApprovalsReviewer)
		default:
			return value, ErrInvalid
		}
		if err != nil {
			return value, ErrInvalid
		}
	}
	if _, err := decoder.Token(); err != nil {
		return value, ErrInvalid
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || !value.ApprovalPolicy.Valid() || !value.ApprovalsReviewer.Valid() {
		return value, ErrInvalid
	}
	return value, nil
}

// ExecutionSnapshot 用于受理事务和查询验证；非法存储不能被当作默认设置。
func ExecutionSnapshot(record Record) (Snapshot, error) {
	value, err := DecodeExecution(record.Value)
	if err != nil || record.Key != ExecutionKey || record.Revision <= 0 {
		return Snapshot{}, errors.Join(ErrStorage, ErrInvalid)
	}
	return Snapshot{Revision: record.Revision, Execution: value}, nil
}

// Repository 的 Update 必须以期望版本原子更新，冲突时不得改动设置。
// Get 不启动事务，允许仓储使用受理动作传入的同一事务读取配置。
type Repository interface {
	Get(context.Context, string) (Record, error)
	Update(context.Context, string, int64, json.RawMessage) (Record, error)
}
