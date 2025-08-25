package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Session 会话结构体
type Session struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SessionManager 会话管理器
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// 全局会话管理器实例
var sessionManager = &SessionManager{
	sessions: make(map[string]*Session),
}

// 默认登录凭据
const (
	DefaultUsername = "root"
	DefaultPassword = "123456"
	SessionDuration = 24 * time.Hour // 会话有效期24小时
	CookieName      = "gamelogin_session"
)

// generateSessionID 生成随机会话ID
func generateSessionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// CreateSession 创建新会话
func (sm *SessionManager) CreateSession(username string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 清理过期会话
	sm.cleanExpiredSessions()

	sessionID := generateSessionID()
	session := &Session{
		ID:        sessionID,
		Username:  username,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(SessionDuration),
	}

	sm.sessions[sessionID] = session
	appLogger.Info("创建新会话: 用户=" + username + ", SessionID=" + sessionID)
	return session
}

// GetSession 获取会话
func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil, false
	}

	// 检查会话是否过期
	if time.Now().After(session.ExpiresAt) {
		// 异步删除过期会话
		go func() {
			sm.mu.Lock()
			delete(sm.sessions, sessionID)
			sm.mu.Unlock()
		}()
		return nil, false
	}

	return session, true
}

// DeleteSession 删除会话
func (sm *SessionManager) DeleteSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, exists := sm.sessions[sessionID]; exists {
		appLogger.Info("删除会话: 用户=" + session.Username + ", SessionID=" + sessionID)
		delete(sm.sessions, sessionID)
	}
}

// cleanExpiredSessions 清理过期会话（内部方法，调用前需要加锁）
func (sm *SessionManager) cleanExpiredSessions() {
	now := time.Now()
	for sessionID, session := range sm.sessions {
		if now.After(session.ExpiresAt) {
			delete(sm.sessions, sessionID)
		}
	}
}

// GetSessionCount 获取当前活跃会话数量
func (sm *SessionManager) GetSessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// 清理过期会话并返回数量
	now := time.Now()
	count := 0
	for _, session := range sm.sessions {
		if now.Before(session.ExpiresAt) {
			count++
		}
	}
	return count
}

// ValidateCredentials 验证登录凭据
func ValidateCredentials(username, password string) (*LogUser, bool) {
	// 使用用户管理器验证凭据
	if userManager == nil {
		// 如果用户管理器未初始化，使用原有的默认验证
		if username == DefaultUsername && password == DefaultPassword {
			return &LogUser{
				Username:    username,
				DisplayName: "系统管理员",
			}, true
		}
		return nil, false
	}

	return userManager.ValidateUser(username, password)
}

// LoginHandler 登录处理函数
func LoginHandler(c *gin.Context) {
	var loginRequest struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&loginRequest); err != nil {
		appLogger.Error("登录请求参数错误: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "请求参数错误",
		})
		return
	}

	// 验证凭据
	user, isValid := ValidateCredentials(loginRequest.Username, loginRequest.Password)
	if !isValid {
		appLogger.Warning("登录失败: 用户名=" + loginRequest.Username + " (凭据无效)")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": "用户名或密码错误",
		})
		return
	}

	// 创建会话
	session := sessionManager.CreateSession(user.Username)

	// 设置Session Cookie
	c.SetCookie(
		CookieName,                     // 名称
		session.ID,                     // 值
		int(SessionDuration.Seconds()), // 最大年龄（秒）
		"/",                            // 路径
		"",                             // 域名
		false,                          // 仅HTTPS
		true,                           // HTTP Only
	)

	appLogger.Info("用户登录成功: " + user.Username + " (" + user.DisplayName + ")")
	c.JSON(http.StatusOK, gin.H{
		"status":       "success",
		"message":      "登录成功",
		"user":         user.Username,
		"display_name": user.DisplayName,
	})
}

// LogoutHandler 退出登录处理函数
func LogoutHandler(c *gin.Context) {
	sessionID, err := c.Cookie(CookieName)
	if err == nil && sessionID != "" {
		sessionManager.DeleteSession(sessionID)
	}

	// 清除Cookie
	c.SetCookie(
		CookieName,
		"",
		-1,
		"/",
		"",
		false,
		true,
	)

	appLogger.Info("用户退出登录")
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "退出登录成功",
	})
}

// AuthMiddleware 认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取Session Cookie
		sessionID, err := c.Cookie(CookieName)
		if err != nil || sessionID == "" {
			// 未找到会话Cookie，重定向到登录页面
			if isAPIRequest(c) {
				c.JSON(http.StatusUnauthorized, gin.H{
					"status":  "error",
					"message": "未登录或登录已过期",
				})
			} else {
				c.Redirect(http.StatusFound, "/login")
			}
			c.Abort()
			return
		}

		// 验证会话
		session, exists := sessionManager.GetSession(sessionID)
		if !exists {
			// 会话无效或过期
			if isAPIRequest(c) {
				c.JSON(http.StatusUnauthorized, gin.H{
					"status":  "error",
					"message": "登录已过期，请重新登录",
				})
			} else {
				c.Redirect(http.StatusFound, "/login")
			}
			c.Abort()
			return
		}

		// 将用户信息存储到上下文中
		c.Set("user", session.Username)
		c.Set("session", session)
		c.Next()
	}
}

// isAPIRequest 判断是否为API请求
func isAPIRequest(c *gin.Context) bool {
	// 根据请求路径或Accept头判断是否为API请求
	path := c.Request.URL.Path
	accept := c.GetHeader("Accept")

	// API路径通常包含这些前缀，或者Accept头包含application/json
	apiPrefixes := []string{"/api/", "/getactivateplayer", "/getnewplayer", "/get_today_payment_stats", "/pay_rank", "/today_online"}

	for _, prefix := range apiPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}

	// 安全地检查Accept头
	if len(accept) > 0 {
		if accept == "application/json" {
			return true
		}
		// 检查是否以"application/json"开头，安全地切片
		if len(accept) >= 16 && accept[:16] == "application/json" {
			return true
		}
	}

	return false
}
