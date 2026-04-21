package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"sort"

	"github.com/aisgo/ais-pkg/logger"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

/* ========================================================================
 * API Key Authentication Middleware (Enhanced Security)
 * ========================================================================
 * 职责: 验证 API Key 请求
 * 安全增强:
 *   - API Key 存储为 SHA256 散列值而非明文
 *   - 使用 constant-time 比较防止时序攻击
 *   - 支持两种方式: X-API-Key Header 和 Authorization Bearer
 *
 * 使用示例:
 *   // 配置中使用原始 API Key
 *   cfg := &APIKeyConfig{
 *       Enabled: true,
 *       Keys: map[string]string{
 *           "client1": "sk_live_1234567890abcdef",
 *           "client2": "sk_test_abcdef1234567890",
 *       },
 *   }
 *
 *   auth := NewAPIKeyAuth(cfg, log)
 *   app.Use(auth.Authenticate())
 *
 *   // 客户端请求时使用原始 API Key
 *   // X-API-Key: sk_live_1234567890abcdef
 *   // 或 Authorization: Bearer sk_live_1234567890abcdef
 * ======================================================================== */

// APIKeyConfig API Key 配置
type APIKeyConfig struct {
	Enabled bool              `yaml:"enabled"`
	Keys    map[string]string `yaml:"keys"` // key_id -> api_key (配置中使用明文)
}

// APIKeyAuth API Key 认证中间件
type APIKeyAuth struct {
	config     *APIKeyConfig
	keyEntries []apiKeyHashEntry
	log        *logger.Logger
}

const apiKeyIDLocalKey = "key_id"

type apiKeyHashEntry struct {
	keyID string
	hash  [32]byte
}

// NewAPIKeyAuth 创建 API Key 认证中间件
// 注意: API Key 会被转换为 SHA256 散列后存储，原始值不会保留在内存中
func NewAPIKeyAuth(cfg *APIKeyConfig, log *logger.Logger) *APIKeyAuth {
	if cfg == nil {
		cfg = &APIKeyConfig{}
	}
	if log == nil {
		log = logger.NewNop()
	}

	// 将 API Key 转换为 SHA256 散列，并固定遍历顺序。
	keyIDs := make([]string, 0, len(cfg.Keys))
	for keyID := range cfg.Keys {
		keyIDs = append(keyIDs, keyID)
	}
	sort.Strings(keyIDs)

	keyEntries := make([]apiKeyHashEntry, 0, len(keyIDs))
	for _, keyID := range keyIDs {
		keyEntries = append(keyEntries, apiKeyHashEntry{
			keyID: keyID,
			hash:  sha256.Sum256([]byte(cfg.Keys[keyID])),
		})
	}

	return &APIKeyAuth{
		config:     cfg,
		keyEntries: keyEntries,
		log:        log,
	}
}

// KeyIDFromContext 从 fiber.Ctx 读取认证后的 key_id
func KeyIDFromContext(c fiber.Ctx) (string, bool) {
	v := c.Locals(apiKeyIDLocalKey)
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok && s != ""
}

// Authenticate 返回 Fiber 中间件
func (a *APIKeyAuth) Authenticate() fiber.Handler {
	return func(c fiber.Ctx) error {
		// 如果未启用认证，直接放行
		if !a.config.Enabled {
			return c.Next()
		}

		// 从 X-API-Key Header 获取
		apiKey := c.Get("X-API-Key")
		if apiKey == "" {
			// 尝试从 Authorization Bearer 获取
			auth := c.Get("Authorization")
			if len(auth) > 7 && auth[:7] == "Bearer " {
				apiKey = auth[7:]
			}
		}

		if apiKey == "" {
			a.log.Warn("Missing API Key",
				zap.String("ip", c.IP()),
				zap.String("path", c.Path()),
			)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code": 401,
				"msg":  "missing api key",
			})
		}

		// 验证 API Key (constant-time 比较防止时序攻击)
		keyID, valid := a.validateAPIKey(apiKey)
		if !valid {
			// 脱敏处理记录日志
			maskedKey := maskAPIKey(apiKey)

			a.log.Warn("Invalid API Key",
				zap.String("ip", c.IP()),
				zap.String("path", c.Path()),
				zap.String("key_preview", maskedKey),
			)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code": 401,
				"msg":  "invalid api key",
			})
		}

		// 将 key_id 存储到 context，用于后续的 tenant_id 映射
		c.Locals(apiKeyIDLocalKey, keyID)

		return c.Next()
	}
}

// validateAPIKey 验证 API Key
// 使用 SHA256 散列 + constant-time 比较防止时序攻击
func (a *APIKeyAuth) validateAPIKey(apiKey string) (string, bool) {
	providedHash := sha256.Sum256([]byte(apiKey))
	matchedKeyID := ""
	matched := 0

	// 必须完整遍历所有已配置的 key，避免首个命中后提前返回形成可观测时序差异。
	for _, entry := range a.keyEntries {
		equal := subtle.ConstantTimeCompare(providedHash[:], entry.hash[:])
		if equal == 1 && matched == 0 {
			matchedKeyID = entry.keyID
		}
		matched |= equal
	}

	return matchedKeyID, matched == 1
}

// maskAPIKey 脱敏 API Key 用于日志记录
func maskAPIKey(apiKey string) string {
	if len(apiKey) > 8 {
		return apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
	}
	return "****"
}
