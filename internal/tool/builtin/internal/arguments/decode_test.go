package arguments_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
	"github.com/xiaoyu98628/callum/internal/tool/builtin/internal/arguments"
)

type input struct {
	Path     string   `json:"path"`
	Limit    int      `json:"limit"`
	Command  []string `json:"command"`
	Versions []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"versions"`
}

func TestDecodeRejectsAmbiguousAndMalformedParameters(t *testing.T) {
	for _, raw := range []string{
		``, `null`, `[]`, `"object"`, `{`,
		`{"path":"first","path":"last"}`, `{"path":"first","pa\u0074h":"last"}`,
		`{"Path":"other"}`, `{"path":"a","extra":1}`, `{"path":null}`, `{"path":1}`,
		`{"limit":null}`, `{"limit":true}`, `{"limit":1.5}`, `{"limit":9223372036854775808}`,
		`{"command":["go",null]}`, `{"command":[false]}`, `{"command":{}}`,
		`{"versions":[null]}`, `{"versions":[{"path":"a","path":"b"}]}`,
		`{"versions":[{"Path":"a","sha256":"x"}]}`, `{"versions":[{"path":"a","unknown":1}]}`,
		`{"versions":[{"path":null}]}`, `{"versions":[{"path":"a","sha256":3}]}`,
		`{"path":"a"} {}`, `{"path":"a"} garbage`, string([]byte{'{', '"', 'p', 'a', 't', 'h', '"', ':', '"', 255, '"', '}'}),
	} {
		t.Run(raw, func(t *testing.T) {
			var dst input
			if _, err := arguments.Decode(json.RawMessage(raw), &dst); !errors.Is(err, tool.ErrInvalid) {
				t.Fatalf("接受无效参数: %q, %v", raw, err)
			}
		})
	}
}

func TestDecodePreservesTypedValuesAndFieldPresence(t *testing.T) {
	var dst input
	fields, err := arguments.Decode(json.RawMessage(` {"path":"中文","command":["go","","&;>"],"limit":0,"versions":[{"path":"a","sha256":"abc"}]} `), &dst)
	if err != nil {
		t.Fatal(err)
	}
	if dst.Path != "中文" || len(dst.Command) != 3 || dst.Command[1] != "" || dst.Command[2] != "&;>" || len(dst.Versions) != 1 || dst.Versions[0].SHA256 != "abc" || !fields["limit"] {
		t.Fatalf("类型值或字段存在标记改变: %+v, %+v", dst, fields)
	}
	fields, err = arguments.Decode(json.RawMessage(`{}`), &input{})
	if err != nil || len(fields) != 0 {
		t.Fatalf("缺失业务必填项应由工具判断: %+v, %v", fields, err)
	}
}

func TestDecodeBoundsInputAndNesting(t *testing.T) {
	var dst input
	raw := json.RawMessage(`{"path":"` + strings.Repeat("x", tool.MaxArgumentBytes) + `"}`)
	if _, err := arguments.Decode(raw, &dst); !errors.Is(err, tool.ErrLimit) {
		t.Fatalf("未限制输入字节: %v", err)
	}
	type node struct {
		Next *node `json:"next"`
	}
	var tree node
	within := strings.Repeat(`{"next":`, 8) + `{}` + strings.Repeat(`}`, 8)
	if _, err := arguments.Decode(json.RawMessage(within), &tree); err != nil {
		t.Fatalf("合法深度被拒绝: %v", err)
	}
	tooDeep := `{"next":` + within + `}`
	if _, err := arguments.Decode(json.RawMessage(tooDeep), &tree); !errors.Is(err, tool.ErrInvalid) {
		t.Fatalf("未限制嵌套深度: %v", err)
	}
}
