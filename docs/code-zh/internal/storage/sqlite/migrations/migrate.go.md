```go
package 迁移集

import (
	上下文包 "context"
	SQL包 "database/sql"
	错误判断包 "errors"
	格式化包 "fmt"
	切片包 "slices"
	字符串包 "strings"

	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	GORM包 "gorm.io/gorm"
)

// 版本 8 固定 MCP 范围 的保存语义，阻止旧程序重写设置时静默丢弃权限范围。
const 表结构版本 = 8

// 执行 在同一个事务内初始化空库或校验当前结构。调用方持有连接与实例锁。
func 执行(执行上下文 上下文包.上下文, 数据库 *GORM包.数据库) (错误 error) {
	if 错误 := 执行上下文.获取错误(); 错误 != nil {
		return 错误
	}
	if 数据库 == nil {
		return 错误判断包.新建("初始化需要数据库")
	}
	// 固定同一连接，确保迁移期间的外键设置只影响初始化连接且在返回前恢复。
	return 数据库.带有上下文(执行上下文).连接(func(连接 *GORM包.数据库) (错误 error) {
		连接 = 连接.会话连接(&GORM包.会话连接{新建数据库: true})
		var 版本 int
		if 错误 := 连接.原始("PRAGMA user_version").扫描(&版本).错误; 错误 != nil {
			return 错误
		}
		if 版本 == 1 || 版本 == 2 {
			// SQLite 重建父表必须在事务外临时关闭外键，避免触发子表级联删除。
			if 错误 := 连接.进程执行("PRAGMA foreign_keys = OFF").错误; 错误 != nil {
				return 错误
			}
			defer func() {
				错误 = 错误判断包.合并(错误, 连接.带有上下文(上下文包.无取消(执行上下文)).进程执行("PRAGMA foreign_keys = ON").错误)
			}()
		}
		return 执行表结构(执行上下文, 连接, 版本)
	})
}

func 执行表结构(执行上下文 上下文包.上下文, 数据库 *GORM包.数据库, 版本 int) (错误 error) {
	事务 := 数据库.带有上下文(执行上下文).开始()
	if 事务.错误 != nil {
		return 格式化包.格式化创建错误("开始数据库初始化: %w", 事务.错误)
	}
	defer func() {
		回滚错误 := 事务.回滚().错误
		if !错误判断包.是否属于(回滚错误, SQL包.事务完成错误) {
			错误 = 错误判断包.合并(错误, 回滚错误)
		}
	}()
	// 外层读取只决定是否需关闭外键；取得写事务后重新读取，串行处理并发初始化。
	var 实际版本 int
	if 错误 := 事务.原始("PRAGMA user_version").扫描(&实际版本).错误; 错误 != nil {
		return 错误
	}
	if (实际版本 == 1 || 实际版本 == 2) && 版本 != 实际版本 {
		return 错误判断包.新建("数据库结构版本在初始化期间发生变化，请重新打开")
	}
	版本 = 实际版本
	if 版本 > 0 && 版本 <= 表结构版本 && 事务.迁移器().存在约束(&表结构包.上下文版本{}, "ck_context_summary") {
		return 错误判断包.新建("数据库使用旧版上下文摘要结构，请使用独立的新数据目录")
	}
	if 版本 > 0 && 版本 <= 3 {
		if !事务.迁移器().存在列(&表结构包.供应商{}, "credential_format") {
			if 错误 := 事务.进程执行("ALTER TABLE model_providers ADD COLUMN credential_format integer NOT NULL DEFAULT 0 CONSTRAINT ck_provider_credential_format CHECK (credential_format IN (0,1))").错误; 错误 != nil {
				return 错误
			}
		}
	}
	表集 := 切片包.拼接(目录管理表集(), 会话表集(), 审批表集(), 设置表集())
	if 版本 == 0 {
		if 错误 := 初始化(事务, append(切片包.复制(表集), &表结构包.后续消息{})); 错误 != nil {
			return 错误
		}
	} else if 版本 == 1 || 版本 == 2 {
		if 错误 := 校验表结构(事务, 表集); 错误 != nil {
			return 格式化包.格式化创建错误("数据库结构与版本 %d 不匹配: %w", 版本, 错误)
		}
		if 版本 == 1 {
			if 错误 := 可选工作区根(事务); 错误 != nil {
				return 格式化包.格式化创建错误("升级工作区目录结构: %w", 错误)
			}
		}
		if 错误 := 允许自动审核方(事务); 错误 != nil {
			return 格式化包.格式化创建错误("升级审批人约束: %w", 错误)
		}
	} else if 版本 != 3 && 版本 != 4 && 版本 != 6 && 版本 != 7 && 版本 != 表结构版本 {
		return 格式化包.格式化创建错误("不支持数据库结构版本 %d，请使用独立开发数据目录", 版本)
	}
	if 错误 := 校验表结构(事务, 表集); 错误 != nil {
		return 格式化包.格式化创建错误("数据库结构与当前版本 %d 不匹配: %w", 表结构版本, 错误)
	}
	if 版本 > 0 && 版本 < 5 && !事务.迁移器().存在表(&表结构包.后续消息{}) {
		if 错误 := 事务.设置("gorm:table_options", "STRICT").迁移器().创建表(&表结构包.后续消息{}); 错误 != nil {
			return 格式化包.格式化创建错误("创建后续消息表: %w", 错误)
		}
	}
	if 错误 := 校验核心表(事务, &表结构包.后续消息{}); 错误 != nil {
		return 格式化包.格式化创建错误("校验后续消息表: %w", 错误)
	}
	if 错误 := 校验自动审核方(事务); 错误 != nil {
		return 错误
	}
	var 必需 int
	if 错误 := 事务.原始(`SELECT "notnull" FROM pragma_table_info('workspaces') WHERE name='root_path'`).扫描(&必需).错误; 错误 != nil {
		return 错误
	}
	if 必需 != 0 {
		return 错误判断包.新建("工作区主目录仍具有非空约束")
	}
	if 错误 := 确保常规设置(事务); 错误 != nil {
		return 格式化包.格式化创建错误("初始化常规设置: %w", 错误)
	}
	if 错误 := 确保MCP(事务, 版本); 错误 != nil {
		return 格式化包.格式化创建错误("初始化 MCP 存储: %w", 错误)
	}
	if 版本 != 表结构版本 {
		if 错误 := 事务.进程执行(格式化包.格式化字符串("PRAGMA user_version = %d", 表结构版本)).错误; 错误 != nil {
			return 错误
		}
	}
	return 事务.提交().错误
}

func 初始化(事务 *GORM包.数据库, 表集 []any) error {
	for _, 表 := range 表集 {
		if 事务.迁移器().存在表(表) {
			return 错误判断包.新建("数据库已有业务表但缺少结构版本，无法安全初始化")
		}
	}
	if 错误 := 事务.设置("gorm:table_options", "STRICT").迁移器().创建表(表集...); 错误 != nil {
		return 格式化包.格式化创建错误("创建数据库结构: %w", 错误)
	}
	return 初始化设置(事务)
}

func 校验表结构(事务 *GORM包.数据库, 表集 []any) error {
	for _, 表 := range 表集 {
		if 错误 := 校验核心表(事务, 表); 错误 != nil {
			return 错误
		}
	}
	for _, 校验 := range []func(*GORM包.数据库) error{校验目录管理, 校验工作区文件夹集, 校验上下文索引, 校验审批集, 校验设置} {
		if 错误 := 校验(事务); 错误 != nil {
			return 错误
		}
	}
	return nil
}

func 校验核心表(事务 *GORM包.数据库, 表 any) error {
	语句 := &GORM包.语句{数据库: 事务}
	if 错误 := 语句.解析(表); 错误 != nil {
		return 错误
	}
	var 建表语句 string
	if 错误 := 事务.原始("SELECT sql FROM sqlite_schema WHERE type='table' AND name=?", 语句.表结构.表).扫描(&建表语句).错误; 错误 != nil {
		return 错误
	}
	if !字符串包.存在后缀(字符串包.目标大写(字符串包.裁剪空间(建表语句)), "STRICT") {
		return 格式化包.格式化创建错误("%s 不是 STRICT 表", 语句.表结构.表)
	}
	// 直接使用 SQLite 的结构事实，避免 GORM 为每张表反复用正则解析大型 检查 建表语句。
	// 同时保留字段、类型、非空及单列主键的原有校验强度。
	type 列信息 struct {
		名称 string
		类型 string
		非空 int
		主键 int
	}
	var 列集 []列信息
	if 错误 := 事务.原始("SELECT name,type,\"notnull\",pk FROM pragma_table_info(?)", 语句.表结构.表).扫描(&列集).错误; 错误 != nil {
		return 错误
	}
	实际 := make(map[string]列信息, len(列集))
	主要 := 0
	for _, 列 := range 列集 {
		实际[列.名称] = 列
		if 列.主键 > 0 {
			主要++
		}
	}
	if 主要 != 1 {
		return 错误判断包.新建("表必须具有单列主键")
	}
	for _, 字段 := range 语句.表结构.字段集 {
		if 字段.数据库名称 == "" {
			continue
		}
		列, 存在 := 实际[字段.数据库名称]
		if !存在 {
			return 格式化包.格式化创建错误("%s 缺少字段 %s", 语句.表结构.表, 字段.数据库名称)
		}
		预期 := 字段.标签设置["TYPE"]
		if 预期 == "" {
			预期 = "integer"
		}
		if !字符串包.相等折叠(列.类型, 预期) {
			return 格式化包.格式化创建错误("%s.%s 类型不匹配", 语句.表结构.表, 字段.数据库名称)
		}
		if 字段.非空 && 列.非空 == 0 {
			return 格式化包.格式化创建错误("%s.%s 缺少非空约束", 语句.表结构.表, 字段.数据库名称)
		}
		if 字段.主键 {
			if 列.主键 == 0 {
				return 错误判断包.新建("表主键不匹配")
			}
		}
		if 检查 := 字段.标签设置["CHECK"]; 检查 != "" {
			名称, _, _ := 字符串包.截断(检查, ",")
			if !事务.迁移器().存在约束(表, 名称) {
				return 格式化包.格式化创建错误("缺少检查约束 %s", 名称)
			}
		}
	}
	for _, 索引 := range 语句.表结构.解析索引集() {
		if !事务.迁移器().存在索引(表, 索引.名称) {
			return 格式化包.格式化创建错误("缺少索引 %s", 索引.名称)
		}
	}
	for 名称 := range 语句.表结构.关系集.关系集 {
		if !事务.迁移器().存在约束(表, 名称) {
			return 格式化包.格式化创建错误("缺少关联约束 %s", 名称)
		}
	}
	return nil
}

// 使用有界元数据查询校验真实唯一索引，避免仅凭索引名称接受损坏的同版本结构。
func 校验唯一索引(事务 *GORM包.数据库, 表, 名称 string, 预期 []string) error {
	var 信息 struct {
		名称 string
		唯一 int
		部分 int
	}
	if 错误 := 事务.原始(`SELECT name,"unique",partial FROM pragma_index_list(?) WHERE name=?`, 表, 名称).扫描(&信息).错误; 错误 != nil {
		return 错误
	}
	var 列集 []string
	if 错误 := 事务.原始("SELECT name FROM pragma_index_info(?) ORDER BY seqno", 名称).扫描(&列集).错误; 错误 != nil {
		return 错误
	}
	if 信息.名称 != 名称 || 信息.唯一 != 1 || 信息.部分 != 0 || !切片包.相等(列集, 预期) {
		return 错误判断包.新建("唯一索引结构不匹配")
	}
	return nil
}
```
