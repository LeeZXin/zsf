// Package model 提供数据库模型的基础字段定义。
// 包含审计字段、时间字段、分页和游标等通用结构体，
// 供各业务模型的嵌入使用，减少重复定义。
package model

import "time"

// AuditFields 审计信息字段，包含创建人和更新人信息。
// 可嵌入到需要记录操作审计的业务模型中。
type AuditFields struct {
	Creator string    `json:"creator" xorm:"varchar(64) comment('创建人')"`               // 创建人
	Updater string    `json:"updater" xorm:"varchar(64) comment('编辑人')"`               // 最后编辑人
	Created time.Time `json:"created" xorm:"datetime created notnull comment('创建时间')"` // 创建时间（自动填充）
	Updated time.Time `json:"updated" xorm:"datetime updated notnull comment('更新时间')"` // 最近更新时间（自动更新）
}

// TimeFields 时间字段，仅包含创建和更新时间。
// 适用于无需记录操作人的场景。
type TimeFields struct {
	Created time.Time `json:"created" xorm:"datetime created notnull comment('创建时间')"` // 创建时间（自动填充）
	Updated time.Time `json:"updated" xorm:"datetime updated notnull comment('更新时间')"` // 最近更新时间（自动更新）
}

// PageFields 分页查询参数。
type PageFields struct {
	PageNum  int      // 当前页码，从1开始
	PageSize int      // 每页记录数
	Cols     []string // 查询的列名列表，为空时查询所有列
}

// CursorFields 游标分页查询参数，适用于大数据集的高效翻页。
type CursorFields struct {
	Before   int64    // 上一页游标，用于向前翻页
	After    int64    // 下一页游标，用于向后翻页
	PageSize int      // 每页记录数
	Cols     []string // 查询的列名列表，为空时查询所有列
}

type OperatorFields struct {
	Creator string `json:"creator" xorm:"varchar(64) comment('创建人')"` // 创建人
	Updater string `json:"updater" xorm:"varchar(64) comment('编辑人')"` // 最后编辑人
}
