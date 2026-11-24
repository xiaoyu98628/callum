package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xiaoyu98628/callum/internal/settings"
)

type closeErrorTransport struct {
	sdk.Transport
	err error
}

func (t closeErrorTransport) Connect(ctx context.Context) (sdk.Connection, error) {
	conn, err := t.Transport.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return closeErrorConnection{Connection: conn, err: t.err}, nil
}

type closeErrorConnection struct {
	sdk.Connection
	err error
}

func (c closeErrorConnection) Close() error {
	return errors.Join(c.Connection.Close(), c.err)
}

func TestCloseConnectionReportsOnlyUnexpectedFailures(t *testing.T) {
	const private = "受控第三方错误原文"
	for _, tc := range []struct {
		name   string
		err    error
		kind   string
		retire bool
	}{
		{name: "正常关闭"},
		{name: "预期取消", err: fmt.Errorf("传输结束: %w", context.Canceled), retire: true},
		{name: "旧会话失效", err: errors.Join(ErrChanged, sdk.ErrSessionMissing)},
		{name: "重连关闭失败", err: errors.New(private), kind: "close_failed"},
		{name: "退出清理失败", err: fmt.Errorf("清理: %w", errors.Join(context.Canceled, errors.New(private))), kind: "close_failed", retire: true},
		{name: "关闭超时", err: context.DeadlineExceeded, kind: "close_timeout", retire: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			clientTransport, serverTransport := sdk.NewInMemoryTransports()
			server := sdk.NewServer(&sdk.Implementation{Name: "fixture", Version: "1"}, nil)
			peer, err := server.Connect(ctx, serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := peer.Close(); err != nil {
					t.Error(err)
				}
			}()
			client := sdk.NewClient(&sdk.Implementation{Name: "fixture", Version: "1"}, nil)
			session, err := client.Connect(ctx, closeErrorTransport{Transport: clientTransport, err: tc.err}, nil)
			if err != nil {
				t.Fatal(err)
			}

			var logs bytes.Buffer
			s := &Service{ctx: ctx, cancel: cancel, entries: make(map[string]*entry),
				logger: slog.New(slog.NewJSONHandler(&logs, nil)).With("module", "mcp")}
			const serverID = "fixture-server"
			s.add(settings.MCPServer{ID: serverID})
			e := s.entries[serverID]
			e.connection = &connection{session: session}
			e.publicConnection.Store(e.connection)
			if !tc.retire {
				if err := e.acquire(ctx); err != nil {
					t.Fatal(err)
				}
				s.closeConnection(e)
				e.release()
			}
			s.Close()
			s.Close()

			if e.connection != nil || e.publicConnection.Load() != nil {
				t.Fatal("关闭后仍保留旧连接")
			}
			if tc.kind == "" {
				if logs.Len() != 0 {
					t.Fatalf("预期关闭产生了诊断: %s", logs.String())
				}
				return
			}
			if strings.Contains(logs.String(), private) {
				t.Fatal("诊断包含第三方错误原文")
			}
			var entry struct {
				Level    string `json:"level"`
				Module   string `json:"module"`
				ServerID string `json:"server_id"`
				Kind     string `json:"kind"`
			}
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatalf("关闭失败应产生一条结构化诊断: %v", err)
			}
			if entry.Level != "WARN" || entry.Module != "mcp" || entry.ServerID != serverID || entry.Kind != tc.kind {
				t.Fatalf("关闭诊断字段错误: %+v", entry)
			}
		})
	}
}
