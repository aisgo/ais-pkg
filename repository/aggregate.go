package repository

import (
	"context"
	"database/sql"
	"regexp"
	"strings"

	"github.com/aisgo/ais-pkg/errors"
)

/* ========================================================================
 * Aggregate Repository Implementation - 聚合查询实现
 * ========================================================================
 * 职责: 实现 AggregateRepository 接口
 * 安全: 对列名进行白名单验证，防止 SQL 注入
 * ======================================================================== */

// columnRegex 列名正则表达式（只允许字母、数字、下划线）
// 注意: 此正则是列名拼接进 SQL 表达式前的唯一安全保障。
// 经过此验证后，列名仅包含 [a-zA-Z_][a-zA-Z0-9_]*，
// 不含任何 SQL 元字符（引号、括号、分号、空格等），因此字符串拼接是安全的。
var columnRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// sqlKeywordsBlocklist SQL 保留关键字黑名单
// 阻止将敏感关键字作为列名传入，防止行为歧义（如 SLEEP、COUNT 等函数名误用为列名）
var sqlKeywordsBlocklist = map[string]struct{}{
	"SLEEP": {}, "BENCHMARK": {}, "LOAD_FILE": {}, "OUTFILE": {}, "DUMPFILE": {},
	"COUNT": {}, "SUM": {}, "AVG": {}, "MAX": {}, "MIN": {},
	"SELECT": {}, "INSERT": {}, "UPDATE": {}, "DELETE": {}, "DROP": {},
	"CREATE": {}, "ALTER": {}, "EXEC": {}, "EXECUTE": {}, "UNION": {},
}

// validateColumn 验证列名是否安全
func validateColumn(column string) error {
	if column == "" {
		return errors.New(errors.ErrCodeInvalidArgument, "column cannot be empty")
	}
	if strings.Contains(column, ".") {
		return errors.New(errors.ErrCodeInvalidArgument, "column must not contain table qualifier")
	}
	if !columnRegex.MatchString(column) {
		return errors.New(errors.ErrCodeInvalidArgument, "invalid column name: "+column)
	}
	// 检查 SQL 关键字黑名单，防止函数名/保留字被误用为列名
	if _, blocked := sqlKeywordsBlocklist[strings.ToUpper(column)]; blocked {
		return errors.New(errors.ErrCodeInvalidArgument, "column name conflicts with SQL keyword: "+column)
	}
	return nil
}

// Sum 求和
func (r *RepositoryImpl[T]) Sum(ctx context.Context, column string, query string, args ...any) (float64, error) {
	if err := validateColumn(column); err != nil {
		return 0, err
	}

	var result float64
	db := r.applyTenantScope(ctx, r.withContext(ctx))
	db = applyUnsafeWhere(db, query, args...)

	// 使用 GORM 的 Raw 方法确保列名安全
	sql := "COALESCE(SUM(" + column + "), 0)"
	if err := db.Model(r.newModelPtr()).Select(sql).Scan(&result).Error; err != nil {
		return 0, normalizeRepositoryError(err, "failed to sum records")
	}

	return result, nil
}

// Avg 平均值
func (r *RepositoryImpl[T]) Avg(ctx context.Context, column string, query string, args ...any) (float64, error) {
	if err := validateColumn(column); err != nil {
		return 0, err
	}

	var result float64
	db := r.applyTenantScope(ctx, r.withContext(ctx))
	db = applyUnsafeWhere(db, query, args...)

	sql := "COALESCE(AVG(" + column + "), 0)"
	if err := db.Model(r.newModelPtr()).Select(sql).Scan(&result).Error; err != nil {
		return 0, normalizeRepositoryError(err, "failed to average records")
	}

	return result, nil
}

// Max 最大值
// 返回值类型取决于数据库驱动的扫描结果（int64/float64/string/[]byte/time.Time 等）
// 无记录时返回 nil
func (r *RepositoryImpl[T]) Max(ctx context.Context, column string, query string, args ...any) (any, error) {
	if err := validateColumn(column); err != nil {
		return nil, err
	}

	var result any
	db := r.applyTenantScope(ctx, r.withContext(ctx))
	db = applyUnsafeWhere(db, query, args...)

	sqlQuery := "MAX(" + column + ")"
	row := db.Model(r.newModelPtr()).Select(sqlQuery).Row()
	if err := row.Scan(&result); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, normalizeRepositoryError(err, "failed to get max value")
	}

	if result == nil {
		return nil, nil
	}
	return result, nil
}

