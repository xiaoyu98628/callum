package patch

import (
	"errors"
	"testing"

	"github.com/xiaoyu98628/callum/internal/tool"
)

func TestPatchTextMatchingAndLineEndings(t *testing.T) {
	for _, test := range []struct {
		name, before, body, want string
		conflict                 bool
	}{
		{"unicode", "标题\n旧内容\n结尾\n", "@@\n 标题\n-旧内容\n+新内容\n 结尾", "标题\n新内容\n结尾\n", false},
		{"crlf", "header\r\nold\r\ntail\r\n", "@@\n header\n-old\n+new\n tail", "header\r\nnew\r\ntail\r\n", false},
		{"no_final_newline", "header\nold", "@@\n header\n-old\n+new\n*** End of File", "header\nnew", false},
		{"mixed", "header\r\nold\nlast\r\n", "@@\n-old\n+new", "header\r\nnew\nlast\r\n", false},
		{"append", "a\n", "@@\n a\n+b\n*** End of File", "a\nb\n", false},
		{"empty", "", "@@\n+new", "new\n", false},
		{"clear", "a\n", "@@\n-a", "", false},
		{"delete_last_unterminated_line", "a\nb", "@@\n-b\n*** End of File", "a\n", false},
		{"ambiguous", "a\na\n", "@@\n-a\n+b", "", true},
		{"missing", "a\n", "@@\n-other\n+b", "", true},
		{"exact_whitespace", " a\n", "@@\n-a\n+b", "", true},
		{"anchor", "section\na\nnext\na\n", "@@ next\n-a\n+b", "section\na\nnext\nb\n", false},
		{"eof", "a\na\n", "@@\n-a\n+b\n*** End of File", "a\nb\n", false},
		{"multiple_hunks", "a\nb\nc\nd\n", "@@\n-a\n+A\n@@\n-d\n+D", "A\nb\nc\nD\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			files, err := parsePatch("*** Begin Patch\n*** Update File: note.txt\n" + test.body + "\n*** End Patch")
			if err != nil {
				t.Fatal(err)
			}
			out, err := applyText([]byte(test.before), files[0])
			if test.conflict {
				if !errors.Is(err, tool.ErrConflict) {
					t.Fatalf("冲突被接受: %q %v", out, err)
				}
				return
			}
			if err != nil || string(out) != test.want {
				t.Fatalf("内容或换行改变: %q want %q err=%v", out, test.want, err)
			}
		})
	}
}
func TestPatchRejectsUnsupportedOrIncompleteSyntax(t *testing.T) {
	for _, body := range []string{
		"", "*** Delete File: a", "*** Update File: a\n*** Move to: b\n@@\n-a\n+b",
		"*** Update File: a\n@@\n context", "*** Update File: a\n@@\n", "*** Add File: a\nnot-prefixed",
		"*** Add File: ../escape\n+x", "*** Add File: a\n+x\n*** Add File: a\n+y", "*** Add File: a\n+\x00",
	} {
		if _, err := parsePatch("*** Begin Patch\n" + body + "\n*** End Patch"); err == nil {
			t.Fatalf("接受了无效补丁: %q", body)
		}
	}
}
