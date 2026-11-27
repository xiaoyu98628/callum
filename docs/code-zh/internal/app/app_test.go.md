```go
package 应用模块

import (
	字节包 "bytes"
	上下文包 "context"
	JSON包 "encoding/json"
	错误判断包 "errors"
	输入输出包 "io"
	结构化日志包 "log/slog"
	网络包 "net"
	HTTP包 "net/http"
	HTTP测试包 "net/http/httptest"
	文件路径包 "path/filepath"
	字符串转换包 "strconv"
	字符串包 "strings"
	测试包 "testing"
	时间包 "time"

	配置模块包 "github.com/xiaoyu98628/callum/internal/config"
	凭据测试包 "github.com/xiaoyu98628/callum/internal/credentials/credentialstest"
)

type 通信帧 struct {
	标识     string          `json:"id"`
	方法 string          `json:"method"`
	参数 JSON包.原始JSON消息 `json:"params"`
	结果 JSON包.原始JSON消息 `json:"result"`
	错误  *struct {
		代码    int             `json:"code"`
		消息 string          `json:"message"`
		数据    JSON包.原始JSON消息 `json:"data"`
	} `json:"error"`
}
type 应用通信对端 struct {
	连接        网络包.连接
	解码器     *JSON包.解码器
	完成        chan error
	取消函数      上下文包.取消函数
	待处理     []通信帧
	序号    int
	已停止     bool
	已见        字符串包.构建器
	读取超时 时间包.时长
}

func 开始应用(测试器 *测试包.测试器, 启动配置 配置模块包.配置, 日志集 输入输出包.写入器) *应用通信对端 {
	测试器.标记测试辅助函数()
	服务端, 客户端 := 网络包.管道()
	执行上下文, 取消函数 := 上下文包.创建可取消上下文(测试器.上下文())
	当前对象 := &应用通信对端{连接: 客户端, 解码器: JSON包.新建解码器(客户端), 完成: make(chan error, 1), 取消函数: 取消函数}
	go func() {
		当前对象.完成 <- 运行(执行上下文, 启动配置, 服务端, 服务端, 结构化日志包.新建(结构化日志包.新建JSON处理器(日志集, nil)), 凭据测试包.系统凭据库{})
	}()
	测试器.注册清理函数(func() {
		取消函数()
		客户端.关闭()
		if !当前对象.已停止 {
			select {
			case 错误 := <-当前对象.完成:
				if 错误 != nil {
					测试器.格式化创建错误("应用未正常清理: %v", 错误)
				}
			case <-时间包.之后(3 * 时间包.秒):
				测试器.错误文本("应用未退出")
			}
		}
	})
	if 文件 := 当前对象.读取(测试器); 文件.方法 != "system.ready" {
		测试器.格式化报告致命错误("应用未就绪: %+v", 文件)
	}
	return 当前对象
}
func (当前应用通信对端 *应用通信对端) 读取(测试器 *测试包.测试器) 通信帧 {
	测试器.标记测试辅助函数()
	超时 := 当前应用通信对端.读取超时
	if 超时 <= 0 {
		超时 = 3 * 时间包.秒
	}
	if 错误 := 当前应用通信对端.连接.设置读取截止时间(时间包.当前时间().添加(超时)); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 文件 通信帧
	if 错误 := 当前应用通信对端.解码器.解码(&文件); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	已编码, 错误 := JSON包.序列化(文件)
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	当前应用通信对端.已见.写入(已编码)
	return 文件
}
func (当前应用通信对端 *应用通信对端) 调用(测试器 *测试包.测试器, 方法 string, 参数 any) 通信帧 {
	测试器.标记测试辅助函数()
	当前应用通信对端.序号++
	标识 := 字符串转换包.整数转字符串(当前应用通信对端.序号)
	if 错误 := 当前应用通信对端.连接.设置写入截止时间(时间包.当前时间().添加(3 * 时间包.秒)); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	if 错误 := JSON包.新建编码器(当前应用通信对端.连接).编码(map[string]any{"jsonrpc": "2.0", "id": 标识, "method": 方法, "params": 参数}); 错误 != nil {
		测试器.报告致命错误(错误)
	}
	for {
		文件 := 当前应用通信对端.读取(测试器)
		if 文件.标识 == 标识 {
			return 文件
		}
		当前应用通信对端.待处理 = append(当前应用通信对端.待处理, 文件)
	}
}
func (当前应用通信对端 *应用通信对端) 事件(测试器 *测试包.测试器, 方法 string) 通信帧 {
	测试器.标记测试辅助函数()
	for 索引, 文件 := range 当前应用通信对端.待处理 {
		if 文件.方法 == 方法 {
			当前应用通信对端.待处理 = append(当前应用通信对端.待处理[:索引], 当前应用通信对端.待处理[索引+1:]...)
			return 文件
		}
	}
	for {
		文件 := 当前应用通信对端.读取(测试器)
		if 文件.方法 == 方法 {
			return 文件
		}
		当前应用通信对端.待处理 = append(当前应用通信对端.待处理, 文件)
	}
}
func (当前应用通信对端 *应用通信对端) 停止(测试器 *测试包.测试器) {
	测试器.标记测试辅助函数()
	文件 := 当前应用通信对端.调用(测试器, "system.shutdown", map[string]any{})
	要求成功(测试器, 文件)
	// 关闭响应之后仍可能有已提交的业务通知，读取至 文件结束 才能完成协议排空。
	for {
		var 帧 通信帧
		if 错误 := 当前应用通信对端.解码器.解码(&帧); 错误 != nil {
			if !错误判断包.是否属于(错误, 输入输出包.文件结束) {
				测试器.报告致命错误(错误)
			}
			break
		}
		当前应用通信对端.待处理 = append(当前应用通信对端.待处理, 帧)
	}
	select {
	case 错误 := <-当前应用通信对端.完成:
		当前应用通信对端.已停止 = true
		if 错误 != nil {
			测试器.报告致命错误(错误)
		}
	case <-时间包.之后(3 * 时间包.秒):
		测试器.报告致命错误("关闭超时")
	}
}
func 要求成功(测试器 *测试包.测试器, 当前通信帧 通信帧) {
	测试器.标记测试辅助函数()
	if 当前通信帧.错误 != nil {
		测试器.格式化报告致命错误("接口失败: %+v", 当前通信帧.错误)
	}
}
func 结果标识(测试器 *测试包.测试器, 当前通信帧 通信帧, 键 string) string {
	测试器.标记测试辅助函数()
	要求成功(测试器, 当前通信帧)
	var 结果 map[string]string
	if 错误 := JSON包.反序列化(当前通信帧.结果, &结果); 错误 != nil || 结果[键] == "" {
		测试器.格式化报告致命错误("缺少标识: %s %v", 当前通信帧.结果, 错误)
	}
	return 结果[键]
}

func Test存储目标模型通过标准输入输出(测试器 *测试包.测试器) {
	取消已观测 := make(chan struct{}, 1)
	上游 := HTTP测试包.新建服务端(HTTP包.处理器函数(func(当前响应写入器 HTTP包.响应写入器, 当前请求 *HTTP包.请求) {
		if 当前请求.URL.路径 != "/v1/chat/completions" || 当前请求.头部.获取("Authorization") != "Bearer fake-integration-key" {
			测试器.错误文本("端点或运行凭据不正确")
			当前响应写入器.写入头部(400)
			return
		}
		var 请求 struct {
			模型    string `json:"model"`
			流   bool   `json:"stream"`
			消息集 []struct {
				内容 string `json:"content"`
			} `json:"messages"`
		}
		if 错误 := JSON包.新建解码器(当前请求.正文).解码(&请求); 错误 != nil || 请求.模型 != "remote-test-model" || len(请求.消息集) == 0 {
			测试器.格式化创建错误("请求转换错误: %v", 错误)
			当前响应写入器.写入头部(400)
			return
		}
		当前响应写入器.头部().设置("X-Request-ID", "upstream-request")
		if 请求.消息集[0].内容 == "供应商失败" {
			当前响应写入器.头部().设置("Content-Type", "application/json")
			当前响应写入器.写入头部(HTTP包.状态未授权)
			if _, 错误 := 输入输出包.写入字符串(当前响应写入器, `{"error":{"message":"fake-integration-key","type":"authentication_error"}}`); 错误 != nil {
				测试器.错误文本(错误)
			}
			return
		}
		if !请求.流 {
			当前响应写入器.头部().设置("Content-Type", "application/json")
			if _, 错误 := 输入输出包.写入字符串(当前响应写入器, `{"id":"reply","choices":[{"index":0,"message":{"role":"assistant","content":"完整回答"},"finish_reason":"stop"}]}`); 错误 != nil {
				测试器.错误文本(错误)
			}
			return
		}
		当前响应写入器.头部().设置("Content-Type", "text/event-stream")
		if _, 错误 := 输入输出包.写入字符串(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"流式回答\"},\"finish_reason\":null}]}\n\n"); 错误 != nil {
			测试器.错误文本(错误)
			return
		}
		当前响应写入器.(HTTP包.刷新器).刷新()
		if 请求.消息集[0].内容 == "等待取消" {
			<-当前请求.上下文().完成()
			取消已观测 <- struct{}{}
			return
		}
		if _, 错误 := 输入输出包.写入字符串(当前响应写入器, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"); 错误 != nil {
			测试器.错误文本(错误)
		}
	}))
	测试器.注册清理函数(上游.关闭)
	启动配置, 错误 := 配置模块包.加载(文件路径包.合并(测试器.创建临时目录(), "data"))
	if 错误 != nil {
		测试器.报告致命错误(错误)
	}
	var 日志集 字节包.缓冲区
	当前对象 := 开始应用(测试器, 启动配置, &日志集)
	进程标识 := 结果标识(测试器, 当前对象.调用(测试器, "provider.create", map[string]any{"name": "集成厂商", "protocol": "chatcompletions", "base_url": 上游.URL + "/v1", "api_key": "fake-integration-key"}), "id")
	模型参数 := map[string]any{"provider_id": 进程标识, "display_name": "集成模型", "remote_model_id": "remote-test-model", "enabled": true}
	消息标识 := 结果标识(测试器, 当前对象.调用(测试器, "model.create", 模型参数), "id")
	调用 := func(流 bool, 输入 string) string {
		return 结果标识(测试器, 当前对象.调用(测试器, "model.invoke", map[string]any{"model_id": 消息标识, "stream": 流, "messages": []map[string]string{{"role": "user", "content": 输入}}}), "run_id")
	}
	运行 := 调用(false, "非流式")
	文件 := 当前对象.事件(测试器, "run.completed")
	if !字节包.包含(文件.参数, []byte(运行)) || !字节包.包含(文件.参数, []byte("完整回答")) || !字节包.包含(文件.参数, []byte("upstream-request")) {
		测试器.格式化报告致命错误("非流式结果错误: %s", 文件.参数)
	}
	调用(true, "流式")
	当前对象.事件(测试器, "run.text_delta")
	当前对象.事件(测试器, "run.completed")
	调用(false, "供应商失败")
	文件 = 当前对象.事件(测试器, "run.failed")
	if !字节包.包含(文件.参数, []byte(`"http_status":401`)) || !字节包.包含(文件.参数, []byte("upstream-request")) {
		测试器.格式化报告致命错误("供应商错误信息丢失: %s", 文件.参数)
	}
	首个 := 调用(true, "等待取消")
	当前对象.事件(测试器, "run.text_delta")
	秒 := 调用(false, "另一调用")
	文件 = 当前对象.事件(测试器, "run.completed")
	if !字节包.包含(文件.参数, []byte(秒)) {
		测试器.格式化报告致命错误("执行隔离错误: %s", 文件.参数)
	}
	要求成功(测试器, 当前对象.调用(测试器, "run.cancel", map[string]any{"run_id": 首个}))
	文件 = 当前对象.事件(测试器, "run.cancelled")
	if !字节包.包含(文件.参数, []byte(首个)) {
		测试器.报告致命错误("取消了错误的执行")
	}
	select {
	case <-取消已观测:
	case <-时间包.之后(3 * 时间包.秒):
		测试器.报告致命错误("取消没有传递到 HTTP 请求")
	}
	要求成功(测试器, 当前对象.调用(测试器, "model.set_enabled", map[string]any{"id": 消息标识, "enabled": false}))
	文件 = 当前对象.调用(测试器, "model.invoke", map[string]any{"model_id": 消息标识, "messages": []map[string]string{{"role": "user", "content": "禁用"}}})
	if 文件.错误 == nil || 文件.错误.代码 != 1004 {
		测试器.格式化报告致命错误("禁用模型仍然接受调用: %+v", 文件)
	}
	要求成功(测试器, 当前对象.调用(测试器, "model.set_enabled", map[string]any{"id": 消息标识, "enabled": true}))
	模型参数["id"] = 消息标识
	模型参数["capabilities"] = map[string]bool{"tools": true}
	模型参数["model_type"] = []string{"text", "image"}
	要求成功(测试器, 当前对象.调用(测试器, "model.update", 模型参数))
	// 类型与能力元数据不能阻止兼容接口执行文本请求；该接口不负责工具循环。
	调用(false, "元数据不限制文本调用")
	当前对象.事件(测试器, "run.completed")
	当前对象.停止(测试器)
	if 字符串包.包含(当前对象.已见.字符串(), "fake-integration-key") || 字符串包.包含(日志集.字符串(), "fake-integration-key") {
		测试器.报告致命错误("凭据泄露到协议或日志")
	}
	// 重启复用同一临时数据库，检查持久化的 UUID 不发生变化。
	对象2 := 开始应用(测试器, 启动配置, 输入输出包.丢弃)
	文件 = 对象2.调用(测试器, "model.list", map[string]any{})
	要求成功(测试器, 文件)
	if !字节包.包含(文件.结果, []byte(消息标识)) || !字节包.包含(文件.结果, []byte(进程标识)) {
		测试器.格式化报告致命错误("重启后配置丢失: %s", 文件.结果)
	}
	对象2.停止(测试器)
}

type 自有输入 struct {
	输入输出包.读取器
	已关闭 int
	错误    error
}

func (当前自有输入 *自有输入) 关闭() error { 当前自有输入.已关闭++; return 当前自有输入.错误 }

type 自有输出 struct {
	字节包.缓冲区
	已关闭 int
	错误    error
}

func (当前自有输出 *自有输出) 关闭() error { 当前自有输出.已关闭++; return 当前自有输出.错误 }
func Test初始化失败关闭自有流集(测试器 *测试包.测试器) {
	关闭错误 := 错误判断包.新建("测试关闭错误")
	输入 := &自有输入{读取器: 字符串包.新建读取器(""), 错误: 关闭错误}
	输出 := &自有输出{}
	启动配置 := 配置模块包.配置{数据库路径: "relative.db", 最大并发执行集: 1, 调用超时: 时间包.秒}
	错误 := 执行(测试器.上下文(), 启动配置, 输入, 输出, 结构化日志包.新建(结构化日志包.新建JSON处理器(输入输出包.丢弃, nil)))
	if !错误判断包.是否属于(错误, 关闭错误) || 输入.已关闭 != 1 || 输出.已关闭 != 1 || 输出.长度() != 0 {
		测试器.格式化报告致命错误("初始化失败清理错误: %v %d %d", 错误, 输入.已关闭, 输出.已关闭)
	}
}

func Test有界上游正文(测试器 *测试包.测试器) {
	for _, 长度 := range []int{4, 5} {
		正文 := &有界正文{读取关闭器: 输入输出包.空操作关闭器(字符串包.新建读取器(字符串包.重复("x", 长度))), 剩余: 4}
		数据, 错误 := 输入输出包.读取全部(正文)
		if string(数据) != "xxxx" || (长度 == 5 && !错误判断包.是否属于(错误, 响应上限错误)) || (长度 == 4 && 错误 != nil) {
			测试器.格式化报告致命错误("响应边界错误: %q %v", 数据, 错误)
		}
		if 错误 := 正文.关闭(); 错误 != nil {
			测试器.报告致命错误(错误)
		}
	}
}
```
