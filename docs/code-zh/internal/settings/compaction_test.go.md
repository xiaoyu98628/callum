```go
package 设置测试

import (
	JSON包 "encoding/json"
	字符串包 "strings"
	测试包 "testing"

	设置包 "github.com/xiaoyu98628/callum/internal/settings"
)

func Test压缩设置为显式并且快照是独立(测试器 *测试包.测试器) {
	值 := 设置包.默认压缩()
	原始, 错误 := JSON包.序列化(值)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已解析, 错误 := 设置包.解码压缩(原始)
	if 错误 != nil || 已解析 != 值 {
		测试器.格式化报告致命错误("默认配置无效: %+v %v", 已解析, 错误)
	}
	for _, 无效 := range []string{
		`{}`, `null`,
		字符串包.替换(string(原始), `"enabled":true`, `"enabled":null`, 1),
		字符串包.替换(string(原始), `"enabled":true`, `"enabled":true,"enabled":false`, 1),
		字符串包.替换(string(原始), `"retain_turns":4`, `"retain_turns":0`, 1),
		字符串包.替换(string(原始), `"retain_turns":4`, `"retain_turns":4,"model_id":"override"`, 1),
		string(原始) + " {}",
		`{"enabled":true,"token_budget":-1,"output_reserve":4096,"safety_margin":1024,"image_token_estimate":8192,"retain_turns":4}`,
		`{"enabled":true,"token_budget":0,"output_reserve":0,"safety_margin":1024,"image_token_estimate":8192,"retain_turns":4}`,
		`{"enabled":true,"token_budget":null,"output_reserve":4096,"safety_margin":1024,"image_token_estimate":8192,"retain_turns":4}`,
	} {
		if _, 错误 := 设置包.解码压缩(JSON包.原始JSON消息(无效)); 错误 == nil {
			测试器.格式化报告致命错误("接受非法压缩配置: %s", 无效)
		}
	}
	原始 := 设置包.快照{修订号: 1, 执行: 设置包.默认执行(), 压缩: &设置包.压缩快照{修订号: 1, 压缩: 值}}
	copy := 原始.复制()
	copy.压缩.已启用 = false
	if !原始.压缩.已启用 {
		测试器.报告致命错误("回调能修改原执行策略")
	}
}

func Test词元预算使用已配置容量并且显式回退(测试器 *测试包.测试器) {
	启动配置 := 设置包.默认压缩()
	if _, 错误 := 启动配置.输入预算(0, 0); 错误 == nil {
		测试器.报告致命错误("未知容量被猜测为默认窗口")
	}
	if 预算, 错误 := 启动配置.输入预算(10000, 0); 错误 != nil || 预算 != 4880 {
		测试器.格式化报告致命错误("预留计算错误: %d %v", 预算, 错误)
	}
	if 预算, 错误 := 启动配置.输入预算(10000, 1000); 错误 != nil || 预算 != 7976 {
		测试器.格式化报告致命错误("输出上限没有限制预留: %d %v", 预算, 错误)
	}
	启动配置.词元预算 = 3000
	if 预算, 错误 := 启动配置.输入预算(0, 0); 错误 != nil || 预算 != 3000 {
		测试器.报告致命错误("未知窗口时显式预算失效")
	}
	启动配置.词元预算 = 99999
	if 预算, 错误 := 启动配置.输入预算(10000, 0); 错误 != nil || 预算 != 4880 {
		测试器.报告致命错误("显式预算放大了已知窗口")
	}
	if _, 错误 := 启动配置.输入预算(5000, 0); 错误 == nil {
		测试器.报告致命错误("预留超过窗口仍获得正预算")
	}
}

func Test压缩快照轮次往返并且校验(测试器 *测试包.测试器) {
	原始 := 设置包.压缩快照{修订号: 7, 压缩: 设置包.默认压缩()}
	原始.已启用, 原始.词元预算, 原始.保留轮次集 = false, 3000, 2
	原始, 错误 := JSON包.序列化(原始)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 已解析 设置包.压缩快照
	if 错误 := JSON包.反序列化(原始, &已解析); 错误 != nil || !已解析.有效() || 已解析 != 原始 {
		测试器.格式化报告致命错误("执行快照未完整保存: %+v %v", 已解析, 错误)
	}
	for _, 无效 := range []string{
		`{}`, `null`, `{"revision":7}`,
		`{"revision":7,"enabled":true,"message_threshold":64,"byte_threshold":2000,"retain_turns":2}`,
		字符串包.替换(string(原始), `"revision":7,`, "", 1),
		字符串包.替换(string(原始), `"revision":7`, `"revision":0`, 1),
		字符串包.替换(string(原始), `"revision":7`, `"revision":null`, 1),
		字符串包.替换(string(原始), `"revision":7`, `"revision":7,"revision":8`, 1),
		字符串包.替换(string(原始), `"token_budget":3000`, `"token_budget":null`, 1),
		字符串包.替换(string(原始), `"output_reserve":4096,`, "", 1),
		字符串包.替换(string(原始), `"retain_turns":2`, `"retain_turns":0`, 1),
		字符串包.替换(string(原始), `"retain_turns":2`, `"retain_turns":2,"unknown":true`, 1),
		string(原始) + " {}",
	} {
		if 错误 := JSON包.反序列化([]byte(无效), &已解析); 错误 == nil {
			测试器.格式化报告致命错误("接受无效执行快照: %s", 无效)
		}
		if 已解析 != 原始 {
			测试器.格式化报告致命错误("解码失败改写了已有快照: %+v", 已解析)
		}
	}
}
```
