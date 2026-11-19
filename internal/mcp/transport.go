package mcp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xiaoyu98628/callum/internal/settings"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type boundedBody struct {
	io.ReadCloser
	remaining int
}

func (b *boundedBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, io.ErrShortBuffer
	}
	if len(p) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= n
	if b.remaining < 0 {
		return 0, errors.New("MCP 响应过大")
	}
	return n, err
}
func limitBody(r io.ReadCloser) io.ReadCloser { return &boundedBody{ReadCloser: r, remaining: 1 << 20} }

// HTTPClient 为持久 SSE 流保留调用方取消，限制连接和响应头阶段。
func HTTPClient(discovered bool) (*http.Client, *http.Transport) {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = http.ProxyFromEnvironment
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	if discovered {
		// 授权服务地址由远程元数据提供。校验实际拨号地址，避免 DNS 变化后访问内网。
		t.Proxy = nil
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrOAuth
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return ErrOAuth
			}
			ip = ip.Unmap()
			if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || netip.MustParsePrefix("100.64.0.0/10").Contains(ip) {
				return ErrOAuth
			}
			return nil
		}
	}
	t.DialContext = dialer.DialContext
	t.TLSHandshakeTimeout = 5 * time.Second
	t.ResponseHeaderTimeout = 8 * time.Second
	t.MaxConnsPerHost = 24
	t.MaxIdleConns = 32
	return &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, t
}

type processTransport struct {
	cfg     settings.MCPServer
	secrets map[string]string
}

func (t *processTransport) Connect(ctx context.Context) (sdk.Connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.Command(t.cfg.Command, t.cfg.Args...)
	cmd.Dir = t.cfg.Directory
	if cmd.Dir == "" {
		cmd.Dir = os.TempDir()
	}
	cmd.Env = processEnvironment(t.cfg.Env, t.secrets)
	readIn, writeIn, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	readOut, writeOut, err := os.Pipe()
	if err != nil {
		return nil, errors.Join(err, readIn.Close(), writeIn.Close())
	}
	cmd.Stdin, cmd.Stdout = readIn, writeOut
	// 第三方 stderr 可包含令牌，沿用 os/exec 默认丢弃。
	stop, err := startProcess(cmd)
	parentErr := errors.Join(readIn.Close(), writeOut.Close())
	if err != nil {
		return nil, errors.Join(err, parentErr, writeIn.Close(), readOut.Close())
	}
	p := &childProcess{in: writeIn, out: readOut, stop: stop, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(p.done) }()
	if parentErr != nil {
		return nil, errors.Join(parentErr, p.Close())
	}
	transport := sdk.IOTransport{Reader: p, Writer: p, MaxLineLength: 1 << 20}
	conn, err := transport.Connect(ctx)
	if err != nil {
		return nil, errors.Join(err, p.Close())
	}
	return conn, nil
}

type childProcess struct {
	in, out *os.File
	stop    func() error
	done    chan struct{}
	once    sync.Once
	err     error
}

func (p *childProcess) Read(b []byte) (int, error)  { return p.out.Read(b) }
func (p *childProcess) Write(b []byte) (int, error) { return p.in.Write(b) }
func (p *childProcess) Close() error {
	p.once.Do(func() {
		inputErr := p.in.Close()
		select {
		case <-p.done:
		case <-time.After(250 * time.Millisecond):
		}
		p.err = errors.Join(inputErr, p.out.Close(), p.stop())
		<-p.done
	})
	return p.err
}
