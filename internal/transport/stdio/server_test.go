package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyu98628/callum/internal/workspace"
)

type fakeWorkspaces struct {
	created workspace.CreateInput
	options workspace.ListOptions
	err     error
}

func (s *fakeWorkspaces) Create(_ context.Context, input workspace.CreateInput) (workspace.Workspace, error) {
	s.created = input
	return workspace.Workspace{ID: "workspace-1", Name: input.Name, RootPath: input.RootPath}, s.err
}

func (s *fakeWorkspaces) List(_ context.Context, options workspace.ListOptions) ([]workspace.Workspace, error) {
	s.options = options
	return []workspace.Workspace{}, s.err
}

type bufferOutput struct {
	bytes.Buffer
	closed   bool
	closeErr error
	writeErr error
}

func (w *bufferOutput) Write(data []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.Buffer.Write(data)
}

func (w *bufferOutput) Close() error {
	w.closed = true
	return w.closeErr
}

type testInput struct {
	io.Reader
	closed   bool
	closeErr error
}

func (r *testInput) Close() error {
	r.closed = true
	return r.closeErr
}

func testServer(t *testing.T, service WorkspaceService) *Server {
	t.Helper()
	server, err := NewServer(service, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func TestServerRequestsAndRecovery(t *testing.T) {
	cases := []struct {
		line string
		id   string
		code string
	}{
		{`{"id":0,"method":"workspace.create","params":{"root_path":"/project","name":"项目"}}`, "0", ""},
		{`{"id":"list","method":"workspace.list","params":{"include_archived":true}}`, `"list"`, ""},
		{`{"id":9007199254740993,"method":"workspace.list"}`, "9007199254740993", ""},
		{`{`, "null", "parse_error"},
		{`null`, "null", "invalid_request"},
		{`[]`, "null", "invalid_request"},
		{`{"method":"workspace.list"}`, "null", "invalid_request"},
		{`{"id":true,"method":"workspace.list"}`, "null", "invalid_request"},
		{`{"id":"bad-method","method":12}`, `"bad-method"`, "invalid_request"},
		{`{"id":"unknown","method":"unknown"}`, `"unknown"`, "method_not_found"},
		{`{"id":"array","method":"workspace.list","params":[]}`, `"array"`, "invalid_params"},
		{`{"id":"null","method":"workspace.list","params":null}`, `"null"`, "invalid_params"},
		{`{"id":"field","method":"workspace.list","params":{"typo":true}}`, `"field"`, "invalid_params"},
		{`{"id":"type","method":"workspace.list","params":{"include_archived":"yes"}}`, `"type"`, "invalid_params"},
		{`{"id":"after-errors","method":"workspace.list"}`, `"after-errors"`, ""},
	}
	lines := []string{"", "  "}
	for _, item := range cases {
		lines = append(lines, item.line)
	}
	service := &fakeWorkspaces{}
	input := &testInput{Reader: strings.NewReader(strings.Join(lines, "\n"))}
	output := &bufferOutput{}
	if err := testServer(t, service).Run(t.Context(), input, output); err != nil {
		t.Fatal(err)
	}
	if !input.closed || !output.closed || service.created.Name != "项目" {
		t.Fatalf("streams or request dispatch: input=%v, output=%v, create=%+v", input.closed, output.closed, service.created)
	}
	decoder := json.NewDecoder(&output.Buffer)
	for _, item := range cases {
		var reply struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *responseError  `json:"error"`
		}
		if err := decoder.Decode(&reply); err != nil {
			t.Fatal(err)
		}
		if string(reply.ID) != item.id {
			t.Fatalf("id changed for %s: %s", item.line, reply.ID)
		}
		if item.code != "" {
			if reply.Error == nil || reply.Error.Code != item.code || len(reply.Result) != 0 {
				t.Fatalf("unexpected error response for %s: %+v", item.line, reply)
			}
		} else if reply.Error != nil || len(reply.Result) == 0 {
			t.Fatalf("unexpected success response for %s: %+v", item.line, reply)
		}
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		t.Fatalf("extra data on stdout: %v", err)
	}
}

func TestServerBusinessErrors(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		var logs bytes.Buffer
		service := &fakeWorkspaces{err: errors.New("private database detail")}
		code := "internal_error"
		if invalid {
			service.err, code = workspace.ErrInvalidInput, "invalid_params"
		}
		server, err := NewServer(service, slog.New(slog.NewJSONHandler(&logs, nil)))
		if err != nil {
			t.Fatal(err)
		}
		output := &bufferOutput{}
		request := `{"id":"request-1","method":"workspace.create","params":{"root_path":"/project"}}`
		if err := server.Run(t.Context(), io.NopCloser(strings.NewReader(request)), output); err != nil {
			t.Fatal(err)
		}
		var reply response
		if err := json.Unmarshal(output.Bytes(), &reply); err != nil {
			t.Fatal(err)
		}
		if reply.Error == nil || reply.Error.Code != code || strings.Contains(output.String(), "private database detail") {
			t.Fatalf("unexpected business error response: %s", output.String())
		}
		if !invalid && (strings.Count(logs.String(), `"level":"ERROR"`) != 1 ||
			!strings.Contains(logs.String(), `"request_id":"request-1"`) ||
			!strings.Contains(logs.String(), `"component":"stdio"`)) {
			t.Fatalf("error boundary or request context missing: %s", logs.String())
		}
		if invalid && logs.Len() != 0 {
			t.Fatalf("parameter error logged as internal failure: %s", logs.String())
		}
	}
}

