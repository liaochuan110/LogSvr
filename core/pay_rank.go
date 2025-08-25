package main

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"
)

// PayInfo 存储在排行榜中的玩家支付信息
type PayInfo struct {
	RoleID   string `json:"roleid"`
	Name     string `json:"name"`
	Level    int    `json:"level"`
	GameSvr  int    `json:"gamesvr"`
	VipLevel int    `json:"viplevel"`
	Money    int    `json:"money"`
}

// PayRankCache 支付排行榜缓存
type PayRankCache struct {
	mu    sync.RWMutex
	cache map[string]*PayInfo // key: RoleID
}

// 全局支付排行榜缓存实例
var payRankCache = &PayRankCache{
	cache: make(map[string]*PayInfo),
}

// UpdatePayInfo 更新玩家的支付信息
// 如果玩家已在缓存中，则累加金额并更新其他信息
// 否则，将新玩家添加到缓存
func (c *PayRankCache) UpdatePayInfo(info *PayInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.cache[info.RoleID]; ok {
		// 玩家存在，累加金额，更新信息
		existing.Money += info.Money
		existing.Name = info.Name
		existing.Level = info.Level
		existing.GameSvr = info.GameSvr
		existing.VipLevel = info.VipLevel
	} else {
		// 玩家不存在，添加新条目
		c.cache[info.RoleID] = info
	}
}

// GetRank 获取按金额排序的支付排行榜
func (c *PayRankCache) GetRank() []*PayInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 将map中的数据复制到slice中
	rank := make([]*PayInfo, 0, len(c.cache))
	for _, info := range c.cache {
		rank = append(rank, info)
	}

	// 按金额从高到低排序
	sort.Slice(rank, func(i, j int) bool {
		return rank[i].Money > rank[j].Money
	})

	return rank
}

// ClearCache 清空支付排行榜缓存
func (c *PayRankCache) ClearCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*PayInfo)
	appLogger.Info("每日充值排行榜缓存已清空")
}

// LoadTodayPayData 从数据库加载今天的支付数据来预热缓存
func (c *PayRankCache) LoadTodayPayData(db *gorm.DB) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var reports []PayReport
	if err := db.Where("created_at >= ?", startOfDay).Order("created_at asc").Find(&reports).Error; err != nil {
		appLogger.Error(fmt.Sprintf("从数据库加载今日充值数据失败: %v", err))
		return
	}

	for _, report := range reports {
		if existing, ok := c.cache[report.RoleID]; ok {
			// 玩家存在，累加金额，更新信息
			existing.Money += report.Money
			existing.Name = report.Name
			existing.Level = report.Level
			existing.GameSvr = report.GameSvr
			existing.VipLevel = report.VipLevel
		} else {
			// 玩家不存在，添加新条目
			c.cache[report.RoleID] = &PayInfo{
				RoleID:   report.RoleID,
				Name:     report.Name,
				Level:    report.Level,
				GameSvr:  report.GameSvr,
				Money:    report.Money,
				VipLevel: report.VipLevel,
			}
		}
	}
	appLogger.Info(fmt.Sprintf("成功从数据库加载 %d 条今日充值记录，重建 %d 个玩家的缓存", len(reports), len(c.cache)))
}
