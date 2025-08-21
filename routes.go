package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(r *gin.Engine, db *gorm.DB) {
	// 记录路由注册开始
	appLogger.Info("开始注册HTTP路由接口")

	// 提供静态页面
	r.GET("/", func(c *gin.Context) {
		c.File("templates/index.html")
	})
	appLogger.Info("静态页面路由注册成功: GET /")

	// 在线人数上报接口
	r.POST("/onlineNum", func(c *gin.Context) {
		var data struct {
			GameSvrID int `json:"gamesvrID" form:"gamesvrID" binding:"required"`
			OnlineNum int `json:"onlineNum" form:"onlineNum" binding:"gte=0"`
		}
		if err := c.ShouldBind(&data); err != nil {
			appLogger.Error(fmt.Sprintf("在线人数上报参数错误: %v", err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// 先将数据存入内存缓存
		onlineNumCache.SetOnlineNum(data.GameSvrID, data.OnlineNum)

		// 同时存入数据库
		if err := db.Create(&OnlineNum{
			GameSvrID: data.GameSvrID,
			OnlineNum: data.OnlineNum,
		}).Error; err != nil {
			appLogger.Error(fmt.Sprintf("在线人数数据写入数据库失败: %v", err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		appLogger.Info(fmt.Sprintf("在线人数上报成功 - 服务器ID: %d, 在线人数: %d", data.GameSvrID, data.OnlineNum))
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})
	appLogger.Info("在线人数上报接口注册成功: POST /onlineNum")

	// 玩家登录接口
	r.POST("/user_login", func(c *gin.Context) {
		var data struct {
			RoleID    string `json:"roleid" form:"roleid" binding:"required"`
			Name      string `json:"name" form:"name" binding:"required"`
			Level     int    `json:"level" form:"level" binding:"required"`
			GameSvr   int    `json:"gamesvr" form:"gamesvr" binding:"required"`
			NewPlayer bool   `json:"new_player" form:"new_player"`
		}
		if err := c.ShouldBind(&data); err != nil {
			appLogger.Error(fmt.Sprintf("玩家登录参数错误: %v", err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// 检查RoleID是否在缓存中
		if existingPlayer, exists := playerCache.GetPlayer(data.RoleID); exists {
			// 如果RoleID在缓存中，只更新缓存数据（new_player字段不覆盖）
			existingPlayer.Name = data.Name
			existingPlayer.Level = data.Level
			existingPlayer.GameSvr = data.GameSvr
			// 注意：new_player字段保持不变，不被覆盖
			playerCache.SetPlayer(existingPlayer)

			//appLogger.Info(fmt.Sprintf("玩家数据更新到缓存 - RoleID: %s, 名称: %s, 等级: %d", data.RoleID, data.Name, data.Level))
			c.JSON(http.StatusOK, gin.H{
				"status":  "success",
				"message": "玩家数据已更新到缓存",
				"action":  "cache_update",
			})
		} else {
			// 如果RoleID不在缓存中，写入数据库并缓存数据
			player := &Player{
				RoleID:    data.RoleID,
				Name:      data.Name,
				Level:     data.Level,
				GameSvr:   data.GameSvr,
				NewPlayer: data.NewPlayer,
			}

			if err := db.Create(player).Error; err != nil {
				appLogger.Error(fmt.Sprintf("新玩家数据写入数据库失败: %v", err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			// 缓存新创建的玩家数据
			playerCache.SetPlayer(player)

			//appLogger.Info(fmt.Sprintf("新玩家数据已写入数据库并缓存 - RoleID: %s, 名称: %s, 等级: %d, 新玩家: %t", data.RoleID, data.Name, data.Level, data.NewPlayer))
			c.JSON(http.StatusOK, gin.H{
				"status":  "success",
				"message": "新玩家数据已写入数据库并缓存",
				"action":  "db_insert_and_cache",
			})
		}
	})
	appLogger.Info("玩家登录接口注册成功: POST /user_login")

	// 支付上报接口
	r.POST("/pay_report", func(c *gin.Context) {
		var data struct {
			RoleID   string `json:"roleid" form:"roleid" binding:"required"`
			GameSvr  int    `json:"gamesvr" form:"gamesvr" binding:"required"`
			Money    int    `json:"money" form:"money" binding:"required"`
			VipLevel int    `json:"viplevel" form:"viplevel" binding:"gte=0"`
		}
		if err := c.ShouldBind(&data); err != nil {
			appLogger.Error(fmt.Sprintf("支付上报参数错误: %v", err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// 创建支付记录
		payReport := &PayReport{
			RoleID:   data.RoleID,
			GameSvr:  data.GameSvr,
			Money:    data.Money,
			VipLevel: data.VipLevel,
		}

		// 保存到数据库
		if err := db.Create(payReport).Error; err != nil {
			appLogger.Error(fmt.Sprintf("支付数据写入数据库失败: %v", err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		appLogger.Info(fmt.Sprintf("支付上报成功 - RoleID: %s, 服务器: %d, 金额: %d, VIP等级: %d", data.RoleID, data.GameSvr, data.Money, data.VipLevel))
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})
	appLogger.Info("支付上报接口注册成功: POST /pay_report")

	// 获取今天每分钟在线人数
	r.GET("/today_online", func(c *gin.Context) {
		// 统一使用UTC时间，避免时区问题
		now := time.Now().UTC()
		startOfDay := now.Truncate(24 * time.Hour)

		// 1. 查询数据库，获取每分钟的总在线人数
		var perMinuteResults []struct {
			Minute    string // Format: "YYYY-MM-DD HH:mm:ss"
			OnlineNum int
		}
		db.Raw(`
			SELECT
				minute,
				SUM(online_num) as online_num
			FROM (
				SELECT
					DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:00') as minute,
					gamesvr_id,
					MAX(online_num) as online_num
				FROM online_num
				WHERE created_at >= ? AND created_at <= ?
				GROUP BY minute, gamesvr_id
			) as t
			GROUP BY minute
		`, startOfDay, now).Scan(&perMinuteResults)

		// 2. 在Go中聚合数据：计算每3分钟内的峰值
		threeMinuteMap := make(map[string]int) // Key: "15:04", Value: max online num

		for _, row := range perMinuteResults {
			// 将数据库返回的无时区时间字符串解析为UTC时间
			t, err := time.ParseInLocation("2006-01-02 15:04:05", row.Minute, time.UTC)
			if err != nil {
				continue // Skip if format is wrong
			}

			// 向下取整到最近的3分钟时间点 (在UTC下计算)
			minute := t.Minute()
			remainder := minute % 3
			threeMinIntervalTime := t.Add(time.Duration(-remainder) * time.Minute)
			label := threeMinIntervalTime.Format("15:04")

			// 如果当前分钟的人数 > map中记录的这个3分钟区间的最大人数，则更新
			if row.OnlineNum > threeMinuteMap[label] {
				threeMinuteMap[label] = row.OnlineNum
			}
		}

		// 3. 组装最终返回给前端的数据
		var finalResults []struct {
			Minute    time.Time `json:"Minute"`
			OnlineNum int       `json:"OnlineNum"`
		}
		for i := 0; i < 480; i++ { // 480 = 24 * 60 / 3
			// 生成从UTC 0点开始的每个3分钟时间点
			t := startOfDay.Add(time.Duration(i*3) * time.Minute)
			label := t.Format("15:04")

			// 从map中获取这个3分钟区间的峰值
			onlineNum := threeMinuteMap[label] // 如果map中没有，默认为0

			finalResults = append(finalResults, struct {
				Minute    time.Time `json:"Minute"`
				OnlineNum int       `json:"OnlineNum"`
			}{Minute: t, OnlineNum: onlineNum})
		}

		c.JSON(http.StatusOK, gin.H{"data": finalResults})
	})
	appLogger.Info("获取今天在线人数统计接口注册成功: GET /today_online")

	// 获取今天活跃玩家人数
	r.GET("/getactivateplayer", func(c *gin.Context) {
		var count int64
		startOfDay := time.Now().Truncate(24 * time.Hour)
		db.Model(&Player{}).Where("created_at >= ?", startOfDay).Distinct("roleid").Count(&count)
		c.JSON(http.StatusOK, gin.H{"active_player_count": count})
	})
	appLogger.Info("获取今天活跃玩家人数接口注册成功: GET /getactivateplayer")

	// 获取今天新增玩家人数
	r.GET("/getnewplayer", func(c *gin.Context) {
		var count int64
		startOfDay := time.Now().Truncate(24 * time.Hour)
		db.Model(&Player{}).Where("created_at >= ? AND new_player = ?", startOfDay, true).Count(&count)
		c.JSON(http.StatusOK, gin.H{"new_player_count": count})
	})
	appLogger.Info("获取今天新增玩家人数接口注册成功: GET /getnewplayer")

	// 获取今天支付统计
	r.GET("/get_today_payment_stats", func(c *gin.Context) {
		startOfDay := time.Now().Truncate(24 * time.Hour)

		var payingPlayerCount int64
		db.Model(&PayReport{}).Where("created_at >= ?", startOfDay).Distinct("roleid").Count(&payingPlayerCount)

		var totalPayment int64
		db.Model(&PayReport{}).Where("created_at >= ?", startOfDay).Select("coalesce(sum(money), 0)").Row().Scan(&totalPayment)

		c.JSON(http.StatusOK, gin.H{
			"paying_player_count": payingPlayerCount,
			"total_payment":       totalPayment,
		})
	})
	appLogger.Info("获取今天支付统计接口注册成功: GET /get_today_payment_stats")

	// 手动清理玩家缓存
	r.POST("/cache/clear_players", func(c *gin.Context) {
		beforeSize := playerCache.GetCacheSize()
		playerCache.ClearCache()
		afterSize := playerCache.GetCacheSize()

		appLogger.Info(fmt.Sprintf("手动清理玩家缓存完成 - 清理前: %d, 清理后: %d", beforeSize, afterSize))

		c.JSON(http.StatusOK, gin.H{
			"status":      "success",
			"message":     "玩家缓存已清空",
			"before_size": beforeSize,
			"after_size":  afterSize,
			"cleared_at":  time.Now().Format("2006-01-02 15:04:05"),
		})
	})
	appLogger.Info("手动清理玩家缓存接口注册成功: POST /cache/clear_players")

	// 记录路由注册完成
	appLogger.Info("所有HTTP路由接口注册完成")
}
