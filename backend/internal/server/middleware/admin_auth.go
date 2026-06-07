// Package middleware provides HTTP middleware for authentication, authorization, and request processing.
package middleware

import (
	"crypto/subtle"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NewAdminAuthMiddleware 创建管理员认证中间件
func NewAdminAuthMiddleware(
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
) AdminAuthMiddleware {
	return AdminAuthMiddleware(adminAuth(authService, userService, settingService))
}

// adminAuth 管理员认证中间件实现
// 支持两种认证方式（通过不同的 header 区分）：
// 1. Admin API Key: x-api-key: <admin-api-key>
// 2. JWT Token: Authorization: Bearer <jwt-token> (需要管理员角色)
func adminAuth(
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// WebSocket upgrade requests cannot set Authorization headers in browsers.
		// For admin WebSocket endpoints (e.g. Ops realtime), allow passing the JWT via
		// Sec-WebSocket-Protocol (subprotocol list) using a prefixed token item:
		//   Sec-WebSocket-Protocol: sub2api-admin, jwt.<token>
		if isWebSocketUpgradeRequest(c) {
			if token := extractJWTFromWebSocketSubprotocol(c); token != "" {
				if !validateJWTForAdmin(c, token, authService, userService) {
					return
				}
				c.Next()
				return
			}
		}

		// 检查 x-api-key header（Admin API Key 认证）
		apiKey := c.GetHeader("x-api-key")
		if apiKey != "" {
			if !validateAdminAPIKey(c, apiKey, settingService, userService) {
				return
			}
			c.Next()
			return
		}

		// 检查 Authorization header（JWT 认证）
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				token := strings.TrimSpace(parts[1])
				if token == "" {
					AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
					return
				}
				if !validateJWTForAdmin(c, token, authService, userService) {
					return
				}
				c.Next()
				return
			}
		}

		// 无有效认证信息
		AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
	}
}

func isWebSocketUpgradeRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	// RFC6455 handshake uses:
	//   Connection: Upgrade
	//   Upgrade: websocket
	upgrade := strings.ToLower(strings.TrimSpace(c.GetHeader("Upgrade")))
	if upgrade != "websocket" {
		return false
	}
	connection := strings.ToLower(c.GetHeader("Connection"))
	return strings.Contains(connection, "upgrade")
}

func extractJWTFromWebSocketSubprotocol(c *gin.Context) string {
	if c == nil {
		return ""
	}
	raw := strings.TrimSpace(c.GetHeader("Sec-WebSocket-Protocol"))
	if raw == "" {
		return ""
	}

	// The header is a comma-separated list of tokens. We reserve the prefix "jwt."
	// for carrying the admin JWT.
	for _, part := range strings.Split(raw, ",") {
		p := strings.TrimSpace(part)
		if strings.HasPrefix(p, "jwt.") {
			token := strings.TrimSpace(strings.TrimPrefix(p, "jwt."))
			if token != "" {
				return token
			}
		}
	}
	return ""
}

// validateAdminAPIKey 验证管理员 API Key
func validateAdminAPIKey(
	c *gin.Context,
	key string,
	settingService *service.SettingService,
	userService *service.UserService,
) bool {
	storedKey, err := settingService.GetAdminAPIKey(c.Request.Context())
	if err != nil {
		AbortWithError(c, 500, "INTERNAL_ERROR", "Internal server error")
		return false
	}

	// 未配置或不匹配，统一返回相同错误（避免信息泄露）
	if storedKey == "" || subtle.ConstantTimeCompare([]byte(key), []byte(storedKey)) != 1 {
		AbortWithError(c, 401, "INVALID_ADMIN_KEY", "Invalid admin API key")
		return false
	}

	// 获取真实的管理员用户
	admin, err := userService.GetFirstAdmin(c.Request.Context())
	if err != nil {
		AbortWithError(c, 500, "INTERNAL_ERROR", "No admin user found")
		return false
	}

	c.Set(string(ContextKeyUser), AuthSubject{
		UserID:      admin.ID,
		Concurrency: admin.Concurrency,
	})
	c.Set(string(ContextKeyUserRole), admin.Role)
	c.Set("auth_method", "admin_api_key")
	return true
}