// Min 最小值
// 返回值类型取决于数据库驱动的扫描结果（int64/float64/string/[]byte/time.Time 等）
// 无记录时返回 nil
func (r *RepositoryImpl[T]) Min(ctx context.Context, column string, query string, args ...any) (any, error) {
	if err := validateColumn(column); err != nil {
		return nil, err
	}

	var result any
	db := r.applyTenantScope(ctx, r.withContext(ctx))
	db = applyUnsafeWhere(db, query, args...)

	sqlQuery := "MIN(" + column + ")"
	row := db.Model(r.newModelPtr()).Select(sqlQuery).Row()
	if err := row.Scan(&result); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, normalizeRepositoryError(err, "failed to get min value")
	}

	if result == nil {
		return nil, nil
	}
	return result, nil
}

// CountByGroup 分组统计
// 用于类似 GROUP BY COUNT(*) 的查询
func (r *RepositoryImpl[T]) CountByGroup(ctx context.Context, groupColumn, query string, args ...any) (map[string]int64, error) {
	if err := validateColumn(groupColumn); err != nil {
		return nil, err
	}

	type Result struct {
		Group string `gorm:"column:group_column"`
		Count int64
	}

	var results []Result
	db := r.applyTenantScope(ctx, r.withContext(ctx))
	db = applyUnsafeWhere(db, query, args...)

	// 安全的列名使用
	sql := groupColumn + " as group_column, COUNT(*) as count"
	if err := db.Model(r.newModelPtr()).
		Select(sql).
		Group(groupColumn).
		Scan(&results).Error; err != nil {
		return nil, normalizeRepositoryError(err, "failed to count by group")
	}

	resultMap := make(map[string]int64)
	for _, r := range results {
		resultMap[r.Group] = r.Count
	}

	return resultMap, nil
}

// SumWithCondition 带条件的求和（推荐使用）
// 使用结构体作为查询条件，更安全；禁止传入原始 SQL 字符串
func (r *RepositoryImpl[T]) SumWithCondition(ctx context.Context, column string, where any, opts ...Option) (float64, error) {
	if err := validateColumn(column); err != nil {
		return 0, err
	}

	var result float64
	db := r.buildQuery(ctx, ApplyOptions(opts))
	db = applyStructuredCondition(db, where)

	sql := "COALESCE(SUM(" + column + "), 0)"
	if err := db.Model(r.newModelPtr()).Select(sql).Scan(&result).Error; err != nil {
		return 0, normalizeRepositoryError(err, "failed to sum records")
	}

	return result, nil
}

// AvgWithCondition 带条件的平均值（推荐使用）
// 使用结构体作为查询条件，更安全；禁止传入原始 SQL 字符串
func (r *RepositoryImpl[T]) AvgWithCondition(ctx context.Context, column string, where any, opts ...Option) (float64, error) {
	if err := validateColumn(column); err != nil {
		return 0, err
	}

	var result float64
	db := r.buildQuery(ctx, ApplyOptions(opts))
	db = applyStructuredCondition(db, where)

	sql := "COALESCE(AVG(" + column + "), 0)"
	if err := db.Model(r.newModelPtr()).Select(sql).Scan(&result).Error; err != nil {
		return 0, normalizeRepositoryError(err, "failed to average records")
	}

	return result, nil
}

// MaxWithCondition 带条件的最大值（推荐使用）
// 使用结构体作为查询条件，更安全；禁止传入原始 SQL 字符串
func (r *RepositoryImpl[T]) MaxWithCondition(ctx context.Context, column string, where any, opts ...Option) (any, error) {
	if err := validateColumn(column); err != nil {
		return nil, err
	}

	var result any
	db := r.buildQuery(ctx, ApplyOptions(opts))
	db = applyStructuredCondition(db, where)

	sql := "MAX(" + column + ")"
	if err := db.Model(r.newModelPtr()).Select(sql).Scan(&result).Error; err != nil {
		return nil, normalizeRepositoryError(err, "failed to get max value")
	}

	return result, nil
}

// MinWithCondition 带条件的最小值（推荐使用）
// 使用结构体作为查询条件，更安全；禁止传入原始 SQL 字符串
func (r *RepositoryImpl[T]) MinWithCondition(ctx context.Context, column string, where any, opts ...Option) (any, error) {
	if err := validateColumn(column); err != nil {
		return nil, err
	}

	var result any
	db := r.buildQuery(ctx, ApplyOptions(opts))
	db = applyStructuredCondition(db, where)

	sql := "MIN(" + column + ")"
	if err := db.Model(r.newModelPtr()).Select(sql).Scan(&result).Error; err != nil {
		return nil, normalizeRepositoryError(err, "failed to get min value")
	}

	return result, nil
}

// IsSafeColumnName 检查列名是否安全（用于调用方验证）
func IsSafeColumnName(column string) bool {
	return columnRegex.MatchString(column)
}

// SanitizeColumnName 清理列名，移除不安全字符
func SanitizeColumnName(column string) string {
	// 移除所有非字母数字下划线字符
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return -1
	}, column)
}
