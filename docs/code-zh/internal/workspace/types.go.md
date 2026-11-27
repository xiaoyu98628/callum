```go
package 工作区

// 工作区 是用户任务操作的项目目录，不是应用数据目录。
type 工作区 struct {
	标识    string   `json:"id"`
	名称    string   `json:"name"`
	根路径   string   `json:"root_path"`
	其他路径集 []string `json:"other_paths"`
	置顶位于  *int64   `json:"pinned_at"`
	已归档位于 *int64   `json:"archived_at"`
	删除时间  *int64   `json:"deleted_at"`
	创建时间  int64    `json:"created_at"`
	更新时间  int64    `json:"updated_at"`
}

// 变更 只更新明确提供的字段；完整目录更新携带原目录集合，避免覆盖并发修改。
type 变更 struct {
	名称     *string   `json:"name,omitempty"`
	根路径    *string   `json:"root_path,omitempty"`
	其他路径集  *[]string `json:"other_paths,omitempty"`
	预期文件夹集 *[]string `json:"expected_folders,omitempty"`
	置顶     *bool     `json:"pinned,omitempty"`
	已归档    *bool     `json:"archived,omitempty"`
	已删除    *bool     `json:"-"`
}

// 筛选 是有界列表查询，已归档、已删除 选择对应状态的记录。
type 筛选 struct {
	已归档 bool `json:"archived"`
	已删除 bool `json:"deleted"`
	上限  int  `json:"limit"`
	偏移  int  `json:"offset"`
}
```