type notifyingInput struct {
	*io.PipeReader
	started chan struct{}
}

func (r *notifyingInput) Read(data []byte) (int, error) {
	close(r.started)
	return r.PipeReader.Read(data)
}

type notifyingOutput struct {
	*io.PipeWriter
	started chan struct{}
}

func (w *notifyingOutput) Write(data []byte) (int, error) {
	close(w.started)
	return w.PipeWriter.Write(data)
}

func awaitStarted(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("I/O did not start")
	}
}

func TestServerCancellationUnblocksIO(t *testing.T) {
	for _, blockedWrite := range []bool{false, true} {
		t.Run(fmtBool(blockedWrite), func(t *testing.T) {
			reader, writer := io.Pipe()
			t.Cleanup(func() {
				if err := reader.Close(); err != nil {
					t.Error(err)
				}
				if err := writer.Close(); err != nil {
					t.Error(err)
				}
			})
			started := make(chan struct{})
			var input io.ReadCloser = &notifyingInput{PipeReader: reader, started: started}
			var output io.WriteCloser = &bufferOutput{}
			if blockedWrite {
				input = io.NopCloser(strings.NewReader(`{"id":1,"method":"workspace.list"}` + "\n"))
				output = &notifyingOutput{PipeWriter: writer, started: started}
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			server := testServer(t, &fakeWorkspaces{})
			go func() { result <- server.Run(ctx, input, output) }()
			awaitStarted(t, started)
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("lost cancellation: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("cancelled I/O goroutine did not stop")
			}
		})
	}
}

func fmtBool(write bool) string {
	if write {
		return "write"
	}
	return "read"
}

func TestServerIOAndCleanupErrors(t *testing.T) {
	writeErr, closeErr := errors.New("write failure"), errors.New("close failure")
	input := &testInput{Reader: strings.NewReader(`{"id":1,"method":"workspace.list"}`), closeErr: closeErr}
	output := &bufferOutput{writeErr: writeErr}
	err := testServer(t, &fakeWorkspaces{}).Run(t.Context(), input, output)
	if !errors.Is(err, writeErr) || !errors.Is(err, closeErr) || !input.closed || !output.closed {
		t.Fatalf("lost I/O or cleanup failure: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	input = &testInput{Reader: strings.NewReader(""), closeErr: closeErr}
	err = testServer(t, &fakeWorkspaces{}).Run(ctx, input, &bufferOutput{})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, closeErr) || err == context.Canceled {
		t.Fatalf("cancellation hid cleanup failure: %v", err)
	}
	input = &testInput{Reader: strings.NewReader(strings.Repeat("x", maxRequestBytes+2))}
	output = &bufferOutput{}
	err = testServer(t, &fakeWorkspaces{}).Run(t.Context(), input, output)
	if err == nil || !input.closed || !output.closed {
		t.Fatalf("oversized frame did not terminate and clean up: %v", err)
	}
}