// validateJWTForAdmin 验证 JWT 并检查后台访问权限
func validateJWTForAdmin(
	c *gin.Context,
	token string,
	authService *service.AuthService,
	userService *service.UserService,
) bool {
	// 验证 JWT token
	claims, err := authService.ValidateToken(token)
	if err != nil {
		if errors.Is(err, service.ErrTokenExpired) {
			AbortWithError(c, 401, "TOKEN_EXPIRED", "Token has expired")
			return false
		}
		AbortWithError(c, 401, "INVALID_TOKEN", "Invalid token")
		return false
	}

	// 从数据库获取用户
	user, err := userService.GetByID(c.Request.Context(), claims.UserID)
	if err != nil {
		AbortWithError(c, 401, "USER_NOT_FOUND", "User not found")
		return false
	}

	// 检查用户状态
	if !user.IsActive() {
		AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
		return false
	}

	// 校验 TokenVersion，确保管理员改密后旧 token 失效
	if claims.TokenVersion != user.TokenVersion {
		AbortWithError(c, 401, "TOKEN_REVOKED", "Token has been revoked (password changed)")
		return false
	}

	// 检查后台访问权限
	if !user.CanAccessAdmin() {
		AbortWithError(c, 403, "FORBIDDEN", "Admin access required")
		return false
	}
	if !adminRequestAllowedForUser(c.Request.URL.Path, user) {
		AbortWithError(c, 403, "FORBIDDEN", "Admin page access denied")
		return false
	}

	c.Set(string(ContextKeyUser), AuthSubject{
		UserID:      user.ID,
		Concurrency: user.Concurrency,
	})
	c.Set(string(ContextKeyUserRole), user.Role)
	c.Set("auth_method", "jwt")

	return true
}

func adminRequestAllowedForUser(path string, user *service.User) bool {
	if user == nil {
		return false
	}
	if user.IsAdmin() {
		return true
	}
	if !user.IsOperator() {
		return false
	}
	if strings.HasPrefix(path, "/api/v1/admin/admin-accounts") {
		return false
	}
	for _, page := range user.OperatorPages {
		if adminPageAllowsPath(page, path) {
			return true
		}
	}
	return false
}

func adminPageAllowsPath(page string, path string) bool {
	page = strings.TrimSpace(page)
	if page == "" {
		return false
	}
	for _, prefix := range adminPageAPIPrefixes(page) {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func adminPageAPIPrefixes(page string) []string {
	const base = "/api/v1/admin"
	switch page {
	case "dashboard":
		return []string{base + "/dashboard"}
	case "ops":
		return []string{base + "/ops"}
	case "users":
		return []string{base + "/users", base + "/user-attributes", base + "/groups"}
	case "groups":
		return []string{base + "/groups", base + "/users", base + "/user-attributes"}
	case "channels_pricing":
		return []string{base + "/channels"}
	case "channels_monitor":
		return []string{base + "/channel-monitors", base + "/channel-monitor-templates", base + "/accounts", base + "/groups"}
	case "subscriptions":
		return []string{base + "/subscriptions", base + "/groups", base + "/users"}
	case "accounts":
		return []string{base + "/accounts", base + "/openai", base + "/gemini", base + "/antigravity", base + "/groups", base + "/proxies", base + "/scheduled-test-plans"}
	case "announcements":
		return []string{base + "/announcements", base + "/groups", base + "/user-attributes"}
	case "proxies":
		return []string{base + "/proxies"}
	case "risk_control":
		return []string{base + "/risk-control"}
	case "redeem":
		return []string{base + "/redeem-codes", base + "/groups", base + "/users"}
	case "promo_codes":
		return []string{base + "/promo-codes"}
	case "affiliates":
		return []string{base + "/affiliates"}
	case "orders":
		return []string{base + "/payment"}
	case "usage":
		return []string{base + "/usage"}
	case "settings":
		return []string{base + "/settings", base + "/data-management", base + "/backups", base + "/system", base + "/api-keys", base + "/error-passthrough-rules", base + "/tls-fingerprint-profiles"}
	default:
		return nil
	}
}
