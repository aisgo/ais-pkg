package repository

import (
	"context"
	stderrors "errors"
	"fmt"

	"gorm.io/gorm"
)

/* ========================================================================
 * Repository Interfaces - 仓储接口定义
 * ========================================================================
 * 职责: 定义通用仓储接口
 * 设计: 使用泛型提供类型安全的数据访问
 * ======================================================================== */

// QueryOption 查询选项
type QueryOption struct {
	// Preloads 预加载关联（如 "User", "User.Profile"）
	Preloads []string
	// Scopes 查询作用域（如软删除、租户过滤）
	Scopes []func(*gorm.DB) *gorm.DB
	// OrderBy 排序（如 "created_at DESC"）
	OrderBy string
	// Select 选择字段（如 "id, name, email"）
	Select []string
	// Joins 连接查询（如 "JOIN orders ON orders.user_id = users.id"）
	Joins []string

	validationErr error
}

// Option 应用查询选项
type Option func(*QueryOption)

// WithPreloads 设置预加载
func WithPreloads(preloads ...string) Option {
	return func(o *QueryOption) {
		o.Preloads = preloads
	}
}

// WithScopes 设置查询作用域
func WithScopes(scopes ...func(*gorm.DB) *gorm.DB) Option {
	return func(o *QueryOption) {
		o.Scopes = scopes
	}
}

// WithOrderBy 设置排序
// 自动校验列名和排序方向，防止 SQL 注入
// 允许格式: "column ASC", "table.column DESC", "col1 ASC, col2 DESC"
func WithOrderBy(orderBy string) Option {
	return func(o *QueryOption) {
		if err := ValidateOrderBy(orderBy); err != nil {
			o.addValidationError(fmt.Errorf("order by: %w", err))
			return
		}
		o.OrderBy = orderBy
	}
}

// WithSelect 设置选择字段
// 自动校验列名，防止 SQL 注入
// 允许格式: "id", "table.column", "COUNT(*) AS count"
func WithSelect(selects ...string) Option {
	return func(o *QueryOption) {
		if err := ValidateSelect(selects); err != nil {
			o.addValidationError(fmt.Errorf("select: %w", err))
			return
		}
		o.Select = selects
	}
}

// WithJoins 设置连接查询
// 自动校验 JOIN 语法，防止 SQL 注入
// 允许格式: "LEFT JOIN orders ON orders.user_id = users.id"
func WithJoins(joins ...string) Option {
	return func(o *QueryOption) {
		if err := ValidateJoins(joins); err != nil {
			o.addValidationError(fmt.Errorf("joins: %w", err))
			return
		}
		o.Joins = joins
	}
}

