//go:build !release

package config

// 普通构建、go run 和未设置 release 标签的 IDE 调试使用开发目录。
const releaseBuild = false
