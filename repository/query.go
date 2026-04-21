package repository

import (
	"context"
	"reflect"

	pkgerrors "github.com/aisgo/ais-pkg/errors"
	"github.com/aisgo/ais-pkg/ulid"
	"gorm.io/gorm"
)

/* ========================================================================
 * Query Repository Implementation - 查询操作实现
 * ========================================================================
 * 职责: 实现 QueryRepository 接口
 * ======================================================================== */

// buildQuery 构建查询
func (r *RepositoryImpl[T]) buildQuery(ctx context.Context, opts *QueryOption) *gorm.DB {
	db := r.withContext(ctx)
	db = r.applyTenantScope(ctx, db)

	if opts == nil {
		return db
	}

	db = applyQueryOptionValidation(db, opts)

	// 应用选择字段
	if len(opts.Select) > 0 {
		db = db.Select(opts.Select)
	}

	// 应用连接查询
	for _, join := range opts.Joins {
		db = db.Joins(join)
	}

	// 应用排序
	if opts.OrderBy != "" {
		db = db.Order(opts.OrderBy)
	}

	// 应用作用域
	for _, scope := range opts.Scopes {
		db = scope(db)
	}

	// 应用预加载
	for _, preload := range opts.Preloads {
		db = db.Preload(preload)
	}

	return db
}

func isULIDPrimaryKeyType(fieldType reflect.Type) bool {
	return fieldType.PkgPath() == "github.com/aisgo/ais-pkg/ulid" && fieldType.Name() == "ID"
}

func (r *RepositoryImpl[T]) normalizePrimaryID(id string) (any, error) {
	schema, err := r.getSchema()
	if err != nil {
		return nil, err
	}

	if schema == nil || schema.PrioritizedPrimaryField == nil {
		return id, nil
	}

	if isULIDPrimaryKeyType(schema.PrioritizedPrimaryField.FieldType) {
		parsedID, err := ulid.ParseID(id)
		if err != nil {
			return nil, pkgerrors.Wrap(pkgerrors.ErrCodeInvalidArgument, "invalid record id", err)
		}
		return parsedID, nil
	}

	return id, nil
}

func (r *RepositoryImpl[T]) normalizePrimaryIDs(ids []string) ([]any, error) {
	normalized := make([]any, 0, len(ids))
	for _, id := range ids {
		value, err := r.normalizePrimaryID(id)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, value)
	}
	return normalized, nil
}

/* ========================================================================
 * FindByID 操作
 * ======================================================================== */

// FindByID 根据 ID 查找记录
func (r *RepositoryImpl[T]) FindByID(ctx context.Context, id string, opts ...Option) (*T, error) {
	opt := ApplyOptions(opts)
	model := r.newModelPtr()
	normalizedID, err := r.normalizePrimaryID(id)
	if err != nil {
		return nil, normalizeRepositoryError(err, "invalid record id")
	}

	query := r.buildQuery(ctx, opt)
	if err := query.Where("id = ?", normalizedID).First(model).Error; err != nil {
		return nil, normalizeRepositoryError(err, "failed to find record")
	}

	return model, nil
}

// FindByIDs 根据 ID 列表查找多条记录
func (r *RepositoryImpl[T]) FindByIDs(ctx context.Context, ids []string, opts ...Option) ([]*T, error) {
	if len(ids) == 0 {
		return []*T{}, nil
	}

	opt := ApplyOptions(opts)
	var models []*T
	normalizedIDs, err := r.normalizePrimaryIDs(ids)
	if err != nil {
		return nil, normalizeRepositoryError(err, "invalid record ids")
	}

	query := r.buildQuery(ctx, opt)
	if err := query.Where("id IN ?", normalizedIDs).Find(&models).Error; err != nil {
		return nil, normalizeRepositoryError(err, "failed to find records")
	}

	return models, nil
}

/* ========================================================================
 * FindOne 操作
 * ======================================================================== */