// ApplyOptions 应用查询选项
func ApplyOptions(opts []Option) *QueryOption {
	o := &QueryOption{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func (o *QueryOption) addValidationError(err error) {
	if o == nil || err == nil {
		return
	}
	o.validationErr = stderrors.Join(o.validationErr, err)
}

func (o *QueryOption) validationError() error {
	if o == nil {
		return nil
	}
	return o.validationErr
}

// PageResult 分页结果
type PageResult[T any] struct {
	List     []T   `json:"list" doc:"数据列表"`
	Total    int64 `json:"total" doc:"总记录数"`
	Page     int   `json:"page" doc:"当前页码"`
	PageSize int   `json:"page_size" doc:"每页大小"`
	Pages    int64 `json:"pages" doc:"总页数"`
}

// CRUDRepository CRUD 操作接口
type CRUDRepository[T any] interface {
	// Create 创建单条记录
	Create(ctx context.Context, model *T) error

	// CreateBatch 批量创建记录
	CreateBatch(ctx context.Context, models []*T, batchSize int) error

	// Update 更新记录（根据主键）
	// 注意: 使用 Updates 语义，默认忽略零值字段
	Update(ctx context.Context, model *T) error

	// UpdateByID 根据 ID 更新指定字段
	UpdateByID(ctx context.Context, id string, updates map[string]any, allowedFields ...string) error

	// UpsertBatch 批量更新或插入记录 (Upsert)
	UpsertBatch(ctx context.Context, models []*T) error

	// Delete 软删除记录（设置 deleted_at）
	Delete(ctx context.Context, id string) error

	// DeleteBatch 批量软删除记录
	DeleteBatch(ctx context.Context, ids []string) error

	// HardDelete 硬删除记录（从数据库移除）
	HardDelete(ctx context.Context, id string) error
}

// QueryRepository 查询操作接口
type QueryRepository[T any] interface {
	// FindByID 根据 ID 查找记录
	FindByID(ctx context.Context, id string, opts ...Option) (*T, error)

	// FindByIDs 根据 ID 列表查找多条记录
	FindByIDs(ctx context.Context, ids []string, opts ...Option) ([]*T, error)

	// FindOneByCondition 使用结构化条件查找单条记录（推荐）。
	FindOneByCondition(ctx context.Context, where any, opts ...Option) (*T, error)

	// FindByCondition 使用结构化条件查找多条记录（推荐）。
	FindByCondition(ctx context.Context, where any, opts ...Option) ([]*T, error)

	// CountByCondition 使用结构化条件统计记录数（推荐）。
	CountByCondition(ctx context.Context, where any) (int64, error)

	// ExistsByCondition 使用结构化条件检查记录是否存在（推荐）。
	ExistsByCondition(ctx context.Context, where any) (bool, error)

	// FindOne 查找单条记录（使用原始 WHERE 字符串）。
	// Deprecated: Unsafe raw query path. Prefer FindOneByCondition.
	FindOne(ctx context.Context, query string, args ...any) (*T, error)

	// FindOneWithOpts 查找单条记录（带选项，使用原始 WHERE 字符串）。
	// Deprecated: Unsafe raw query path. Prefer FindOneByCondition.
	FindOneWithOpts(ctx context.Context, query string, opts []Option, args ...any) (*T, error)

	// FindByQuery 查找多条记录（使用原始 WHERE 字符串）。
	// Deprecated: Unsafe raw query path. Prefer FindByCondition.
	FindByQuery(ctx context.Context, query string, args ...any) ([]*T, error)

	// FindByQueryWithOpts 查找多条记录（带选项，使用原始 WHERE 字符串）。
	// Deprecated: Unsafe raw query path. Prefer FindByCondition.
	FindByQueryWithOpts(ctx context.Context, query string, opts []Option, args ...any) ([]*T, error)

	// Count 统计记录数（使用原始 WHERE 字符串）。
	// Deprecated: Unsafe raw query path. Prefer CountByCondition.
	Count(ctx context.Context, query string, args ...any) (int64, error)

	// Exists 检查记录是否存在（使用原始 WHERE 字符串）。
	// Deprecated: Unsafe raw query path. Prefer ExistsByCondition.
	Exists(ctx context.Context, query string, args ...any) (bool, error)
}

// PageRepository 分页查询接口
type PageRepository[T any] interface {
	// FindPageByModel 使用结构化条件分页查询（推荐）。
	FindPageByModel(ctx context.Context, page, pageSize int, model any, opts ...Option) (*PageResult[T], error)

	// FindPage 分页查询
	// Deprecated: Unsafe raw query path. Prefer FindPageByModel.
	FindPage(ctx context.Context, page, pageSize int, query string, args ...any) (*PageResult[T], error)

	// FindPageWithOpts 分页查询（带选项）
	// Deprecated: Unsafe raw query path. Prefer FindPageByModel.
	FindPageWithOpts(ctx context.Context, page, pageSize int, query string, opts []Option, args ...any) (*PageResult[T], error)
}

// AggregateRepository 聚合查询接口
type AggregateRepository[T any] interface {
	// SumWithCondition 使用结构化条件求和（推荐）。
	SumWithCondition(ctx context.Context, column string, where any, opts ...Option) (float64, error)

	// AvgWithCondition 使用结构化条件求平均值（推荐）。
	AvgWithCondition(ctx context.Context, column string, where any, opts ...Option) (float64, error)

	// MaxWithCondition 使用结构化条件求最大值（推荐）。
	MaxWithCondition(ctx context.Context, column string, where any, opts ...Option) (any, error)

	// MinWithCondition 使用结构化条件求最小值（推荐）。
	MinWithCondition(ctx context.Context, column string, where any, opts ...Option) (any, error)

	// Sum 求和
	// Deprecated: Unsafe raw query path. Prefer SumWithCondition.
	Sum(ctx context.Context, column string, query string, args ...any) (float64, error)

	// Avg 平均值
	// Deprecated: Unsafe raw query path. Prefer AvgWithCondition.
	Avg(ctx context.Context, column string, query string, args ...any) (float64, error)

	// Max 最大值
	// Deprecated: Unsafe raw query path. Prefer MaxWithCondition.
	Max(ctx context.Context, column string, query string, args ...any) (any, error)

	// Min 最小值
	// Deprecated: Unsafe raw query path. Prefer MinWithCondition.
	Min(ctx context.Context, column string, query string, args ...any) (any, error)
}

// TransactionRepository 事务支持接口
type TransactionRepository[T any] interface {
	// Execute 在事务中执行操作（支持隐式事务传播）
	Execute(ctx context.Context, fn func(ctx context.Context) error) error

	// Transaction 在事务中执行操作 (Deprecated: Use Execute instead)
	Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error

	// WithTx 创建事务版本的仓储
	WithTx(tx *gorm.DB) Repository[T]
}

// Repository 通用仓储接口
// 组合了所有子接口
type Repository[T any] interface {
	CRUDRepository[T]
	QueryRepository[T]
	PageRepository[T]
	AggregateRepository[T]
	TransactionRepository[T]

	// GetDB 获取底层 GORM DB 实例（用于复杂查询）
	GetDB() *gorm.DB
}
