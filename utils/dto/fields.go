// Package dto 提供数据传输对象（DTO）的通用字段定义。
// 用于在 API 响应中格式化时间字段，将从数据库模型读取的 time.Time 类型
// 转换为标准字符串格式返回给客户端。
package dto

import (
	"time"

	"github.com/LeeZXin/zsf/utils/model"
)

// AuditFields 审计字段DTO，包含创建和更新信息。
// 与 model.AuditFields 对应，但时间字段已格式化为字符串。
type AuditFields struct {
	Creator string `json:"creator"` // 创建人
	Updater string `json:"updater"` // 最后编辑人
	Created string `json:"created"` // 创建时间（格式化后的字符串）
	Updated string `json:"updated"` // 最近更新时间（格式化后的字符串）
}

// NewAuditFields 将模型层的审计字段转换为DTO层表示。
// 参数:
//   - fields: 模型层的AuditFields，包含time.Time类型的时间字段
//
// 返回值: 格式化后的AuditFields DTO
func NewAuditFields(fields model.AuditFields) AuditFields {
	return AuditFields{
		Creator: fields.Creator,
		Updater: fields.Updater,
		Created: FormatTime(fields.Created),
		Updated: FormatTime(fields.Updated),
	}
}

// TimeFields 时间字段DTO，仅包含创建和更新时间。
// 用于简化场景下仅返回时间信息的场景。
type TimeFields struct {
	Created string `json:"created"` // 创建时间（格式化后的字符串）
	Updated string `json:"updated"` // 最近更新时间（格式化后的字符串）
}

// NewTimeFields 将模型层的时间字段转换为DTO层表示。
// 参数:
//   - fields: 模型层的TimeFields，包含time.Time类型的时间字段
//
// 返回值: 格式化后的TimeFields DTO
func NewTimeFields(fields model.TimeFields) TimeFields {
	return TimeFields{
		Created: FormatTime(fields.Created),
		Updated: FormatTime(fields.Updated),
	}
}

// FormatTime 将 time.Time 格式化为标准日期时间字符串。
// 如果时间为零值（未设置），返回空字符串。
// 格式: "2006-01-02 15:04:05" (time.DateTime)
// 参数:
//   - t: 待格式化的时间
//
// 返回值: 格式化后的字符串
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.DateTime)
}

type OperatorFields struct {
	Creator string `json:"creator"` // 创建人
	Updater string `json:"updater"` // 最后编辑人
}

func NewOperatorFields(fields model.OperatorFields) OperatorFields {
	return OperatorFields{
		Creator: fields.Creator,
		Updater: fields.Updater,
	}
}
