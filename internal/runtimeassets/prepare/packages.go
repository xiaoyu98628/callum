package prepare

import (
	"context"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/xiaoyu98628/callum/internal/runtimeassets"
)

func packageDirectory(target Target) string {
	if target.Platform == "windows" {
		return "python/Lib/site-packages"
	}
	parts := strings.Split(target.Python.Version, ".")
	if len(parts) < 2 {
		return ""
	}
	for _, part := range parts[:2] {
		if part == "" || strings.ContainsFunc(part, func(r rune) bool { return !unicode.IsDigit(r) || r > 127 }) {
			return ""
		}
	}
	return "python/lib/python" + strings.Join(parts[:2], ".") + "/site-packages"
}

func validatePackages(target Target) error {
	if len(target.PythonPackages) == 0 {
		return nil
	}
	if len(target.PythonPackages) > 32 || packageDirectory(target) == "" {
		return errors.New("Python 依赖数量或版本无效")
	}
	seen := map[string]bool{}
	for _, dependency := range target.PythonPackages {
		parsed, err := url.Parse(dependency.URL)
		_, hashErr := hex.DecodeString(dependency.SHA256)
		if !simpleName(dependency.Name) || seen[dependency.Name] || err != nil || parsed.Scheme != "https" || parsed.Host != "files.pythonhosted.org" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || hashErr != nil || len(dependency.SHA256) != 64 || dependency.Version == "" || !simpleName(dependency.Archive) || !strings.HasSuffix(dependency.Archive, ".whl") || dependency.Prefix != "." || !safePath(dependency.Entry) {
			return errors.New("Python wheel 来源、校验值或模块入口无效")
		}
		seen[dependency.Name] = true
	}
	return nil
}

func packageManifest(target Target) []runtimeassets.PythonPackage {
	var packages []runtimeassets.PythonPackage
	for _, dependency := range target.PythonPackages {
		packages = append(packages, runtimeassets.PythonPackage{Name: dependency.Name, Version: dependency.Version, Path: packageDirectory(target) + "/" + dependency.Entry})
	}
	return packages
}

func installPackages(ctx context.Context, target Target, stage string, archives []string) error {
	if len(archives) == 0 {
		return nil
	}
	if err := validatePackages(target); err != nil {
		return err
	}
	if len(archives) != len(target.PythonPackages) {
		return errors.New("Python 依赖归档数量不一致")
	}
	directory := filepath.Join(stage, filepath.FromSlash(packageDirectory(target)))
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	for _, archive := range archives {
		if err := extractInto(ctx, archive, directory, "."); err != nil {
			return err
		}
	}
	return nil
}
