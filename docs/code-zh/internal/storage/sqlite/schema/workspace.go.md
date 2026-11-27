```go
package 表结构

// 工作区 映射 工作区集（工作区表），软删除不级联修改会话状态。
type 工作区 struct {
	标识    string  `gorm:"type:text;primaryKey;not null;check:ck_workspace_id,id GLOB '????????-????-7???-[89ab]???-????????????' AND length(replace(id,'-','')) = 32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'"`
	名称    string  `gorm:"type:text;not null;check:ck_workspace_name,length(trim(name)) > 0"`
	根路径   *string `gorm:"type:text;uniqueIndex:uq_workspace_path;check:ck_workspace_path,length(root_path) > 0"`
	置顶位于  *int64
	已归档位于 *int64
	删除时间  *int64 `gorm:"index:ix_workspace_listing,priority:1"`
	创建时间  int64  `gorm:"not null;autoCreateTime:false"`
	更新时间  int64  `gorm:"not null;autoUpdateTime:false;index:ix_workspace_listing,priority:2,sort:desc"`
}

func (工作区) 表名称() string { return "workspaces" }

// 工作区附加文件夹 保存项目的其他源目录，主目录由 工作区 保存。
type 工作区附加文件夹 struct {
	标识    int64  `gorm:"primaryKey;autoIncrement"`
	工作区标识 string `gorm:"type:text;not null;uniqueIndex:uq_workspace_additional_folder,priority:1"`
	路径    string `gorm:"type:text;not null;uniqueIndex:uq_workspace_additional_folder,priority:2;index:ix_workspace_additional_folder_path;check:ck_workspace_additional_folder_path,length(path) > 0"`
	工作区   *工作区   `gorm:"foreignKey:WorkspaceID;references:ID;constraint:OnDelete:CASCADE"`
}

func (工作区附加文件夹) 表名称() string { return "workspace_additional_folders" }
```
