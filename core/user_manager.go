package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

// LogUser 用户数据结构
type LogUser struct {
	gorm.Model
	Username    string     `gorm:"column:username;type:varchar(50);uniqueIndex;not null" json:"username"`
	Password    string     `gorm:"column:password;type:varchar(64);not null" json:"-"` // 不返回到JSON
	DisplayName string     `gorm:"column:display_name;type:varchar(100);not null" json:"display_name"`
	IsActive    bool       `gorm:"column:is_active;type:bool;default:true" json:"is_active"`
	LastLogin   *time.Time `gorm:"column:last_login;type:datetime" json:"last_login"`
}

// TableName 指定表名
func (LogUser) TableName() string {
	return "log_users"
}

// UserManager 用户管理器
type UserManager struct {
	db    *gorm.DB
	cache map[string]*LogUser
	mu    sync.RWMutex
}

// 全局用户管理器实例
var userManager *UserManager

// InitUserManager 初始化用户管理器
func InitUserManager(database *gorm.DB) {
	userManager = &UserManager{
		db:    database,
		cache: make(map[string]*LogUser),
	}

	// 自动迁移用户表（使用新表名 log_users）
	database.AutoMigrate(&LogUser{})
	appLogger.Info("用户表结构初始化完成 (log_users)")

	// 创建默认管理员用户
	userManager.CreateDefaultAdmin()

	// 加载用户到缓存
	userManager.LoadUsersToCache()

	appLogger.Info("用户管理器初始化完成")
}

// HashPassword 对密码进行哈希处理
func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// CreateDefaultAdmin 创建默认管理员用户
func (um *UserManager) CreateDefaultAdmin() {
	um.mu.Lock()
	defer um.mu.Unlock()

	// 检查root用户是否已经存在
	var existingUser LogUser
	result := um.db.Where("username = ?", "root").First(&existingUser)
	if result.Error == nil {
		// root用户已存在
		appLogger.Info("默认管理员用户 root 已存在，跳过创建过程")
		return
	}

	// root用户不存在，创建新的默认管理员
	defaultPassword := "123456"
	hashedPassword := HashPassword(defaultPassword)
	appLogger.Info(fmt.Sprintf("正在创建默认管理员: 原始密码=%s, 哈希后密码=%s",
		defaultPassword, hashedPassword[:20]+"..."))

	defaultAdmin := &LogUser{
		Username:    "root",
		Password:    hashedPassword,
		DisplayName: "系统管理员",
		IsActive:    true,
	}

	if err := um.db.Create(defaultAdmin).Error; err != nil {
		appLogger.Error(fmt.Sprintf("创建默认管理员失败: %v", err))
	} else {
		appLogger.Info(fmt.Sprintf("默认管理员用户创建成功: root, ID=%d", defaultAdmin.ID))
		// 立即将新创建的用户添加到缓存中
		if um.cache == nil {
			um.cache = make(map[string]*LogUser)
		}
		um.cache[defaultAdmin.Username] = defaultAdmin
		appLogger.Info("新创建的root用户已添加到缓存")
	}
}

// LoadUsersToCache 加载用户到缓存
func (um *UserManager) LoadUsersToCache() {
	um.mu.Lock()
	defer um.mu.Unlock()

	var users []LogUser
	if err := um.db.Where("is_active = ?", true).Find(&users).Error; err != nil {
		appLogger.Error(fmt.Sprintf("加载用户到缓存失败: %v", err))
		return
	}

	um.cache = make(map[string]*LogUser)
	for i := range users {
		um.cache[users[i].Username] = &users[i]
		appLogger.Info(fmt.Sprintf("用户加载到缓存: 用户名=%s, ID=%d, 活跃状态=%t, 密码哈希=%s",
			users[i].Username, users[i].ID, users[i].IsActive, users[i].Password[:20]+"..."))
	}

	appLogger.Info(fmt.Sprintf("成功加载 %d 个用户到缓存", len(users)))

	// 打印缓存中的所有用户名
	appLogger.Info(fmt.Sprintf("缓存中的用户名列表: %v", func() []string {
		var usernames []string
		for username := range um.cache {
			usernames = append(usernames, username)
		}
		return usernames
	}()))
}

