```go
package 工作区

import (
	上下文包 "context"
	错误判断包 "errors"
	切片包 "slices"
	时间包 "time"

	GORM包 "gorm.io/gorm"

	表结构包 "github.com/xiaoyu98628/callum/internal/storage/sqlite/schema"
	业务域包 "github.com/xiaoyu98628/callum/internal/workspace"
)

// 仓储 持有借用的 GORM 实例，不负责关闭连接。
type 仓储 struct{ 数据库 *GORM包.数据库 }

var _ 业务域包.仓储 = (*仓储)(nil)

func 新建仓储(数据库 *GORM包.数据库) (*仓储, error) {
	if 数据库 == nil {
		return nil, 业务域包.无效错误
	}
	return &仓储{数据库: 数据库}, nil
}

func (当前仓储 *仓储) 创建(执行上下文 上下文包.上下文, 输入 业务域包.工作区) (输出 业务域包.工作区, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.工作区
		错误 := GORM包.记录未找到错误
		if 输入.根路径 != "" {
			错误 = 事务.条件("root_path = ?", 输入.根路径).取出(&行).错误
		}
		switch {
		case 错误 == nil:
			if 行.删除时间 == nil {
				return 业务域包.冲突错误
			}
			if 错误 := 事务.模型(&行).更新集(map[string]any{"name": 输入.名称, "deleted_at": nil, "updated_at": 输入.更新时间}).错误; 错误 != nil {
				return 错误
			}
			行.名称, 行.删除时间, 行.更新时间 = 输入.名称, nil, 输入.更新时间
			if 输入.其他路径集 != nil {
				if 错误 := 事务.条件("workspace_id = ?", 行.标识).删除(&表结构包.工作区附加文件夹{}).错误; 错误 != nil {
					return 错误
				}
			}
		case 错误判断包.是否属于(错误, GORM包.记录未找到错误):
			行 = 表结构包.工作区{标识: 输入.标识, 名称: 输入.名称, 根路径: 可选路径(输入.根路径), 创建时间: 输入.创建时间, 更新时间: 输入.更新时间}
			if 错误 := 事务.创建(&行).错误; 错误 != nil {
				return 错误
			}
		default:
			return 错误
		}
		if len(输入.其他路径集) > 0 {
			文件夹集 := make([]表结构包.工作区附加文件夹, 0, len(输入.其他路径集))
			for _, 路径 := range 输入.其他路径集 {
				文件夹集 = append(文件夹集, 表结构包.工作区附加文件夹{工作区标识: 行.标识, 路径: 路径})
			}
			if 错误 := 事务.创建(&文件夹集).错误; 错误 != nil {
				return 错误
			}
		}
		输出, 错误 = 工作区带有文件夹集(事务, 行)
		return 错误
	})
	return 输出, 错误
}

func (当前仓储 *仓储) 获取(执行上下文 上下文包.上下文, 标识 string) (输出 业务域包.工作区, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.工作区
		if 错误 := 事务.条件("id = ?", 标识).取出(&行).错误; 错误 != nil {
			if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
				return 业务域包.未找到错误
			}
			return 错误
		}
		输出, 错误 = 工作区带有文件夹集(事务, 行)
		return 错误
	})
	return 输出, 错误
}

func (当前仓储 *仓储) 列表(执行上下文 上下文包.上下文, 筛选 业务域包.筛选) (输出 []业务域包.工作区, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		查询 := 事务.模型(&表结构包.工作区{})
		if 筛选.已删除 {
			查询 = 查询.条件("deleted_at IS NOT NULL")
		} else {
			查询 = 查询.条件("deleted_at IS NULL")
			if 筛选.已归档 {
				查询 = 查询.条件("archived_at IS NOT NULL")
			} else {
				查询 = 查询.条件("archived_at IS NULL")
			}
		}
		var 行集 []表结构包.工作区
		if 错误 := 查询.顺序("pinned_at DESC").顺序("updated_at DESC").顺序("id DESC").上限(筛选.上限).偏移(筛选.偏移).查找(&行集).错误; 错误 != nil {
			return 错误
		}
		输出 = make([]业务域包.工作区, 0, len(行集))
		标识集 := make([]string, 0, len(行集))
		索引 := make(map[string]int, len(行集))
		for _, 行 := range 行集 {
			索引[行.标识] = len(输出)
			标识集 = append(标识集, 行.标识)
			输出 = append(输出, 工作区来源行(行))
		}
		文件夹集, 错误 := 其他文件夹集(事务, 标识集)
		if 错误 != nil {
			return 错误
		}
		for _, 文件夹 := range 文件夹集 {
			值 := &输出[索引[文件夹.工作区标识]]
			值.其他路径集 = append(值.其他路径集, 文件夹.路径)
		}
		return nil
	})
	return 输出, 错误
}

func (当前仓储 *仓储) 更新(执行上下文 上下文包.上下文, 标识 string, 变更 业务域包.变更) (输出 业务域包.工作区, 错误 error) {
	错误 = 当前仓储.数据库.带有上下文(执行上下文).事务(func(事务 *GORM包.数据库) error {
		var 行 表结构包.工作区
		if 错误 := 事务.条件("id = ?", 标识).取出(&行).错误; 错误 != nil {
			if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
				return 业务域包.未找到错误
			}
			return 错误
		}
		切换根 := 变更.根路径 != nil && *变更.根路径 != 路径值(行.根路径)
		目录集变化通知 := 切换根
		if 变更.其他路径集 != nil {
			if 变更.根路径 == nil || 变更.预期文件夹集 == nil {
				return 业务域包.无效错误
			}
			当前, 错误 := 工作区带有文件夹集(事务, 行)
			if 错误 != nil {
				return 错误
			}
			文件夹集 := append([]string{}, 当前.其他路径集...)
			if 当前.根路径 != "" {
				文件夹集 = append(文件夹集, 当前.根路径)
			}
			if (len(*变更.预期文件夹集) == 0 && 当前.根路径 != "") || (len(*变更.预期文件夹集) > 0 && (*变更.预期文件夹集)[0] != 当前.根路径) {
				return 业务域包.变化通知错误
			}
			if !切片包.相等(切片包.已排序(切片包.值集(文件夹集)), 切片包.已排序(切片包.值集(*变更.预期文件夹集))) {
				return 业务域包.变化通知错误
			}
			目录集变化通知 = 切换根 || !切片包.相等(当前.其他路径集, 切片包.已排序(切片包.值集(*变更.其他路径集)))
		}
		if 目录集变化通知 && 行.删除时间 != nil {
			return 业务域包.未找到错误
		}
		// 与执行受理共用写事务边界，避免工具运行期间改变相对路径及审批范围。
		if 目录集变化通知 || 变更.已归档 != nil || 变更.已删除 != nil {
			var 数量 int64
			if 错误 := 事务.模型(&表结构包.执行{}).关联("JOIN conversations ON conversations.id = conversation_runs.conversation_id").条件("conversations.workspace_id = ? AND conversation_runs.status IN ?", 标识, []string{"running", "awaiting_approval", "awaiting_user_input"}).数量(&数量).错误; 错误 != nil {
				return 错误
			}
			if 数量 != 0 {
				return 业务域包.繁忙错误
			}
		}
		当前时间 := 时间包.当前时间().Unix毫秒()
		更新集 := map[string]any{"updated_at": 当前时间}
		if 变更.名称 != nil {
			行.名称 = *变更.名称
			更新集["name"] = 行.名称
		}
		if 切换根 {
			if 变更.其他路径集 == nil {
				if 错误 := 交换根(事务, 行, *变更.根路径); 错误 != nil {
					return 错误
				}
			} else if *变更.根路径 != "" {
				var 数量 int64
				if 错误 := 事务.模型(&表结构包.工作区{}).条件("root_path = ? AND id <> ?", *变更.根路径, 标识).数量(&数量).错误; 错误 != nil {
					return 错误
				}
				if 数量 != 0 {
					return 业务域包.冲突错误
				}
			}
			更新集["root_path"] = 可选路径(*变更.根路径)
		}
		if 目录集变化通知 && 变更.其他路径集 != nil {
			if 错误 := 事务.条件("workspace_id = ?", 标识).删除(&表结构包.工作区附加文件夹{}).错误; 错误 != nil {
				return 错误
			}
			for _, 路径 := range *变更.其他路径集 {
				if 错误 := 事务.创建(&表结构包.工作区附加文件夹{工作区标识: 标识, 路径: 路径}).错误; 错误 != nil {
					return 错误
				}
			}
		}
		for 键, 命令行参数 := range map[string]*bool{"pinned_at": 变更.置顶, "archived_at": 变更.已归档, "deleted_at": 变更.已删除} {
			if 命令行参数 != nil {
				if *命令行参数 {
					更新集[键] = 当前时间
				} else {
					更新集[键] = nil
				}
			}
		}
		if 错误 := 事务.模型(&行).更新集(更新集).错误; 错误 != nil {
			return 错误
		}
		if 错误 := 事务.条件("id = ?", 标识).取出(&行).错误; 错误 != nil {
			return 错误
		}
		输出, 错误 = 工作区带有文件夹集(事务, 行)
		return 错误
	})
	return 输出, 错误
}

// 交换根 在同一事务中交换主目录与已有附加目录，不增删源目录或改变项目标识。
func 交换根(事务 *GORM包.数据库, 行 表结构包.工作区, 路径 string) error {
	var 文件夹 表结构包.工作区附加文件夹
	if 错误 := 事务.条件("workspace_id = ? AND path = ?", 行.标识, 路径).取出(&文件夹).错误; 错误 != nil {
		if 错误判断包.是否属于(错误, GORM包.记录未找到错误) {
			return 业务域包.无效错误
		}
		return 错误
	}
	var 数量 int64
	if 错误 := 事务.模型(&表结构包.工作区{}).条件("root_path = ? AND id <> ?", 路径, 行.标识).数量(&数量).错误; 错误 != nil {
		return 错误
	}
	if 数量 != 0 {
		return 业务域包.冲突错误
	}
	return 事务.模型(&文件夹).更新("path", 路径值(行.根路径)).错误
}

func 可选路径(路径 string) *string {
	if 路径 == "" {
		return nil
	}
	return &路径
}

func 路径值(路径 *string) string {
	if 路径 == nil {
		return ""
	}
	return *路径
}

func 工作区来源行(行 表结构包.工作区) 业务域包.工作区 {
	return 业务域包.工作区{
		标识: 行.标识, 名称: 行.名称, 根路径: 路径值(行.根路径), 其他路径集: []string{},
		置顶位于: 行.置顶位于, 已归档位于: 行.已归档位于, 删除时间: 行.删除时间,
		创建时间: 行.创建时间, 更新时间: 行.更新时间,
	}
}

func 工作区带有文件夹集(事务 *GORM包.数据库, 行 表结构包.工作区) (业务域包.工作区, error) {
	文件夹集, 错误 := 其他文件夹集(事务, []string{行.标识})
	if 错误 != nil {
		return 业务域包.工作区{}, 错误
	}
	值 := 工作区来源行(行)
	for _, 文件夹 := range 文件夹集 {
		值.其他路径集 = append(值.其他路径集, 文件夹.路径)
	}
	return 值, nil
}

func 其他文件夹集(事务 *GORM包.数据库, 标识集 []string) ([]表结构包.工作区附加文件夹, error) {
	if len(标识集) == 0 {
		return nil, nil
	}
	var 文件夹集 []表结构包.工作区附加文件夹
	错误 := 事务.选择("workspace_id", "path").条件("workspace_id IN ?", 标识集).顺序("path").查找(&文件夹集).错误
	return 文件夹集, 错误
}
```
