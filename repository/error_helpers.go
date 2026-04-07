package repository

import (
	"context"
	stderrors "errors"
	"strings"

	pkgerrors "github.com/aisgo/ais-pkg/errors"
	"gorm.io/gorm"
)

func normalizeRepositoryError(err error, message string) error {
	if err == nil {
		return nil
	}
	if bizErr, ok := pkgerrors.AsBizError(err); ok {
		return bizErr
	}

	var validationErr *ValidationError
	switch {
	case stderrors.As(err, &validationErr):
		return pkgerrors.Wrap(pkgerrors.ErrCodeInvalidArgument, message, err)
	case stderrors.Is(err, gorm.ErrRecordNotFound):
		return pkgerrors.Wrap(pkgerrors.ErrCodeNotFound, message, err)
	case stderrors.Is(err, context.DeadlineExceeded):
		return pkgerrors.Wrap(pkgerrors.ErrCodeTimeout, message, err)
	case stderrors.Is(err, context.Canceled):
		return pkgerrors.Wrap(pkgerrors.ErrCodeCanceled, message, err)
	default:
		return pkgerrors.Wrap(pkgerrors.ErrCodeInternal, message, err)
	}
}

func applyQueryOptionValidation(db *gorm.DB, opts *QueryOption) *gorm.DB {
	if err := opts.validationError(); err != nil {
		db.AddError(pkgerrors.Wrap(pkgerrors.ErrCodeInvalidArgument, "invalid query options", err))
	}
	return db
}

func applyStructuredCondition(db *gorm.DB, where any) *gorm.DB {
	if query, ok := where.(string); ok {
		// 空字符串视为无条件（等同于 nil），非空字符串拒绝以防止原始 SQL 注入
		if strings.TrimSpace(query) != "" {
			db.AddError(pkgerrors.New(pkgerrors.ErrCodeInvalidArgument, "structured conditions do not accept raw SQL strings"))
		}
		return db
	}
	if where != nil {
		return db.Where(where)
	}
	return db
}

func applyUnsafeWhere(db *gorm.DB, query string, args ...any) *gorm.DB {
	if strings.TrimSpace(query) == "" {
		return db
	}
	return db.Where(query, args...)
}
