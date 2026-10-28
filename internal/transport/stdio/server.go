package stdio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/xiaoyu98628/callum/internal/workspace"
)

const maxRequestBytes = 1024 * 1024

// WorkspaceService 是通信处理所需的业务能力，通信层不直接访问数据库。
type WorkspaceService interface {
	Create(context.Context, workspace.CreateInput) (workspace.Workspace, error)
	List(context.Context, workspace.ListOptions) ([]workspace.Workspace, error)
}

// Server 顺序处理逐行 JSON 请求，每个请求均通过 id 关联一条响应。
type Server struct {
	workspaces WorkspaceService
	logger     *slog.Logger
}

type response struct {
	ID     json.RawMessage `json:"id"`
	Result any             `json:"result,omitempty"`
	Error  *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewServer 复用应用装配的业务服务和日志，不创建额外的存储资源。
func NewServer(workspaces WorkspaceService, logger *slog.Logger) (*Server, error) {
	if workspaces == nil || logger == nil {
		return nil, errors.New("stdio 工作空间服务和日志不能为空")
	}
	return &Server{workspaces: workspaces, logger: logger.With("component", "stdio")}, nil
}

// Run 接管输入和输出的关闭责任。它们的 Close 必须能够中断正在进行的读写。
// EOF 正常结束；取消时关闭管道，等待关闭回调完成后才返回，不遗留后台读写任务。
func (s *Server) Run(ctx context.Context, input io.ReadCloser, output io.WriteCloser) (runErr error) {
	if ctx == nil || input == nil || output == nil {
		return errors.New("stdio 上下文、输入和输出不能为空")
	}
	var closeOnce sync.Once
	var closeErr error
	closeStreams := func() {
		closeOnce.Do(func() { closeErr = errors.Join(input.Close(), output.Close()) })
	}
	stopClose := context.AfterFunc(ctx, closeStreams)
	defer func() {
		stopClose()
		// sync.Once 等待已经开始的关闭操作，保证清理错误已经写入。
		closeStreams()
		if closeErr != nil {
			runErr = errors.Join(runErr, closeErr)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), maxRequestBytes+1)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		reply := s.handle(ctx, line)
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := encoder.Encode(reply); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("写入 stdio 响应失败: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取 stdio 请求失败（单行上限 1 MiB）: %w", err)
	}
	return nil
}

func (s *Server) handle(ctx context.Context, line []byte) response {
	id := json.RawMessage("null")
	if !json.Valid(line) {
		return failure(id, "parse_error", "请求不是有效 JSON")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil || fields == nil {
		return failure(id, "invalid_request", "请求必须是 JSON 对象")
	}
	var requestID any
	decoder := json.NewDecoder(bytes.NewReader(fields["id"]))
	decoder.UseNumber()
	if err := decoder.Decode(&requestID); err != nil {
		return failure(id, "invalid_request", "id 必须是字符串或数字")
	}
	switch requestID.(type) {
	case string, json.Number:
		id = fields["id"]
	default:
		return failure(id, "invalid_request", "id 必须是字符串或数字")
	}
	var method string
	if err := json.Unmarshal(fields["method"], &method); err != nil || method == "" {
		return failure(id, "invalid_request", "method 必须是非空字符串")
	}
	logger := s.logger.With("request_id", requestID, "method", method)
	var result any
	var err error
	switch method {
	case "workspace.create":
		var input workspace.CreateInput
		if err := decodeParams(fields["params"], &input); err != nil {
			return failure(id, "invalid_params", err.Error())
		}
		result, err = s.workspaces.Create(ctx, input)
	case "workspace.list":
		var options workspace.ListOptions
		if err := decodeParams(fields["params"], &options); err != nil {
			return failure(id, "invalid_params", err.Error())
		}
		result, err = s.workspaces.List(ctx, options)
	default:
		return failure(id, "method_not_found", "未知方法: "+method)
	}
	if err != nil {
		if errors.Is(err, workspace.ErrInvalidInput) {
			return failure(id, "invalid_params", err.Error())
		}
		// 请求处理是业务错误的记录边界；取消和通信错误交由启动边界处理。
		if ctx.Err() == nil {
			logger.ErrorContext(ctx, "stdio 请求处理失败", "error", err)
		}
		return failure(id, "internal_error", "请求处理失败，请查看服务日志")
	}
	logger.DebugContext(ctx, "stdio 请求处理完成")
	return response{ID: id, Result: result}
}

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("params 必须是 JSON 对象")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("params 无效: %w", err)
	}
	return nil
}

func failure(id json.RawMessage, code, message string) response {
	return response{ID: id, Error: &responseError{Code: code, Message: message}}
}
