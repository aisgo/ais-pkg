package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
)

func TestNewCORS_Disabled(t *testing.T) {
	cfg := CORSConfig{
		Enabled: false,
	}

	app := fiber.New()
	app.Use(NewCORS(cfg))

	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("test")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// 禁用时不应设置 CORS 头
	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestNewCORS_DefaultConfig(t *testing.T) {
	cfg := CORSConfig{
		Enabled: true,
	}

	app := fiber.New()
	app.Use(NewCORS(cfg))

	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("test")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// 默认配置应允许所有源
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestNewCORS_SpecificOrigin(t *testing.T) {
	cfg := CORSConfig{
		Enabled:      true,
		AllowOrigins: []string{"https://example.com"},
	}

	app := fiber.New()
	app.Use(NewCORS(cfg))

	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("test")
	})

	// 测试允许的源
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, "https://example.com", resp.Header.Get("Access-Control-Allow-Origin"))

	// 测试不允许的源
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("Origin", "https://evil.com")

	resp2, err := app.Test(req2)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp2.StatusCode)
	assert.Empty(t, resp2.Header.Get("Access-Control-Allow-Origin"))
}

func TestNewCORS_PreflightRequest(t *testing.T) {
	cfg := CORSConfig{
		Enabled:          true,
		AllowOrigins:     []string{"https://example.com"},
		AllowMethods:     []string{"GET", "POST", "PUT"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	app := fiber.New()
	app.Use(NewCORS(cfg))

	app.Options("/test", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")

	resp, err := app.Test(req)
	assert.NoError(t, err)

	// 预检请求应返回 204
	assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)

	// 验证 CORS 头
	assert.Equal(t, "https://example.com", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "Content-Type")
	assert.Equal(t, "true", resp.Header.Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "3600", resp.Header.Get("Access-Control-Max-Age"))
}

func TestNewCORS_ExposeHeaders(t *testing.T) {
	cfg := CORSConfig{
		Enabled:       true,
		AllowOrigins:  []string{"https://example.com"},
		ExposeHeaders: []string{"X-Custom-Header", "X-Total-Count"},
	}

	app := fiber.New()
	app.Use(NewCORS(cfg))

	app.Get("/test", func(c fiber.Ctx) error {
		c.Set("X-Custom-Header", "custom-value")
		return c.SendString("test")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	assert.Contains(t, resp.Header.Get("Access-Control-Expose-Headers"), "X-Custom-Header")
	assert.Contains(t, resp.Header.Get("Access-Control-Expose-Headers"), "X-Total-Count")
}

func TestParseAllowOrigins(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single origin",
			input:    "https://example.com",
			expected: []string{"https://example.com"},
		},
		{
			name:     "multiple origins",
			input:    "https://example.com,https://www.example.com",
			expected: []string{"https://example.com", "https://www.example.com"},
		},
		{
			name:     "with spaces",
			input:    "https://example.com , https://www.example.com ",
			expected: []string{"https://example.com", "https://www.example.com"},
		},
		{
			name:     "wildcard",
			input:    "*",
			expected: []string{"*"},
		},
		{
			name:     "subdomain wildcard",
			input:    "https://*.example.com",
			expected: []string{"https://*.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseAllowOrigins(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseAllowMethods(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single method",
			input:    "GET",
			expected: []string{"GET"},
		},
		{
			name:     "multiple methods",
			input:    "GET,POST,PUT,DELETE",
			expected: []string{"GET", "POST", "PUT", "DELETE"},
		},
		{
			name:     "lowercase to uppercase",
			input:    "get,post,put",
			expected: []string{"GET", "POST", "PUT"},
		},
		{
			name:     "with spaces",
			input:    "GET , POST , PUT",
			expected: []string{"GET", "POST", "PUT"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseAllowMethods(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseAllowHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single header",
			input:    "Content-Type",
			expected: []string{"Content-Type"},
		},
		{
			name:     "multiple headers",
			input:    "Origin,Content-Type,Accept",
			expected: []string{"Origin", "Content-Type", "Accept"},
		},
		{
			name:     "with spaces",
			input:    "Content-Type , Authorization ",
			expected: []string{"Content-Type", "Authorization"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseAllowHeaders(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseExposeHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single header",
			input:    "X-Custom-Header",
			expected: []string{"X-Custom-Header"},
		},
		{
			name:     "multiple headers",
			input:    "X-Custom-Header,X-Total-Count,X-Page-Size",
			expected: []string{"X-Custom-Header", "X-Total-Count", "X-Page-Size"},
		},
		{
			name:     "with spaces",
			input:    "X-Custom-Header , X-Total-Count ",
			expected: []string{"X-Custom-Header", "X-Total-Count"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseExposeHeaders(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
