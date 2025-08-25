package main

import (
	"sync"
	"time"

	"gorm.io/gorm"
)

// DateToInt 将日期字符串转换为整型 (YYYYMMDD)
func DateToInt(dateStr string) int {
	if dateStr == "" {
		return int(time.Now().Unix() / 86400 * 86400)
	}

	if t, err := time.Parse("2006-01-02", dateStr); err == nil {
		return t.Year()*10000 + int(t.Month())*100 + t.Day()
	}

	// 如果解析失败，返回今天的日期
	now := time.Now()
	return now.Year()*10000 + int(now.Month())*100 + now.Day()
}

// GetCurrentDateInt 获取当前日期的整型表示
func GetCurrentDateInt() int {
	now := time.Now()
	return now.Year()*10000 + int(now.Month())*100 + now.Day()
}

// OnlineNum 在线人数数据结构
type OnlineNum struct {
	gorm.Model
	GameSvrID int `gorm:"column:gamesvr_id;type:int;not null"`
	OnlineNum int `gorm:"column:online_num;type:int;not null"`
	DateInt   int `gorm:"column:date_int;type:int;not null;index:idx_date_gamesvr"`
}

// Player 玩家数据结构
type Player struct {
	gorm.Model
	RoleID    string `gorm:"column:roleid;type:varchar(50);not null"`
	Name      string `gorm:"column:name;type:varchar(100);not null"`
	Level     int    `gorm:"column:level;type:int;not null"`
	GameSvr   int    `gorm:"column:gamesvr;type:int;not null"`
	NewPlayer bool   `gorm:"column:new_player;type:bool;default:true"`
	DateInt   int    `gorm:"column:date_int;type:int;not null;index:idx_date_gamesvr"`
}

// PayReport 支付上报数据结构
type PayReport struct {
	gorm.Model
	RoleID   string `gorm:"column:roleid;type:varchar(50);not null"`
	Name     string `gorm:"column:name;type:varchar(100);not null"`
	Level    int    `gorm:"column:level;type:int;not null"`
	GameSvr  int    `gorm:"column:gamesvr;type:int;not null"`
	Money    int    `gorm:"column:money;type:int;not null"`
	VipLevel int    `gorm:"column:vip_level;type:int;not null;default:0"`
	DateInt  int    `gorm:"column:date_int;type:int;not null;index:idx_date_gamesvr"`
}

// OnlineNumCache 在线人数内存缓存
type OnlineNumCache struct {
	mu    sync.RWMutex
	cache map[int]int // key: gamesvrID, value: onlineNum
}

// 全局在线人数缓存实例
var onlineNumCache = &OnlineNumCache{
	cache: make(map[int]int),
}

// PlayerCache 玩家数据内存缓存
type PlayerCache struct {
	mu    sync.RWMutex
	cache map[string]*Player // key: RoleID, value: Player数据
}

// 全局玩家缓存实例
var playerCache = &PlayerCache{
	cache: make(map[string]*Player),
}

// SetOnlineNum 设置在线人数到内存缓存
func (c *OnlineNumCache) SetOnlineNum(gameSvrID, onlineNum int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[gameSvrID] = onlineNum
}

// GetOnlineNum 从内存缓存获取在线人数
func (c *OnlineNumCache) GetOnlineNum(gameSvrID int) (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	onlineNum, exists := c.cache[gameSvrID]
	return onlineNum, exists
}

// GetAllOnlineNums 获取所有在线人数数据
func (c *OnlineNumCache) GetAllOnlineNums() map[int]int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 返回副本，避免外部修改
	result := make(map[int]int)
	for k, v := range c.cache {
		result[k] = v
	}
	return result
}

// SetPlayer 设置玩家数据到内存缓存
func (c *PlayerCache) SetPlayer(player *Player) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[player.RoleID] = player
}

// GetPlayer 从内存缓存获取玩家数据
func (c *PlayerCache) GetPlayer(roleID string) (*Player, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	player, exists := c.cache[roleID]
	return player, exists
}

// GetAllPlayers 获取所有玩家数据
func (c *PlayerCache) GetAllPlayers() map[string]*Player {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 返回副本，避免外部修改
	result := make(map[string]*Player)
	for k, v := range c.cache {
		result[k] = v
	}
	return result
}

// ClearCache 手动清空缓存
func (c *PlayerCache) ClearCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*Player)
}

// GetCacheSize 获取缓存大小
func (c *PlayerCache) GetCacheSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// 设置表名
func (OnlineNum) TableName() string {
	return "online_num"
}

func (Player) TableName() string {
	return "player"
}

func (PayReport) TableName() string {
	return "pay_report"
}
