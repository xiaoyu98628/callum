package transport

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/stdio/protocol"
)

func TestBlockedOutputCancelsAndClosesConnection(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	}()
	connection, err := New(t.Context(), io.NopCloser(strings.NewReader("")), writer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); !errors.Is(err, errSlowConsumer) {
			t.Errorf("未保留阻塞错误: %v", err)
		}
	})
	connection.writeTimeout = 30 * time.Millisecond
	if err := connection.Send(protocol.Notification{JSONRPC: "2.0", Method: "system.ready"}); err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); !errors.Is(err, errSlowConsumer) {
		t.Fatalf("未报告阻塞: %v", err)
	}
}
