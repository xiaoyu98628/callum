```go
package 供应商

// 供应商 是不含凭据的厂商配置，可用于列表和编辑信息查询。
type 供应商 struct {
	标识       string
	名称       string
	协议       string
	基础URL    string
	已配置API密钥 bool
	创建时间     int64
	更新时间     int64
}

// 供应商输入 是完整配置更新；API密钥 为 nil 时保留已有凭据，非 nil 时替换。
// 此结构供服务内部使用，不直接作为通信协议结构。
type 供应商输入 struct {
	名称    string
	协议    string
	基础URL string
	API密钥 *string `json:"-"`
}

// 推理级别 保存推理等级的本地名称和远端参数值。
type 推理级别 struct {
	显示名称 string `json:"display_name"`
	远程值  string `json:"remote_value"`
}

// 模型输入 保存模型规格及启用设置。
// 能力、推理等级和 词元 容量为 nil 时表示未知；模型类型 为 nil 时按空数组保存。
type 模型输入 struct {
	供应商标识  string
	显示名称   string
	远程模型标识 string
	已启用    bool
	上下文窗口  *int64
	最大输出词元 *int64
	// 能力集 记录模型拥有的能力，不表示用户已启用这些能力。
	能力集 map[string]bool
	// 模型类型 保存用户选中的内容类型；包含 图片 时启用图片输入，不作为工具开关。
	模型类型  []string
	推理等级集 []推理级别
}

// 有效模型类型 判断用户选择的内容类型是否有效；适配器仍只提供已经实现的输入和输出。
func 有效模型类型(类别 string) bool {
	switch 类别 {
	case "text", "image", "audio", "video":
		return true
	default:
		return false
	}
}

// 模型 是包含本地记录标识与时间戳的模型配置，不代表适配器已实现其声明的能力。
type 模型 struct {
	模型输入
	标识   string
	创建时间 int64
	更新时间 int64
}

// 运行时配置 仅用于内部模型调用装配，包含凭据，不得整体写入日志。
type 运行时配置 struct {
	模型    模型
	供应商名称 string
	协议    string
	基础URL string
	API密钥 string `json:"-"`
}

// 列表选项集 使用不可变标识作为游标，不依赖会因增删而漂移的 偏移。
type 列表选项集 struct {
	之后标识 string `json:"after_id,omitempty"`
	上限   int    `json:"limit,omitempty"`
}

// 页 是有界目录查询，只有 还有更多 时才提供下一页游标。
type 页[元素类型 any] struct {
	项集      []元素类型 `json:"items"`
	还有更多    bool   `json:"has_more"`
	下一个之后标识 string `json:"next_after_id"`
}

const 默认列表上限 = 100
const 最大列表上限 = 200

// 最大列表来源字节 限制一页从存储读取的配置字节，通信层另检查实际 JSON 字节。
const 最大列表来源字节 = 2 << 20

// 规范化列表选项集 拒绝无效分页，不把错误参数解释成全量查询。
func 规范化列表选项集(值集 ...列表选项集) (列表选项集, error) {
	if len(值集) > 1 {
		return 列表选项集{}, 无效配置错误
	}
	var 值 列表选项集
	if len(值集) == 1 {
		值 = 值集[0]
	}
	if 值.上限 == 0 {
		值.上限 = 默认列表上限
	}
	if 值.上限 < 1 || 值.上限 > 最大列表上限 || (值.之后标识 != "" && !有效标识(值.之后标识)) {
		return 列表选项集{}, 无效配置错误
	}
	return 值, nil
}
```
