```go
package 标准输入输出

import (
	缓冲输入输出包 "bufio"
	字节包 "bytes"
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	输入输出包 "io"
	结构化日志包 "log/slog"
	字符串包 "strings"
	测试包 "testing"
	时间包 "time"

	调度器包 "github.com/xiaoyu98628/callum/internal/agent/executor"
	推理调用包 "github.com/xiaoyu98628/callum/internal/inference"
	执行器包 "github.com/xiaoyu98628/callum/internal/inference/executor"
	模型包 "github.com/xiaoyu98628/callum/internal/model"
	供应商包 "github.com/xiaoyu98628/callum/internal/model/provider"
	供应商服务包 "github.com/xiaoyu98628/callum/internal/model/provider/service"
	执行包 "github.com/xiaoyu98628/callum/internal/stdio/execution"
	协议包 "github.com/xiaoyu98628/callum/internal/stdio/protocol"
	传输模块包 "github.com/xiaoyu98628/callum/internal/stdio/transport"
)

type 测试客户端 struct{}

func (测试客户端) 生成(上下文包.上下文, 模型包.请求) (模型包.响应, error) {
	return 模型包.响应{文本: "完整回答", 请求标识: "provider-id"}, nil
}
func (测试客户端) 流式生成(_ 上下文包.上下文, _ 模型包.请求, 发出事件 func(模型包.事件) error) error {
	if 错误 := 发出事件(模型包.事件{类型: 模型包.事件文本增量, 文本: "流式回答"}); 错误 != nil {
		return 错误
	}
	return 发出事件(模型包.事件{类型: 模型包.事件已完成, 请求标识: "provider-id"})
}

type 标识结果 struct {
	标识 string `json:"id"`
}

type 帧 struct {
	标识     JSON包.原始JSON消息 `json:"id"`
	方法 string          `json:"method"`
	结果 JSON包.原始JSON消息 `json:"result"`
	参数 JSON包.原始JSON消息 `json:"params"`
	错误  *协议包.错误 `json:"error"`
}
type 通信对端 struct {
	输入      *输入输出包.管道写入器
	输出     *输入输出包.管道读取器
	帧集     chan 帧
	完成       chan error
	读取器完成 chan struct{}
	取消函数     上下文包.取消函数
}

func 存储用于测试(测试器 *测试包.测试器) *供应商服务包.服务 {
	测试器.标记测试辅助函数()
	服务, 错误 := 供应商服务包.新建(&内存仓储{供应商集: make(map[string]供应商包.供应商输入), 模型集: make(map[string]供应商包.模型)})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	return 服务
}

type 服务端测试夹具 struct {
	供应商集 *供应商服务包.服务
	解析   推理调用包.解析器
}

func 服务端用于测试(测试器 *测试包.测试器, 供应商集 *供应商服务包.服务) *服务端测试夹具 {
	测试器.标记测试辅助函数()
	return &服务端测试夹具{供应商集: 供应商集, 解析: func(上下文包.上下文, string) (模型包.客户端, string, error) { return 测试客户端{}, "remote", nil }}
}

// 测试装配独立的连接和执行管理器，遵循应用中的关闭顺序。
func (当前服务端测试夹具 *服务端测试夹具) 提供服务(执行上下文 上下文包.上下文, 输入 输入输出包.读取关闭器, 输出 输入输出包.写入关闭器) (返回错误 error) {
	连接, 错误 := 传输模块包.新建(执行上下文, 输入, 输出)
	if 错误 != nil {
		return 错误
	}
	defer func() { 返回错误 = 错误判断包.合并(返回错误, 连接.关闭()) }()
	共享, 错误 := 调度器包.新建(连接.上下文(), 2, 时间包.秒)
	if 错误 != nil {
		return 错误
	}
	defer 共享.关闭()
	执行集, 错误 := 执行器包.新建带有调度器(共享, 当前服务端测试夹具.解析, func(事件 推理调用包.事件) error { return 执行包.发出事件(连接.通知, 事件) })
	if 错误 != nil {
		return 错误
	}
	服务端, 错误 := 新建(当前服务端测试夹具.供应商集, 执行集, 结构化日志包.新建(结构化日志包.新建JSON处理器(输入输出包.丢弃, nil)))
	if 错误 != nil {
		return 错误
	}
	return 服务端.提供服务(连接)
}
func 打开通信对端(测试器 *测试包.测试器, 服务端 *服务端测试夹具) *通信对端 {
	测试器.标记测试辅助函数()
	输入读取, 输入写入 := 输入输出包.管道()
	输出读取, 输出写入 := 输入输出包.管道()
	执行上下文, 取消函数 := 上下文包.创建可取消上下文(测试器.上下文())
	当前对象 := &通信对端{输入: 输入写入, 输出: 输出读取, 帧集: make(chan 帧, 32), 完成: make(chan error, 1), 读取器完成: make(chan struct{}), 取消函数: 取消函数}
	go func() { 当前对象.完成 <- 服务端.提供服务(执行上下文, 输入读取, 输出写入) }()
	go func() {
		defer close(当前对象.读取器完成)
		defer close(当前对象.帧集)
		扫描器 := 缓冲输入输出包.新建扫描器(输出读取)
		扫描器.缓冲区(make([]byte, 4096), 传输模块包.最大响应字节+2)
		for 扫描器.扫描() {
			var 文件 帧
			if JSON包.反序列化(扫描器.字节(), &文件) != nil {
				return
			}
			select {
			case 当前对象.帧集 <- 文件:
			case <-执行上下文.完成():
				return
			}
		}
	}()
	测试器.注册清理函数(func() {
		取消函数()
		输入写入.关闭()
		输出读取.关闭()
		select {
		case 错误 := <-当前对象.完成:
			if 错误 != nil {
				测试器.格式化创建错误("服务清理失败: %v", 错误)
			}
		case <-时间包.之后(3 * 时间包.秒):
			测试器.错误文本("服务未退出")
		}
		select {
		case <-当前对象.读取器完成:
		case <-时间包.之后(3 * 时间包.秒):
			测试器.错误文本("协议读取未退出")
		}
	})
	if 文件 := 当前对象.下一个(测试器); 文件.方法 != "system.ready" {
		测试器.格式化报告致命错误("缺少就绪事件: %+v", 文件)
	}
	return 当前对象
}
func (当前通信对端 *通信对端) 下一个(测试器 *测试包.测试器) 帧 {
	测试器.标记测试辅助函数()
	select {
	case 文件, 成功 := <-当前通信对端.帧集:
		if !成功 {
			测试器.报告致命错误("协议输出提前关闭")
		}
		return 文件
	case <-时间包.之后(3 * 时间包.秒):
		测试器.报告致命错误("等待协议消息超时")
		return 帧{}
	}
}
func (当前通信对端 *通信对端) 原始(测试器 *测试包.测试器, 行 string) {
	测试器.标记测试辅助函数()
	if _, 错误 := 输入输出包.写入字符串(当前通信对端.输入, 行+"\n"); 错误 != nil {
		测试器.报告致命错误(错误)
	}
}
func (当前通信对端 *通信对端) 调用(测试器 *测试包.测试器, 方法 string, 参数 any) 帧 {
	测试器.标记测试辅助函数()
	数据, 错误 := JSON包.序列化(map[string]any{"jsonrpc": "2.0", "id": "request", "method": 方法, "params": 参数})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前通信对端.原始(测试器, string(数据))
	文件 := 当前通信对端.下一个(测试器)
	if string(文件.标识) != `"request"` {
		测试器.格式化报告致命错误("请求响应顺序错误: %+v", 文件)
	}
	return 文件
}
func 断言成功(测试器 *测试包.测试器, 当前帧 帧) {
	测试器.标记测试辅助函数()
	if 当前帧.错误 != nil {
		测试器.格式化报告致命错误("请求失败: %+v", 当前帧.错误)
	}
}

func Test目录管理通过协议(测试器 *测试包.测试器) {
	存储 := 存储用于测试(测试器)
	当前对象 := 打开通信对端(测试器, 服务端用于测试(测试器, 存储))
	供应商参数 := map[string]any{"name": "测试厂商", "protocol": "chatcompletions", "base_url": "https://example.invalid/v1", "api_key": "fake-private-key"}
	文件 := 当前对象.调用(测试器, "provider.create", 供应商参数)
	断言成功(测试器, 文件)
	var 供应商标识 标识结果
	if 错误 := JSON包.反序列化(文件.结果, &供应商标识); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	供应商参数["id"] = 供应商标识.标识
	供应商参数["name"] = "更新名称"
	delete(供应商参数, "api_key")
	断言成功(测试器, 当前对象.调用(测试器, "provider.update", 供应商参数))
	文件 = 当前对象.调用(测试器, "provider.list", map[string]any{})
	断言成功(测试器, 文件)
	if 字节包.包含(文件.结果, []byte("fake-private-key")) || !字节包.包含(文件.结果, []byte(`"has_api_key":true`)) {
		测试器.格式化报告致命错误("凭据查询不正确: %s", 文件.结果)
	}
	参数 := map[string]any{"provider_id": 供应商标识.标识, "display_name": "测试模型", "remote_model_id": "remote", "enabled": true, "capabilities": map[string]bool{"reasoning": true}, "model_type": []string{"text", "image"}, "reasoning_levels": []map[string]string{{"display_name": "中", "remote_value": "medium"}}}
	文件 = 当前对象.调用(测试器, "model.create", 参数)
	断言成功(测试器, 文件)
	var 模型标识 标识结果
	if 错误 := JSON包.反序列化(文件.结果, &模型标识); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	参数["id"] = 模型标识.标识
	参数["enabled"] = false
	参数["capabilities"] = nil
	参数["model_type"] = []string{}
	参数["reasoning_levels"] = nil
	断言成功(测试器, 当前对象.调用(测试器, "model.update", 参数))
	文件 = 当前对象.调用(测试器, "model.get", map[string]any{"id": 模型标识.标识})
	断言成功(测试器, 文件)
	if !字节包.包含(文件.结果, []byte(`"enabled":false`)) || !字节包.包含(文件.结果, []byte(`"capabilities":null`)) {
		测试器.格式化报告致命错误("零值更新失败: %s", 文件.结果)
	}
	文件 = 当前对象.调用(测试器, "model.list", map[string]any{"selectable_only": true})
	断言成功(测试器, 文件)
	if string(文件.结果) != `{"items":[],"has_more":false,"next_after_id":""}` {
		测试器.报告致命错误(string(文件.结果))
	}
	断言成功(测试器, 当前对象.调用(测试器, "model.set_enabled", map[string]any{"id": 模型标识.标识, "enabled": true}))
	运行时, 错误 := 存储.解析(测试器.上下文(), 模型标识.标识)
	if 错误 != nil || 运行时.API密钥 != "fake-private-key" {
		测试器.格式化报告致命错误("省略密钥应保留: %v", 错误)
	}
	文件 = 当前对象.调用(测试器, "provider.delete", map[string]any{"id": 供应商标识.标识})
	if 文件.错误 == nil || 文件.错误.代码 != 1003 {
		测试器.格式化报告致命错误("关联删除未拒绝: %+v", 文件)
	}
	供应商参数["api_key"] = ""
	断言成功(测试器, 当前对象.调用(测试器, "provider.update", 供应商参数))
	文件 = 当前对象.调用(测试器, "provider.get", map[string]any{"id": 供应商标识.标识})
	断言成功(测试器, 文件)
	if !字节包.包含(文件.结果, []byte(`"has_api_key":false`)) {
		测试器.报告致命错误(string(文件.结果))
	}
	断言成功(测试器, 当前对象.调用(测试器, "model.delete", map[string]any{"id": 模型标识.标识}))
	断言成功(测试器, 当前对象.调用(测试器, "provider.delete", map[string]any{"id": 供应商标识.标识}))
}

func Test无效协议并且参数(测试器 *测试包.测试器) {
	当前对象 := 打开通信对端(测试器, 服务端用于测试(测试器, 存储用于测试(测试器)))
	for _, 测试 := range []struct {
		行 string
		代码 int
	}{
		{`{"jsonrpc":`, -32700},
		{`null`, -32600},
		{string([]byte{'"', 0xff, '"'}), -32700},
		{`[]`, -32600},
		{`{"JSONRPC":"2.0","id":1,"method":"system.info"}`, -32600},
		{`{"jsonrpc":"2.0","id":true,"method":"system.info"}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"missing"}`, -32601},
		{`{"jsonrpc":"2.0","id":1,"method":"system.info","params":{"unknown":true}}`, -32602},
		{`{"jsonrpc":"2.0","id":1,"method":"provider.create","params":{"name":"p","protocol":"chatcompletions","base_url":"https://example.invalid","api_key":null}}`, -32602},
		{`{"jsonrpc":"2.0","id":1,"method":"model.create","params":{"provider_id":"01900000-0000-7000-8000-000000000001","display_name":"m","remote_model_id":"m","enabled":true,"capabilities":{"tools":null}}}`, -32602},
		{`{"jsonrpc":"2.0","id":1,"method":"model.invoke","params":{"model_id":"01900000-0000-7000-8000-000000000001","messages":[{"role":"tool","content":"x"}]}}`, -32602},
	} {
		当前对象.原始(测试器, 测试.行)
		文件 := 当前对象.下一个(测试器)
		if 文件.错误 == nil || 文件.错误.代码 != 测试.代码 {
			测试器.格式化报告致命错误("%s: %+v", 测试.行, 文件)
		}
	}
	// 合法通知不产生响应，下一条应直接是随后请求的结果。
	当前对象.原始(测试器, `{"jsonrpc":"2.0","method":"missing"}`)
	断言成功(测试器, 当前对象.调用(测试器, "system.info", map[string]any{}))
}

func Test响应早于流式并且生成事件集(测试器 *测试包.测试器) {
	当前对象 := 打开通信对端(测试器, 服务端用于测试(测试器, 存储用于测试(测试器)))
	for _, 流 := range []bool{true, false} {
		文件 := 当前对象.调用(测试器, "model.invoke", map[string]any{"model_id": "01900000-0000-7000-8000-000000000001", "stream": 流, "messages": []map[string]string{{"role": "user", "content": "你好"}}})
		断言成功(测试器, 文件)
		var 已受理 struct {
			执行标识 string `json:"run_id"`
		}
		if 错误 := JSON包.反序列化(文件.结果, &已受理); 错误 != nil || 已受理.执行标识 == "" {
			测试器.报告致命错误("缺少执行标识")
		}
		文件 = 当前对象.下一个(测试器)
		if 流 {
			if 文件.方法 != "run.text_delta" {
				测试器.格式化报告致命错误("缺少增量: %+v", 文件)
			}
			文件 = 当前对象.下一个(测试器)
		}
		if 文件.方法 != "run.completed" || !字节包.包含(文件.参数, []byte(已受理.执行标识)) {
			测试器.格式化报告致命错误("终态错误: %+v", 文件)
		}
	}
}

func Test已阻止输出并且输入取消(测试器 *测试包.测试器) {
	测试器.执行("输入阻塞时取消", func(测试器 *测试包.测试器) {
		服务端 := 服务端用于测试(测试器, 存储用于测试(测试器))
		当前对象 := 打开通信对端(测试器, 服务端)
		当前对象.取消函数()
	})
	测试器.执行("超长输入", func(测试器 *测试包.测试器) {
		服务端 := 服务端用于测试(测试器, 存储用于测试(测试器))
		错误 := 服务端.提供服务(测试器.上下文(), 输入输出包.空操作关闭器(字符串包.新建读取器(字符串包.重复("x", 传输模块包.最大请求字节+10))), 丢弃关闭器{})
		if 错误 == nil {
			测试器.报告致命错误("超长请求未拒绝")
		}
	})
}

type 丢弃关闭器 struct{}

func (丢弃关闭器) 写入(字节数据 []byte) (int, error) { return len(字节数据), nil }
func (丢弃关闭器) 关闭() error                { return nil }

type 失败中关闭 struct {
	输入输出包.读取器
	错误 error
}

func (当前失败中关闭 失败中关闭) 关闭() error { return 当前失败中关闭.错误 }

type 失败中写入 struct{ 错误 error }

func (当前失败中写入 失败中写入) 写入([]byte) (int, error) { return 0, 当前失败中写入.错误 }
func (当前失败中写入 失败中写入) 关闭() error              { return nil }
func Test清理并且损坏输出错误判断已保留(测试器 *测试包.测试器) {
	服务端 := 服务端用于测试(测试器, 存储用于测试(测试器))
	关闭错误 := 错误判断包.新建("关闭失败")
	错误 := 服务端.提供服务(测试器.上下文(), 失败中关闭{读取器: 字符串包.新建读取器(""), 错误: 关闭错误}, 丢弃关闭器{})
	if !错误判断包.是否属于(错误, 关闭错误) {
		测试器.格式化报告致命错误("关闭错误丢失: %v", 错误)
	}
	写入错误 := 错误判断包.新建("写入失败")
	错误 = 服务端.提供服务(测试器.上下文(), 输入输出包.空操作关闭器(字符串包.新建读取器("")), 失败中写入{写入错误})
	if !错误判断包.是否属于(错误, 写入错误) {
		测试器.格式化报告致命错误("写入错误丢失: %v", 错误)
	}
}

func Test连接关闭是可重复并且保留清理错误(测试器 *测试包.测试器) {
	关闭错误 := 错误判断包.新建("关闭输入失败")
	连接, 错误 := 传输模块包.新建(测试器.上下文(), 失败中关闭{读取器: 字符串包.新建读取器(""), 错误: 关闭错误}, 丢弃关闭器{})
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for range 2 {
		if 错误 := 连接.关闭(); !错误判断包.是否属于(错误, 关闭错误) {
			测试器.格式化报告致命错误("重复关闭丢失清理错误: %v", 错误)
		}
	}
}

type 等待中客户端 struct{ 已开始 chan struct{} }

func (当前等待中客户端 等待中客户端) 生成(执行上下文 上下文包.上下文, _ 模型包.请求) (模型包.响应, error) {
	close(当前等待中客户端.已开始)
	<-执行上下文.完成()
	return 模型包.响应{}, 执行上下文.获取错误()
}
func (当前等待中客户端 等待中客户端) 流式生成(执行上下文 上下文包.上下文, _ 模型包.请求, _ func(模型包.事件) error) error {
	close(当前等待中客户端.已开始)
	<-执行上下文.完成()
	return 执行上下文.获取错误()
}

func Test文件结束并且关闭取消并且排空活动执行(测试器 *测试包.测试器) {
	for _, 关闭 := range []bool{false, true} {
		测试器.执行(map[bool]string{false: "EOF", true: "关闭请求"}[关闭], func(测试器 *测试包.测试器) {
			服务端 := 服务端用于测试(测试器, 存储用于测试(测试器))
			已开始 := make(chan struct{})
			服务端.解析 = func(上下文包.上下文, string) (模型包.客户端, string, error) {
				return 等待中客户端{已开始}, "remote", nil
			}
			当前对象 := 打开通信对端(测试器, 服务端)
			断言成功(测试器, 当前对象.调用(测试器, "model.invoke", map[string]any{"model_id": "01900000-0000-7000-8000-000000000001", "messages": []map[string]string{{"role": "user", "content": "等待"}}}))
			select {
			case <-已开始:
			case <-时间包.之后(时间包.秒):
				测试器.报告致命错误("执行未启动")
			}
			if 关闭 {
				断言成功(测试器, 当前对象.调用(测试器, "system.shutdown", map[string]any{}))
			} else {
				if 错误 := 当前对象.输入.关闭(); 错误 != nil {
					测试器.报告致命错误(错误)
				}
			}
			if 文件 := 当前对象.下一个(测试器); 文件.方法 != "run.cancelled" {
				测试器.格式化报告致命错误("未排空取消事件: %+v", 文件)
			}
		})
	}
}
```