// ValidateUser 验证用户登录凭据
func (um *UserManager) ValidateUser(username, password string) (*LogUser, bool) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	// 调试信息：显示当前缓存状态
	appLogger.Info(fmt.Sprintf("开始验证用户: %s", username))
	appLogger.Info(fmt.Sprintf("当前缓存中的用户数量: %d", len(um.cache)))
	for cachedUsername := range um.cache {
		appLogger.Info(fmt.Sprintf("缓存中的用户: %s", cachedUsername))
	}

	user, exists := um.cache[username]
	if !exists {
		appLogger.Warning(fmt.Sprintf("用户验证失败: 用户 '%s' 不存在于缓存中", username))
		return nil, false
	}

	if !user.IsActive {
		appLogger.Warning(fmt.Sprintf("用户验证失败: 用户 '%s' 已被停用", username))
		return nil, false
	}

	hashedPassword := HashPassword(password)
	appLogger.Info(fmt.Sprintf("用户 '%s' 密码验证: 输入密码哈希=%s, 存储密码哈希=%s", username, hashedPassword[:20]+"...", user.Password[:20]+"..."))

	if user.Password != hashedPassword {
		appLogger.Warning(fmt.Sprintf("用户验证失败: 用户 '%s' 密码不匹配", username))
		return nil, false
	}

	// 更新最后登录时间（异步执行，不阻塞验证）
	go um.updateLastLogin(username)

	appLogger.Info(fmt.Sprintf("用户验证成功: 用户 '%s'", username))
	return user, true
}

// updateLastLogin 更新用户最后登录时间
func (um *UserManager) updateLastLogin(username string) {
	now := time.Now()
	if err := um.db.Model(&LogUser{}).Where("username = ?", username).Update("last_login", now).Error; err != nil {
		appLogger.Error(fmt.Sprintf("更新用户最后登录时间失败: %v", err))
	}

	// 同时更新缓存
	um.mu.Lock()
	if user, exists := um.cache[username]; exists {
		user.LastLogin = &now
	}
	um.mu.Unlock()
}

// CreateUser 创建新用户
func (um *UserManager) CreateUser(username, password, displayName string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	// 检查用户名是否已存在
	if _, exists := um.cache[username]; exists {
		return fmt.Errorf("用户名 '%s' 已存在", username)
	}

	// 创建新用户
	newUser := &LogUser{
		Username:    username,
		Password:    HashPassword(password),
		DisplayName: displayName,
		IsActive:    true,
	}

	if err := um.db.Create(newUser).Error; err != nil {
		return fmt.Errorf("创建用户失败: %v", err)
	}

	// 添加到缓存
	um.cache[username] = newUser

	appLogger.Info(fmt.Sprintf("新用户创建成功: %s (%s)", username, displayName))
	return nil
}

// GetUser 获取用户信息
func (um *UserManager) GetUser(username string) (*LogUser, bool) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	user, exists := um.cache[username]
	return user, exists
}

// ListUsers 获取所有活跃用户列表
func (um *UserManager) ListUsers() []*LogUser {
	um.mu.RLock()
	defer um.mu.RUnlock()

	users := make([]*LogUser, 0, len(um.cache))
	for _, user := range um.cache {
		if user.IsActive {
			users = append(users, user)
		}
	}

	return users
}

// UpdateUserPassword 更新用户密码
func (um *UserManager) UpdateUserPassword(username, newPassword string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	user, exists := um.cache[username]
	if !exists {
		return fmt.Errorf("用户 '%s' 不存在", username)
	}

	hashedPassword := HashPassword(newPassword)

	// 更新数据库
	if err := um.db.Model(&LogUser{}).Where("username = ?", username).Update("password", hashedPassword).Error; err != nil {
		return fmt.Errorf("更新密码失败: %v", err)
	}

	// 更新缓存
	user.Password = hashedPassword

	appLogger.Info(fmt.Sprintf("用户 %s 密码更新成功", username))
	return nil
}

// DeactivateUser 停用用户
func (um *UserManager) DeactivateUser(username string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	// 检查用户是否存在
	if _, exists := um.cache[username]; !exists {
		return fmt.Errorf("用户 '%s' 不存在", username)
	}

	// 不允许停用root用户
	if username == "root" {
		return fmt.Errorf("不能停用root管理员用户")
	}

	// 更新数据库
	if err := um.db.Model(&LogUser{}).Where("username = ?", username).Update("is_active", false).Error; err != nil {
		return fmt.Errorf("停用用户失败: %v", err)
	}

	// 从缓存中移除
	delete(um.cache, username)

	appLogger.Info(fmt.Sprintf("用户 %s 已停用", username))
	return nil
}

// GetUserCount 获取活跃用户数量
func (um *UserManager) GetUserCount() int {
	um.mu.RLock()
	defer um.mu.RUnlock()

	count := 0
	for _, user := range um.cache {
		if user.IsActive {
			count++
		}
	}
	return count
}
