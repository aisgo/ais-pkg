package middleware

import (
	"errors"
	"net/http"

	"github.com/aisgo/ais-pkg/logger"
	"github.com/aisgo/ais-pkg/response"
	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

/* ========================================================================
 * Error Handler - 统一错误处理中间件
 * ========================================================================
 * 职责: 捕获并处理所有未处理的错误
 * 特性:
 *   - 特殊处理 Fiber 错误（保留状态码和消息）
 *   - 统一日志记录
 *   - 统一响应格式
 * ======================================================================== */

// NewErrorHandler returns a Fiber ErrorHandler with unified logging and response formatting.
//
// 特殊处理 fiber.Error:
//   - 保留原始 HTTP 状态码
//   - 使用 fiber.Error.Message，若为空则回退到标准 HTTP 状态文本
//
// 其他错误:
//   - 记录错误日志
//   - 使用 response.Error 统一处理（支持 BizError）
func NewErrorHandler(log *logger.Logger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		if err == nil {
			return nil
		}

		// 特殊处理 Fiber 错误
		var fiberErr *fiber.Error
		if errors.As(err, &fiberErr) {
			msg := fiberErr.Message
			if msg == "" {
				msg = http.StatusText(fiberErr.Code)
			}
			if msg == "" {
				msg = http.StatusText(http.StatusInternalServerError)
			}
			return c.Status(fiberErr.Code).JSON(response.Result{
				Code: fiberErr.Code,
				Msg:  msg,
				Data: &struct{}{},
			})
		}

		// 其他错误：记录日志并使用统一响应
		if log != nil {
			log.Error("unhandled error", zap.Error(err))
		}
		return response.Error(c, err)
	}
}
