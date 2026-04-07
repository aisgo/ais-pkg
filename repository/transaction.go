package repository

import (
	"context"

	"github.com/aisgo/ais-pkg/errors"

	"gorm.io/gorm"
)

/* ========================================================================
 * Transaction Repository Implementation - 事务支持实现
 * ========================================================================
 * 职责: 实现 TransactionRepository 接口
 * ======================================================================== */

// Transaction 在事务中执行操作
// Deprecated: 请使用 Execute 方法以支持隐式事务传播
func (r *RepositoryImpl[T]) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	db := r.withContext(ctx)

	return db.Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	})
}

// Execute 在事务中执行操作（支持隐式事务传播，语义为 REQUIRED）。
// 若 ctx 中已存在事务，则直接加入该事务，不再创建嵌套事务（避免 SavePoint 行为不一致）。
// 若无事务，则开启新事务，fn 执行失败时自动回滚。
func (r *RepositoryImpl[T]) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	// 已有事务：直接加入，由外层事务控制提交/回滚
	if HasTransaction(ctx) {
		return fn(ctx)
	}

	// 无事务：开启新事务，GORM 自动提交或回滚
	db := r.withContext(ctx)
	return db.Transaction(func(tx *gorm.DB) error {
		// 将新事务注入 context，传递给下层调用
		ctxWithTx := context.WithValue(ctx, ctxTxKey{}, tx)
		return fn(ctxWithTx)
	})
}

// WithTx 创建事务版本的仓储
// 返回的仓储实例使用传入的事务 DB，同时保留原实例的所有配置（如 strictUpdates）
func (r *RepositoryImpl[T]) WithTx(tx *gorm.DB) Repository[T] {
	return &RepositoryImpl[T]{
		db:            tx,
		strictUpdates: r.strictUpdates,
	}
}

/* ========================================================================
 * 事务辅助方法
 * ======================================================================== */

// TransactionContext 事务上下文
// 用于在复杂业务场景中传递事务
type TransactionContext struct {
	tx *gorm.DB
}

// NewTransactionContext 创建事务上下文
func NewTransactionContext(tx *gorm.DB) *TransactionContext {
	return &TransactionContext{tx: tx}
}

// GetTx 获取事务 DB
func (tc *TransactionContext) GetTx() *gorm.DB {
	return tc.tx
}

// HasTx 检查是否有事务
func (tc *TransactionContext) HasTx() bool {
	return tc.tx != nil
}

// ExecInTransaction 在事务中执行操作（使用 TransactionContext）
func (r *RepositoryImpl[T]) ExecInTransaction(ctx context.Context, fn func(tc *TransactionContext) error) error {
	db := r.withContext(ctx)

	if err := db.Transaction(func(tx *gorm.DB) error {
		return fn(&TransactionContext{tx: tx})
	}); err != nil {
		return errors.Wrap(errors.ErrCodeInternal, "transaction failed", err)
	}

	return nil
}

// WithTxContext 创建带事务上下文的仓储
// 如果 tc 有事务，使用事务 DB 并保留原实例的所有配置；否则使用普通 DB
func (r *RepositoryImpl[T]) WithTxContext(tc *TransactionContext) Repository[T] {
	if tc != nil && tc.HasTx() {
		return &RepositoryImpl[T]{
			db:            tc.GetTx(),
			strictUpdates: r.strictUpdates,
		}
	}
	return r
}