// FindOneByCondition 使用结构化条件查找单条记录（推荐）
func (r *RepositoryImpl[T]) FindOneByCondition(ctx context.Context, where any, opts ...Option) (*T, error) {
	opt := ApplyOptions(opts)
	model := r.newModelPtr()
	db := applyStructuredCondition(r.buildQuery(ctx, opt), where)

	if err := db.First(model).Error; err != nil {
		return nil, normalizeRepositoryError(err, "failed to find record")
	}

	return model, nil
}

// FindByCondition 使用结构化条件查找多条记录（推荐）
func (r *RepositoryImpl[T]) FindByCondition(ctx context.Context, where any, opts ...Option) ([]*T, error) {
	opt := ApplyOptions(opts)
	var models []*T
	db := applyStructuredCondition(r.buildQuery(ctx, opt), where)

	if err := db.Find(&models).Error; err != nil {
		return nil, normalizeRepositoryError(err, "failed to find records")
	}

	return models, nil
}

// FindOne 查找单条记录（使用自定义条件）
func (r *RepositoryImpl[T]) FindOne(ctx context.Context, query string, args ...any) (*T, error) {
	return r.FindOneWithOpts(ctx, query, nil, args...)
}

// FindOneWithOpts 查找单条记录（带选项）
func (r *RepositoryImpl[T]) FindOneWithOpts(ctx context.Context, query string, opts []Option, args ...any) (*T, error) {
	var opt *QueryOption
	if len(opts) > 0 {
		opt = ApplyOptions(opts)
	}

	model := r.newModelPtr()
	db := r.buildQuery(ctx, opt)

	if err := applyUnsafeWhere(db, query, args...).First(model).Error; err != nil {
		return nil, normalizeRepositoryError(err, "failed to find record")
	}

	return model, nil
}

/* ========================================================================
 * FindByQuery 操作
 * ======================================================================== */

// FindByQuery 查找多条记录（使用自定义条件）
func (r *RepositoryImpl[T]) FindByQuery(ctx context.Context, query string, args ...any) ([]*T, error) {
	return r.FindByQueryWithOpts(ctx, query, nil, args...)
}

// FindByQueryWithOpts 查找多条记录（带选项）
func (r *RepositoryImpl[T]) FindByQueryWithOpts(ctx context.Context, query string, opts []Option, args ...any) ([]*T, error) {
	var opt *QueryOption
	if len(opts) > 0 {
		opt = ApplyOptions(opts)
	}

	var models []*T
	db := r.buildQuery(ctx, opt)

	if err := applyUnsafeWhere(db, query, args...).Find(&models).Error; err != nil {
		return nil, normalizeRepositoryError(err, "failed to find records")
	}

	return models, nil
}

/* ========================================================================
 * Count/Exists 操作
 * ======================================================================== */

// Count 统计记录数
func (r *RepositoryImpl[T]) Count(ctx context.Context, query string, args ...any) (int64, error) {
	db := applyUnsafeWhere(r.applyTenantScope(ctx, r.withContext(ctx)), query, args...)
	return r.countWithDB(db)
}

// CountByCondition 使用结构化条件统计记录数（推荐）
func (r *RepositoryImpl[T]) CountByCondition(ctx context.Context, where any) (int64, error) {
	db := applyStructuredCondition(r.applyTenantScope(ctx, r.withContext(ctx)), where)
	return r.countWithDB(db)
}

func (r *RepositoryImpl[T]) countWithDB(db *gorm.DB) (int64, error) {
	var count int64
	if err := db.Model(r.newModelPtr()).Count(&count).Error; err != nil {
		return 0, normalizeRepositoryError(err, "failed to count records")
	}

	return count, nil
}

// Exists 检查记录是否存在
func (r *RepositoryImpl[T]) Exists(ctx context.Context, query string, args ...any) (bool, error) {
	count, err := r.Count(ctx, query, args...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ExistsByCondition 使用结构化条件检查记录是否存在（推荐）
func (r *RepositoryImpl[T]) ExistsByCondition(ctx context.Context, where any) (bool, error) {
	count, err := r.CountByCondition(ctx, where)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
