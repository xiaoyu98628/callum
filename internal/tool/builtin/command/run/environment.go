package run

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"
)

// 白名单只承载基础路径和语言环境，不继承 API Key、令牌、代理凭据或任意工具配置。
func baseEnvironment(source []string) []string {
	values := map[string]string{}
	for _, entry := range source {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			continue
		}
		if runtime.GOOS == "windows" {
			key = strings.ToUpper(key)
		}
		switch key {
		case "PATH", "HOME", "USERPROFILE", "TMPDIR", "TEMP", "TMP", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "LANG", "LC_ALL", "LC_CTYPE":
			values[key] = value
		}
	}
	// 可执行文件查找和子进程环境均不使用隐含当前目录或相对 PATH 项。
	var paths []string
	for _, path := range filepath.SplitList(values["PATH"]) {
		if filepath.IsAbs(path) {
			paths = append(paths, filepath.Clean(path))
		}
	}
	values["PATH"] = strings.Join(paths, string(filepath.ListSeparator))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

func environmentValue(environment []string, key string) string {
	for _, entry := range environment {
		name, value, _ := strings.Cut(entry, "=")
		if name == key {
			return value
		}
	}
	return ""
}

// sandboxEnvironment 将常用缓存统一定向到本次私有目录，保留平台环境变量名比较规则。
func sandboxEnvironment(environment []string, directory string) []string {
	for _, key := range []string{"TMPDIR", "TMP", "TEMP", "GOCACHE", "GOTMPDIR", "npm_config_cache", "PIP_CACHE_DIR"} {
		environment = slices.DeleteFunc(environment, func(entry string) bool {
			name, _, _ := strings.Cut(entry, "=")
			if runtime.GOOS == "windows" {
				return strings.EqualFold(name, key)
			}
			return name == key
		})
		value := directory
		switch key {
		case "GOCACHE":
			value = filepath.Join(directory, "go-build")
		case "npm_config_cache":
			value = filepath.Join(directory, "npm-cache")
		}
		environment = append(environment, key+"="+value)
	}
	return environment
}
