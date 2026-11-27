```go
package 聊天补全

import (
	SHA256包 "crypto/sha256"
	十六进制包 "encoding/hex"
	JSON包 "encoding/json"
	正则表达式包 "regexp"
	切片包 "slices"
	字符串转换包 "strconv"
	字符串包 "strings"

	OpenAI包 "github.com/openai/openai-go/v3"
	共享包 "github.com/openai/openai-go/v3/shared"

	模型包 "github.com/xiaoyu98628/callum/internal/model"
)

const 最大工具参数 = 16 << 10
const 最大工具调用字节 = 8 << 20

var 工具名称模式 = 正则表达式包.必须编译(`^[a-zA-Z0-9_-]{1,64}$`)

func 添加工具集(参数 *OpenAI包.对话生成新建参数, 定义集 []模型包.工具定义) error {
	已见 := map[string]bool{}
	for _, 丁 := range 定义集 {
		var 表结构 map[string]any
		if !工具名称模式.匹配字符串(丁.名称) || 已见[丁.名称] || JSON包.反序列化(丁.参数, &表结构) != nil || 表结构 == nil {
			return 模型包.无效响应错误
		}
		已见[丁.名称] = true
		参数.工具集 = append(参数.工具集, OpenAI包.对话生成工具联合参数{所属函数: &OpenAI包.对话生成函数工具参数{函数: 共享包.函数定义参数{名称: 丁.名称, 说明: OpenAI包.字符串(丁.说明), 参数: 共享包.函数参数(表结构)}}})
	}
	// 不指定必选工具，使用服务端默认的自动工具选择。
	return nil
}

func 检查工具结束原因(原因 string, 已启用 bool) error {
	if 原因 == "tool_calls" && 已启用 {
		return nil
	}
	return 检查结束原因(原因)
}

func 响应调用集(原始, 原因 string, 已启用 bool) ([]模型包.工具调用, error) {
	var 消息 struct {
		调用集 []struct {
			标识 string `json:"id"`
			类型 string `json:"type"`
			函数 struct {
				名称 string `json:"name"`
				参数 string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	if JSON包.反序列化([]byte(原始), &消息) != nil {
		return nil, 模型包.无效响应错误
	}
	if 原因 == "tool_calls" && !已启用 {
		return nil, 模型包.不支持输出错误
	}
	if len(消息.调用集) == 0 {
		if 原因 == "tool_calls" {
			return nil, 模型包.无效响应错误
		}
		return nil, nil
	}
	if !已启用 {
		return nil, 模型包.不支持输出错误
	}
	if 原因 != "tool_calls" {
		return nil, 模型包.无效响应错误
	}
	输出 := make([]模型包.工具调用, 0, len(消息.调用集))
	已见 := map[string]bool{}
	调用字节 := 0
	// 先校验整批身份与边界；有其他协议错误时不得误判为可纠正的参数错误。
	for _, 丙 := range 消息.调用集 {
		调用字节 += len(丙.标识) + len(丙.函数.名称) + len(丙.函数.参数)
		if 调用字节 > 最大工具调用字节 {
			return nil, &模型包.输入上限错误详情{尺寸: "工具调用累计字节", 实际: int64(调用字节), 上限: 最大工具调用字节}
		}
		if 丙.标识 == "" || len(丙.标识) > 256 || 已见[丙.标识] || 丙.类型 != "function" || !工具名称模式.匹配字符串(丙.函数.名称) {
			return nil, 模型包.无效响应错误
		}
		已见[丙.标识] = true
	}
	for 索引, 丙 := range 消息.调用集 {
		if len(丙.函数.参数) > 最大工具参数 {
			return nil, 超限参数(索引, 丙.函数.名称, 丙.函数.参数)
		}
		var 对象 map[string]JSON包.原始JSON消息
		if JSON包.反序列化([]byte(丙.函数.参数), &对象) != nil || 对象 == nil {
			摘要 := SHA256包.校验和256([]byte(丙.函数.名称 + "\x00" + 丙.函数.参数))
			return nil, &模型包.工具参数错误{索引: 索引, 指纹: 十六进制包.编码目标字符串(摘要[:])}
		}
		输出 = append(输出, 模型包.工具调用{标识: 丙.标识, 名称: 丙.函数.名称, 参数: JSON包.原始JSON消息(丙.函数.参数)})
	}
	return 输出, nil
}

// 超限分片的落点由网络分块决定；固定前缀的摘要可识别重复提交的相同大参数。
func 超限参数(索引 int, 名称, 参数 string) *模型包.工具参数错误 {
	摘要 := SHA256包.校验和256([]byte(名称 + "\x00" + 参数[:最大工具参数]))
	return &模型包.工具参数错误{索引: 索引, 工具: 名称, 实际字节: len(参数), 上限字节: 最大工具参数, 指纹: 十六进制包.编码目标字符串(摘要[:])}
}

type 调用片段 struct {
	标识, 名称, 参数 字符串包.构建器
	类别         string
}
type 工具调用捕获 struct {
	调用集 map[int]*调用片段
	大小  int
	未知  map[string][]JSON包.原始JSON消息
}

func (当前工具调用捕获 *工具调用捕获) 添加(原始 JSON包.原始JSON消息) error {
	var 列表 []struct {
		索引 *int    `json:"index"`
		标识 *string `json:"id"`
		类型 *string `json:"type"`
		函数 *struct {
			名称 *string `json:"name"`
			参数 *string `json:"arguments"`
		} `json:"function"`
	}
	if JSON包.反序列化(原始, &列表) != nil {
		return 模型包.无效响应错误
	}
	if 当前工具调用捕获.调用集 == nil {
		当前工具调用捕获.调用集 = map[int]*调用片段{}
	}
	var 条目集 []map[string]JSON包.原始JSON消息
	if 错误 := JSON包.反序列化(原始, &条目集); 错误 != nil {
		return 错误
	}
	for 位置, 丁 := range 列表 {
		if 丁.索引 == nil || *丁.索引 < 0 {
			return 模型包.无效响应错误
		}
		if 错误 := 当前工具调用捕获.捕获未知(条目集[位置], *丁.索引); 错误 != nil {
			return 错误
		}
		文件 := 当前工具调用捕获.调用集[*丁.索引]
		if 文件 == nil {
			文件 = &调用片段{}
			当前工具调用捕获.调用集[*丁.索引] = 文件
		}
		if 丁.类型 != nil {
			if *丁.类型 != "function" || (文件.类别 != "" && 文件.类别 != *丁.类型) {
				return 模型包.不支持输出错误
			}
			文件.类别 = *丁.类型
		}
		if 丁.标识 != nil {
			文件.标识.写入字符串(*丁.标识)
			当前工具调用捕获.大小 += len(*丁.标识)
		}
		if 丁.函数 != nil {
			if 丁.函数.名称 != nil {
				文件.名称.写入字符串(*丁.函数.名称)
				当前工具调用捕获.大小 += len(*丁.函数.名称)
			}
			if 丁.函数.参数 != nil {
				文件.参数.写入字符串(*丁.函数.参数)
				当前工具调用捕获.大小 += len(*丁.函数.参数)
			}
		}
		if 文件.标识.长度() > 256 || 文件.名称.长度() > 64 || 当前工具调用捕获.大小 > 最大工具调用字节 {
			return 模型包.输入上限错误
		}
		if 文件.参数.长度() > 最大工具参数 {
			if 文件.标识.长度() == 0 || 文件.类别 != "function" || !工具名称模式.匹配字符串(文件.名称.字符串()) {
				return 模型包.输入上限错误
			}
			return 超限参数(*丁.索引, 文件.名称.字符串(), 文件.参数.字符串())
		}
	}
	return nil
}

func (当前工具调用捕获 *工具调用捕获) 结果() (JSON包.原始JSON消息, error) {
	if len(当前工具调用捕获.调用集) == 0 {
		return nil, nil
	}
	键集 := make([]int, 0, len(当前工具调用捕获.调用集))
	for 键 := range 当前工具调用捕获.调用集 {
		键集 = append(键集, 键)
	}
	切片包.排序(键集)
	输出 := make([]map[string]any, 0, len(键集))
	for 索引, 键 := range 键集 {
		if 索引 != 键 {
			return nil, 模型包.无效响应错误
		}
		文件 := 当前工具调用捕获.调用集[键]
		输出 = append(输出, map[string]any{"id": 文件.标识.字符串(), "type": 文件.类别, "function": map[string]string{"name": 文件.名称.字符串(), "arguments": 文件.参数.字符串()}})
	}
	return JSON包.序列化(输出)
}

// 校验工具历史 保证调用与结果完整配对，防止后续请求携带悬空或重复工具结果。
func 校验工具历史(消息集 []模型包.消息) error {
	待处理 := map[string]bool{}
	for _, 映射 := range 消息集 {
		if 映射.角色 != 模型包.角色工具 && len(待处理) > 0 {
			return 模型包.无效响应错误
		}
		if len(映射.工具调用集) > 0 {
			if 映射.角色 != 模型包.角色助手 {
				return 模型包.无效响应错误
			}
			for _, 丙 := range 映射.工具调用集 {
				var 参数 map[string]JSON包.原始JSON消息
				if 丙.标识 == "" || len(丙.标识) > 256 || 待处理[丙.标识] || !工具名称模式.匹配字符串(丙.名称) || len(丙.参数) > 最大工具参数 || JSON包.反序列化(丙.参数, &参数) != nil || 参数 == nil {
					return 模型包.无效响应错误
				}
				待处理[丙.标识] = true
			}
		}
		if 映射.角色 == 模型包.角色工具 {
			if !待处理[映射.工具调用标识] {
				return 模型包.无效响应错误
			}
			delete(待处理, 映射.工具调用标识)
		} else if 映射.工具调用标识 != "" {
			return 模型包.无效响应错误
		}
	}
	if len(待处理) > 0 {
		return 模型包.无效响应错误
	}
	return nil
}

// 未知字段按属性保留片段，不猜测供应商增量合并规则，也不保存完整 SSE 网络包。
func (当前工具调用捕获 *工具调用捕获) 捕获未知(条目 map[string]JSON包.原始JSON消息, 索引 int) error {
	if 当前工具调用捕获.未知 == nil {
		当前工具调用捕获.未知 = map[string][]JSON包.原始JSON消息{}
	}
	前缀 := "tool_calls." + 字符串转换包.整数转字符串(索引) + "."
	for 键, 值 := range 条目 {
		switch 键 {
		case "index", "id", "type":
			continue
		case "function":
			var 字段集 map[string]JSON包.原始JSON消息
			if 错误 := JSON包.反序列化(值, &字段集); 错误 != nil {
				return 错误
			}
			for 名称, 部分 := range 字段集 {
				if 名称 != "name" && 名称 != "arguments" {
					路径 := 前缀 + "function." + 名称
					当前工具调用捕获.未知[路径] = append(当前工具调用捕获.未知[路径], 部分)
				}
			}
		default:
			路径 := 前缀 + 键
			当前工具调用捕获.未知[路径] = append(当前工具调用捕获.未知[路径], 值)
		}
	}
	return nil
}
```
