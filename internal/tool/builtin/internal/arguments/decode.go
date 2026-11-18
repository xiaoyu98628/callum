// Package arguments 提供内置工具共用的 JSON 参数结构校验，不处理业务默认值或权限。
package arguments

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xiaoyu98628/callum/internal/tool"
)

const maxDepth = 8

// Decode 将单个参数对象解码到结构体，返回顶层字段的存在标记。
// 顶层及嵌套对象均按显式结构体 JSON 标签精确匹配字段名，类型由目标结构体校验。
// 所有层级均拒绝重复字段、null 和过深嵌套，错误时不得使用 dst 中的部分值。
func Decode(raw json.RawMessage, dst any) (map[string]bool, error) {
	if len(raw) > tool.MaxArgumentBytes {
		return nil, tool.ErrLimit
	}
	if !utf8.Valid(raw) {
		return nil, tool.ErrInvalid
	}
	target := reflect.TypeOf(dst)
	if target == nil || target.Kind() != reflect.Pointer || reflect.ValueOf(dst).IsNil() {
		return nil, tool.ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, tool.ErrInvalid
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return nil, tool.ErrInvalid
		}
		seen[key] = true
		field := fieldType(target.Elem(), key)
		if field == nil {
			return nil, tool.ErrInvalid
		}
		if !value(decoder, field, 1) {
			return nil, fieldError(key, field)
		}
	}
	last, err := decoder.Token()
	if err != nil || last != json.Delim('}') {
		return nil, tool.ErrInvalid
	}
	var trailing any
	if !errors.Is(decoder.Decode(&trailing), io.EOF) {
		return nil, tool.ErrInvalid
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if typed, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
			field := target.Elem()
			for key := range strings.SplitSeq(typed.Field, ".") {
				for field != nil && field.Kind() == reflect.Pointer {
					field = field.Elem()
				}
				if field == nil {
					break
				}
				if field.Kind() == reflect.Slice || field.Kind() == reflect.Array {
					// Go 1.27 的错误路径包含数组下标，只按已声明的元素类型继续定位。
					index, err := strconv.Atoi(key)
					if err != nil || index < 0 {
						field = nil
						break
					}
					field = field.Elem()
					continue
				}
				field = fieldType(field, key)
			}
			if field != nil {
				return nil, fieldError(typed.Field, field)
			}
		}
		return nil, tool.ErrInvalid
	}
	return seen, nil
}

func value(decoder *json.Decoder, target reflect.Type, depth int) bool {
	if depth > maxDepth {
		return false
	}
	token, err := decoder.Token()
	if err != nil || token == nil {
		return false
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return true
	}
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			token, err := decoder.Token()
			key, ok := token.(string)
			if err != nil || !ok || seen[key] {
				return false
			}
			seen[key] = true
			field := fieldType(target, key)
			if field == nil || !value(decoder, field, depth+1) {
				return false
			}
		}
	case '[':
		if target.Kind() != reflect.Slice && target.Kind() != reflect.Array {
			return false
		}
		for decoder.More() {
			if !value(decoder, target.Elem(), depth+1) {
				return false
			}
		}
	default:
		return false
	}
	last, err := decoder.Token()
	return err == nil && ((delim == '{' && last == json.Delim('}')) || (delim == '[' && last == json.Delim(']')))
}

// 工具参数显式声明结构体字段，不依赖 encoding/json 的字段名大小写兼容匹配。
func fieldType(target reflect.Type, key string) reflect.Type {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target.Kind() != reflect.Struct {
		return nil
	}
	for field := range target.Fields() {
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" {
			name = field.Name
		}
		if name != "-" && name == key {
			return field.Type
		}
	}
	return nil
}

func fieldError(key string, target reflect.Type) error {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	expected := "符合工具定义的值"
	switch target.Kind() {
	case reflect.String:
		expected = "字符串"
	case reflect.Bool:
		expected = "布尔值"
	case reflect.Int, reflect.Int64:
		expected = "整数"
	case reflect.Struct:
		expected = "JSON 对象"
	case reflect.Slice, reflect.Array:
		expected = "数组"
		if target.Elem().Kind() == reflect.String {
			expected = "字符串数组"
		}
	}
	message := "必须是" + expected + "，且不能为 null；请按工具定义重新提供"
	if target.Kind() == reflect.Slice || target.Kind() == reflect.Array {
		message += "，不要把数组编码成字符串"
	}
	return &tool.ArgumentError{Field: key, Message: message}
}
