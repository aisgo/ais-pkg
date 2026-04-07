package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

/* ========================================================================
 * CORS Middleware - 跨域资源共享中间件
 * ========================================================================
 * 职责: 处理跨域请求，添加 CORS 响应头
 * 技术: Fiber v3 CORS
 *
 * 安全注意事项:
 *   - 当 AllowCredentials 为 true 时，AllowOrigins 不能使用通配符 "*"
 *   - 避免使用过于宽松的源配置（如 "https://*.example.com"）
 *   - AllowOriginsFunc 必须有严格的验证逻辑
 *
 * 使用示例:
 *   // 默认配置（允许所有源）
 *   app.Use(middleware.NewCORS(middleware.CORSConfig{}))
 *
 *   // 生产环境配置
 *   app.Use(middleware.NewCORS(middleware.CORSConfig{
 *       AllowOrigins:     []string{"https://example.com", "https://www.example.com"},
 *       AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
 *       AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
 *       AllowCredentials: true,
 *       MaxAge:           3600,
 *   }))
 * ======================================================================== */

// CORSConfig CORS 配置
type CORSConfig struct {
	// Enabled 是否启用 CORS 中间件
	Enabled bool `yaml:"enabled"`

	// AllowOrigins 允许的源列表
	// 支持单源、多源、子域名匹配（如 https://*.example.com）
	// 使用通配符 "*" 表示允许所有源（仅用于开发环境）
	AllowOrigins []string `yaml:"allow_origins"`

	// AllowMethods 允许的 HTTP 方法列表
	// 默认: GET, POST, HEAD, PUT, DELETE, PATCH
	AllowMethods []string `yaml:"allow_methods"`

	// AllowHeaders 允许的请求头列表
	// 用于响应预检请求，告知客户端可以使用哪些请求头
	AllowHeaders []string `yaml:"allow_headers"`

	// AllowCredentials 是否允许携带凭证（Cookie、Authorization 等）
	// 注意: 当设置为 true 时，AllowOrigins 不能使用通配符 "*"
	AllowCredentials bool `yaml:"allow_credentials"`

	// ExposeHeaders 允许客户端访问的响应头白名单
	// 默认为空，表示不允许访问任何自定义响应头
	ExposeHeaders []string `yaml:"expose_headers"`

	// MaxAge 预检请求的缓存时间（秒）
	// 0: 不设置 Access-Control-Max-Age（浏览器默认 5 秒）
	// 负数: 禁用缓存，设置为 0
	// 正数: 设置缓存秒数
	MaxAge int `yaml:"max_age"`

	// AllowPrivateNetwork 是否允许来自私有网络的请求
	// 设置 Access-Control-Allow-Private-Network: true
	AllowPrivateNetwork bool `yaml:"allow_private_network"`
}

// NewCORS 创建 CORS 中间件
// 如果未启用（Enabled=false），返回跳过的中间件
func NewCORS(cfg CORSConfig) fiber.Handler {
	// 未启用，返回跳过的中间件
	if !cfg.Enabled {
		return func(c fiber.Ctx) error {
			return c.Next()
		}
	}

	// 构建 Fiber CORS 配置
	corsCfg := cors.Config{
		AllowCredentials:    cfg.AllowCredentials,
		ExposeHeaders:       cfg.ExposeHeaders,
		MaxAge:              cfg.MaxAge,
		AllowPrivateNetwork: cfg.AllowPrivateNetwork,
	}

	// 设置 AllowOrigins
	if len(cfg.AllowOrigins) > 0 {
		corsCfg.AllowOrigins = cfg.AllowOrigins
	}
	// 默认为 []string{"*"}

	// 设置 AllowMethods
	if len(cfg.AllowMethods) > 0 {
		corsCfg.AllowMethods = cfg.AllowMethods
	}

	// 设置 AllowHeaders
	if len(cfg.AllowHeaders) > 0 {
		corsCfg.AllowHeaders = cfg.AllowHeaders
	}

	return cors.New(corsCfg)
}

// ParseAllowOrigins 从逗号分隔的字符串解析 AllowOrigins
// 支持格式: "https://example.com,https://www.example.com"
func ParseAllowOrigins(origins string) []string {
	if origins == "" {
		return nil
	}
	parts := strings.Split(origins, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ParseAllowMethods 从逗号分隔的字符串解析 AllowMethods
// 支持格式: "GET,POST,PUT,DELETE"
func ParseAllowMethods(methods string) []string {
	if methods == "" {
		return nil
	}
	parts := strings.Split(methods, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.ToUpper(strings.TrimSpace(part))
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ParseAllowHeaders 从逗号分隔的字符串解析 AllowHeaders
// 支持格式: "Origin,Content-Type,Accept,Authorization"
func ParseAllowHeaders(headers string) []string {
	if headers == "" {
		return nil
	}
	parts := strings.Split(headers, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			// HTTP 头名称不区分大小写，但通常使用 Title Case
			result = append(result, trimmed)
		}
	}
	return result
}

// ParseExposeHeaders 从逗号分隔的字符串解析 ExposeHeaders
// 支持格式: "X-Custom-Header,X-Total-Count"
func ParseExposeHeaders(headers string) []string {
	if headers == "" {
		return nil
	}
	parts := strings.Split(headers, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
