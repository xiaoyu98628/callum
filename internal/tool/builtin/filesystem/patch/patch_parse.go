package patch

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool"
)

const maxPatchFiles = 8

// patchFile 只描述创建和修改，删除和移动标记不能退化成其他操作。
type patchFile struct {
	path, action, content string
	hunks                 []patchHunk
	removedUpper          int64
}
type patchHunk struct {
	anchor string
	lines  []string
	end    bool
}

func canonicalPatch(files []patchFile) string {
	var out strings.Builder
	out.WriteString("*** Begin Patch\n")
	for _, file := range files {
		if file.action == "create" {
			out.WriteString("*** Add File: " + file.path + "\n")
			if file.content != "" {
				for line := range strings.SplitSeq(strings.TrimSuffix(file.content, "\n"), "\n") {
					out.WriteString("+" + line + "\n")
				}
			}
		} else {
			out.WriteString("*** Update File: " + file.path + "\n")
			for _, h := range file.hunks {
				out.WriteString("@@")
				if h.anchor != "" {
					out.WriteString(" " + h.anchor)
				}
				out.WriteByte('\n')
				for _, line := range h.lines {
					out.WriteString(line)
					out.WriteByte('\n')
				}
				if h.end {
					out.WriteString("*** End of File\n")
				}
			}
		}
	}
	out.WriteString("*** End Patch\n")
	return out.String()
}

func parsePatch(value string) ([]patchFile, error) {
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) || len(value) > tool.MaxArgumentBytes {
		return nil, tool.ErrInvalid
	}
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) < 3 || lines[0] != "*** Begin Patch" || lines[len(lines)-1] != "*** End Patch" {
		return nil, tool.ErrInvalid
	}
	var files []patchFile
	seen := map[string]bool{}
	for i := 1; i < len(lines)-1; {
		line := lines[i]
		f := patchFile{}
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			f.path, f.action = strings.TrimPrefix(line, "*** Add File: "), "create"
		case strings.HasPrefix(line, "*** Update File: "):
			f.path, f.action = strings.TrimPrefix(line, "*** Update File: "), "update"
		default:
			return nil, tool.ErrInvalid
		}
		path, err := patchPath(f.path)
		if err != nil || seen[path] {
			return nil, tool.ErrInvalid
		}
		f.path = path
		seen[path] = true
		i++
		if f.action == "create" {
			var content strings.Builder
			for i < len(lines)-1 && strings.HasPrefix(lines[i], "+") {
				content.WriteString(lines[i][1:])
				content.WriteByte('\n')
				i++
			}
			f.content = content.String() // 空文件允许没有正文行。
		} else {
			for i < len(lines)-1 && (lines[i] == "@@" || strings.HasPrefix(lines[i], "@@ ")) {
				h := patchHunk{anchor: strings.TrimPrefix(lines[i], "@@")}
				h.anchor = strings.TrimPrefix(h.anchor, " ")
				i++
				changed := false
				for i < len(lines)-1 && len(lines[i]) > 0 && (lines[i][0] == ' ' || lines[i][0] == '+' || lines[i][0] == '-') {
					v := lines[i]
					h.lines = append(h.lines, v)
					changed = changed || v[0] != ' '
					if v[0] == '-' {
						f.removedUpper += int64(len(v) - 1 + 2)
					}
					i++
				}
				if !changed {
					return nil, tool.ErrInvalid
				}
				if i < len(lines)-1 && lines[i] == "*** End of File" {
					h.end = true
					i++
				}
				f.hunks = append(f.hunks, h)
				if h.end {
					break
				}
			}
			if len(f.hunks) == 0 {
				return nil, tool.ErrInvalid
			}
		}
		files = append(files, f)
		if len(files) > maxPatchFiles {
			return nil, tool.ErrLimit
		}
	}
	if len(files) == 0 {
		return nil, tool.ErrInvalid
	}
	return files, nil
}

type textLine struct {
	text, ending string
	inserted     bool
}

func splitText(data []byte) []textLine {
	var lines []textLine
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			lines = append(lines, textLine{text: string(data)})
			break
		}
		text, ending := data[:i], "\n"
		if len(text) > 0 && text[len(text)-1] == '\r' {
			text = text[:len(text)-1]
			ending = "\r\n"
		}
		lines = append(lines, textLine{text: string(text), ending: ending})
		data = data[i+1:]
	}
	return lines
}

// applyText 只接受唯一的精确匹配，不做空白、大小写或标点模糊匹配。
// 未修改的行保留原始换行；插入行沿用邻近换行，文件尾原来无换行时仍保持。
func applyText(data []byte, file patchFile) ([]byte, error) {
	original := splitText(data)
	ending := "\n"
	for _, line := range original {
		if line.ending != "" {
			ending = line.ending
			break
		}
	}
	var out []textLine
	cursor := 0
	for _, h := range file.hunks {
		start := cursor
		if h.anchor != "" {
			found := -1
			for i := cursor; i < len(original); i++ {
				if original[i].text == h.anchor {
					if found >= 0 {
						return nil, tool.ErrConflict
					}
					found = i
				}
			}
			if found < 0 {
				return nil, tool.ErrConflict
			}
			start = found + 1
		}
		var old []string
		for _, line := range h.lines {
			if line[0] != '+' {
				old = append(old, line[1:])
			}
		}
		match := -1
		if len(old) == 0 {
			switch {
			case h.end:
				match = len(original)
			case len(original) == 0:
				match = 0
			case h.anchor != "":
				match = start
			default:
				return nil, tool.ErrConflict
			}
		} else {
			for i := start; i+len(old) <= len(original); i++ {
				if h.end && i+len(old) != len(original) {
					continue
				}
				same := true
				for j, v := range old {
					if original[i+j].text != v {
						same = false
						break
					}
				}
				if same {
					if match >= 0 {
						return nil, tool.ErrConflict
					}
					match = i
				}
			}
		}
		if match < cursor {
			return nil, tool.ErrConflict
		}
		out = append(out, original[cursor:match]...)
		style := ending
		if match < len(original) && original[match].ending != "" {
			style = original[match].ending
		}
		pos := match
		for _, line := range h.lines {
			switch line[0] {
			case ' ':
				out = append(out, original[pos])
				pos++
			case '-':
				pos++
			case '+':
				out = append(out, textLine{text: line[1:], ending: style, inserted: true})
			}
		}
		cursor = pos
	}
	out = append(out, original[cursor:]...)
	if len(original) > 0 && len(out) > 0 && original[len(original)-1].ending == "" && out[len(out)-1].inserted {
		out[len(out)-1].ending = ""
	}
	var result bytes.Buffer
	for i, line := range out {
		result.WriteString(line.text)
		if line.ending == "" && i < len(out)-1 {
			result.WriteString(ending)
		} else {
			result.WriteString(line.ending)
		}
		if result.Len() > 1<<20 {
			return nil, tool.ErrLimit
		}
	}
	if bytes.Equal(data, result.Bytes()) {
		return nil, tool.ErrInvalid
	}
	return result.Bytes(), nil
}
