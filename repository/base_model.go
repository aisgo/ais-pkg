package repository

import (
	"time"

	"github.com/aisgo/ais-pkg/ulid"
	"gorm.io/gorm"
)

/* ========================================================================
 * Base Model - 基础模型
 * ========================================================================
 * 职责: 定义所有模型的公共字段和方法
 * 使用: 所有 GORM 模型都应嵌入此结构体
 * 字段: 与其他微服务 BaseEntity 保持一致
 * ======================================================================== */

// BaseModel 所有模型的基类
// 包含通用字段：ID、CreatedAt、UpdatedAt、DeletedAt
//
// ID 字段使用 ulid.ID 类型，存储为 PostgreSQL bytea (16 bytes)
// JSON 序列化时输出为可读的 ULID 字符串格式（26 字符）
//
// 软删除设计说明：
//   - 使用标准的时间列语义：created_at、updated_at、deleted_at。
//   - DeletedAt 使用 gorm.DeletedAt 参与 GORM 软删除机制，删除时写入实际时间。
//   - json:"-" 确保软删除状态不暴露到 API 响应中。
type BaseModel struct {
	ID        ulid.ID        `json:"id" gorm:"type:bytea;primaryKey;comment:主键ID(ULID)"`
	CreatedAt time.Time      `json:"created_at" gorm:"column:created_at;autoCreateTime;comment:创建时间"`
	UpdatedAt time.Time      `json:"updated_at" gorm:"column:updated_at;autoUpdateTime;comment:更新时间"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"column:deleted_at;index;comment:删除时间"`
}

// BeforeCreate GORM 钩子：在创建记录前自动生成 ULID
// ULID 特性: 时间排序、URL 安全、大小写不敏感、128 位唯一性
func (m *BaseModel) BeforeCreate(tx *gorm.DB) error {
	if m.ID.IsZero() {
		m.ID = ulid.NewID()
	}
	return nil
}
