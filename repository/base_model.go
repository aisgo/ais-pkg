package repository

import (
	"time"

	"github.com/aisgo/ais-pkg/ulid"
	"gorm.io/gorm"
	"gorm.io/plugin/soft_delete"
)

/* ========================================================================
 * Base Model - 基础模型
 * ========================================================================
 * 职责: 定义所有模型的公共字段和方法
 * 使用: 所有 GORM 模型都应嵌入此结构体
 * 字段: 与其他微服务 BaseEntity 保持一致
 * ======================================================================== */

// BaseModel 所有模型的基类
// 包含通用字段：ID、创建时间、更新时间、软删除标记
//
// ID 字段使用 ulid.ID 类型，存储为 PostgreSQL bytea (16 bytes)
// JSON 序列化时输出为可读的 ULID 字符串格式（26 字符）
//
// 软删除设计说明：
//   - Deleted 字段类型为 soft_delete.DeletedAt，配合 softDelete:flag 标签使用 0/1 整数标记。
//   - 使用 flag 模式（而非 unix-time 模式）可避免将删除时间戳暴露给 JSON 消费方，
//     同时保证跨数据库（MySQL TINYINT、PostgreSQL SMALLINT）的兼容性。
//   - json:"-" 确保 Deleted 字段不被序列化到 API 响应中，防止内部删除状态泄露。
//   - 若需要 unix-time 模式（softDelete:unix）则需同步去掉 json:"-"，
//     否则前端/gRPC 消费方将看到整型时间戳而非布尔标记，可能引发类型不兼容。
type BaseModel struct {
	ID         ulid.ID               `json:"id" gorm:"type:bytea;primaryKey;comment:主键ID(ULID)"`
	CreateTime time.Time             `json:"create_time" gorm:"column:create_time;autoCreateTime;comment:创建时间"`
	UpdateTime time.Time             `json:"update_time" gorm:"column:update_time;autoUpdateTime;comment:更新时间"`
	Deleted    soft_delete.DeletedAt `json:"-" gorm:"column:deleted;default:0;softDelete:flag;comment:软删除标记(1=已删除)"`
}

// BeforeCreate GORM 钩子：在创建记录前自动生成 ULID
// ULID 特性: 时间排序、URL 安全、大小写不敏感、128 位唯一性
func (m *BaseModel) BeforeCreate(tx *gorm.DB) error {
	if m.ID.IsZero() {
		m.ID = ulid.NewID()
	}
	return nil
}
