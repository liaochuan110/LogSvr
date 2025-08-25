package main

import (
	"fmt"
	"net/http"
	"sort"
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
			Name     string `json:"name" form:"name" binding:"required"`
			Level    int    `json:"level" form:"level" binding:"required"`
			GameSvr  int    `json:"gamesvr" form:"gamesvr" binding:"required"`
			Money    int    `json:"money" form:"money" binding:"required"`
			VipLevel int    `json:"viplevel" form:"viplevel" binding:"gte=0"`
		}
		if err := c.ShouldBind(&data); err != nil {
			appLogger.Error(fmt.Sprintf("支付上报参数错误: %v", err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// 更新支付排行榜缓存
		payRankCache.UpdatePayInfo(&PayInfo{
			RoleID:   data.RoleID,
			Name:     data.Name,
			Level:    data.Level,
			GameSvr:  data.GameSvr,
			Money:    data.Money,
			VipLevel: data.VipLevel,
		})

		// 创建支付记录
		payReport := &PayReport{
			RoleID:   data.RoleID,
			Name:     data.Name,
			Level:    data.Level,
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

		appLogger.Info(fmt.Sprintf("支付上报成功 - RoleID: %s, 名称: %s, 等级: %d, 服务器: %d, 金额: %d, VIP等级: %d", data.RoleID, data.Name, data.Level, data.GameSvr, data.Money, data.VipLevel))
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})
	appLogger.Info("支付上报接口注册成功: POST /pay_report")

	// 获取充值排行榜（支持日期和区服筛选）
	r.GET("/pay_rank", func(c *gin.Context) {
		// 获取查询参数
		dateParam := c.Query("date")
		serverParam := c.Query("server")

		// 处理日期参数
		var targetDate time.Time
		if dateParam != "" {
			if parsed, err := time.Parse("2006-01-02", dateParam); err == nil {
				targetDate = parsed
			} else {
				targetDate = time.Now().Truncate(24 * time.Hour)
			}
		} else {
			targetDate = time.Now().Truncate(24 * time.Hour)
		}

		// 如果是今天且全服，直接使用缓存
		isToday := targetDate.Format("2006-01-02") == time.Now().Format("2006-01-02")
		if isToday && (serverParam == "" || serverParam == "0") {
			rank := payRankCache.GetRank()
			c.JSON(http.StatusOK, gin.H{"rank": rank})
			return
		}

		// 否则从数据库查询
		startOfDay := targetDate
		endOfDay := targetDate.Add(24 * time.Hour)

		// 构建查询
		query := db.Model(&PayReport{}).Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay)

		// 处理区服筛选
		if serverParam != "" && serverParam != "0" {
			query = query.Where("gamesvr = ?", serverParam)
		}

		// 查询数据库并聚合数据
		var reports []PayReport
		query.Order("created_at asc").Find(&reports)

		// 手动聚合数据（按RoleID累加金额）
		payInfoMap := make(map[string]*PayInfo)
		for _, report := range reports {
			if existing, ok := payInfoMap[report.RoleID]; ok {
				// 玩家存在，累加金额并更新信息
				existing.Money += report.Money
				existing.Name = report.Name
				existing.Level = report.Level
				existing.GameSvr = report.GameSvr
				existing.VipLevel = report.VipLevel
			} else {
				// 玩家不存在，添加新条目
				payInfoMap[report.RoleID] = &PayInfo{
					RoleID:   report.RoleID,
					Name:     report.Name,
					Level:    report.Level,
					GameSvr:  report.GameSvr,
					Money:    report.Money,
					VipLevel: report.VipLevel,
				}
			}
		}

		// 转换为数组并排序
		var rank []*PayInfo
		for _, info := range payInfoMap {
			rank = append(rank, info)
		}

		// 按金额从高到低排序
		sort.Slice(rank, func(i, j int) bool {
			return rank[i].Money > rank[j].Money
		})

		c.JSON(http.StatusOK, gin.H{"rank": rank})
	})
	appLogger.Info("获取充值排行榜接口注册成功: GET /pay_rank")

	// 获取在线人数曲线（支持日期和区服筛选）
	r.GET("/today_online", func(c *gin.Context) {
		// 获取查询参数
		dateParam := c.Query("date")
		serverParam := c.Query("server")

		// 处理日期参数
		var targetDate time.Time
		if dateParam != "" {
			if parsed, err := time.Parse("2006-01-02", dateParam); err == nil {
				targetDate = parsed.UTC()
			} else {
				targetDate = time.Now().UTC().Truncate(24 * time.Hour)
			}
		} else {
			targetDate = time.Now().UTC().Truncate(24 * time.Hour)
		}

		startOfDay := targetDate
		endOfDay := targetDate.Add(24 * time.Hour)

		// 1. 构建基础SQL查询
		baseSQL := `
			SELECT
				minute,
				SUM(online_num) as online_num
			FROM (
				SELECT
					DATE_FORMAT(created_at, '%%Y-%%m-%%d %%H:%%i:00') as minute,
					gamesvr_id,
					MAX(online_num) as online_num
				FROM online_num
				WHERE created_at >= ? AND created_at <= ?`

		args := []interface{}{startOfDay, endOfDay}

		// 处理区服筛选
		if serverParam != "" && serverParam != "0" {
			baseSQL += " AND gamesvr_id = ?"
			args = append(args, serverParam)
		}

		baseSQL += `
				GROUP BY minute, gamesvr_id
			) as t
			GROUP BY minute`

		// 2. 查询数据库，获取每分钟的总在线人数
		var perMinuteResults []struct {
			Minute    string // Format: "YYYY-MM-DD HH:mm:ss"
			OnlineNum int
		}
		db.Raw(baseSQL, args...).Scan(&perMinuteResults)

		// 3. 在Go中聚合数据：计算每3分钟内的峰值
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

		// 4. 组装最终返回给前端的数据
		var finalResults []struct {
			Minute    time.Time `json:"Minute"`
			OnlineNum int       `json:"OnlineNum"`
		}
		for i := 0; i < 480; i++ { // 480 = 24 * 60 / 3
			// 生成从指定日期0点开始的每个3分钟时间点
			t := startOfDay.Add(time.Duration(i*3) * time.Minute)
			label := t.Format("15:04")

			// 从 map中获取这个3分钟区间的峰值
			onlineNum := threeMinuteMap[label] // 如果map中没有，默认为0

			finalResults = append(finalResults, struct {
				Minute    time.Time `json:"Minute"`
				OnlineNum int       `json:"OnlineNum"`
			}{Minute: t, OnlineNum: onlineNum})
		}

		c.JSON(http.StatusOK, gin.H{"data": finalResults})
	})
	appLogger.Info("获取今天在线人数统计接口注册成功: GET /today_online")

	// 获取活跃玩家人数（支持日期和区服筛选）
	r.GET("/getactivateplayer", func(c *gin.Context) {
		// 获取查询参数
		dateParam := c.Query("date")
		serverParam := c.Query("server")

		// 处理日期参数
		var targetDate time.Time
		if dateParam != "" {
			if parsed, err := time.Parse("2006-01-02", dateParam); err == nil {
				targetDate = parsed
			} else {
				targetDate = time.Now().Truncate(24 * time.Hour)
			}
		} else {
			targetDate = time.Now().Truncate(24 * time.Hour)
		}

		startOfDay := targetDate
		endOfDay := targetDate.Add(24 * time.Hour)

		// 构建查询
		query := db.Model(&Player{}).Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay)

		// 处理区服筛选
		if serverParam != "" && serverParam != "0" {
			query = query.Where("gamesvr = ?", serverParam)
		}

		var count int64
		query.Distinct("roleid").Count(&count)
		c.JSON(http.StatusOK, gin.H{"active_player_count": count})
	})
	appLogger.Info("获取今天活跃玩家人数接口注册成功: GET /getactivateplayer")

	// 获取新增玩家人数（支持日期和区服筛选）
	r.GET("/getnewplayer", func(c *gin.Context) {
		// 获取查询参数
		dateParam := c.Query("date")
		serverParam := c.Query("server")

		// 处理日期参数
		var targetDate time.Time
		if dateParam != "" {
			if parsed, err := time.Parse("2006-01-02", dateParam); err == nil {
				targetDate = parsed
			} else {
				targetDate = time.Now().Truncate(24 * time.Hour)
			}
		} else {
			targetDate = time.Now().Truncate(24 * time.Hour)
		}

		startOfDay := targetDate
		endOfDay := targetDate.Add(24 * time.Hour)

		// 构建查询
		query := db.Model(&Player{}).Where("created_at >= ? AND created_at < ? AND new_player = ?", startOfDay, endOfDay, true)

		// 处理区服筛选
		if serverParam != "" && serverParam != "0" {
			query = query.Where("gamesvr = ?", serverParam)
		}

		var count int64
		query.Count(&count)
		c.JSON(http.StatusOK, gin.H{"new_player_count": count})
	})
	appLogger.Info("获取今天新增玩家人数接口注册成功: GET /getnewplayer")

	// 获取支付统计（支持日期和区服筛选）
	r.GET("/get_today_payment_stats", func(c *gin.Context) {
		// 获取查询参数
		dateParam := c.Query("date")
		serverParam := c.Query("server")

		// 处理日期参数
		var targetDate time.Time
		if dateParam != "" {
			if parsed, err := time.Parse("2006-01-02", dateParam); err == nil {
				targetDate = parsed
			} else {
				targetDate = time.Now().Truncate(24 * time.Hour)
			}
		} else {
			targetDate = time.Now().Truncate(24 * time.Hour)
		}

		startOfDay := targetDate
		endOfDay := targetDate.Add(24 * time.Hour)

		// 构建查询
		payingPlayerQuery := db.Model(&PayReport{}).Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay)
		totalPaymentQuery := db.Model(&PayReport{}).Where("created_at >= ? AND created_at < ?", startOfDay, endOfDay)

		// 处理区服筛选
		if serverParam != "" && serverParam != "0" {
			payingPlayerQuery = payingPlayerQuery.Where("gamesvr = ?", serverParam)
			totalPaymentQuery = totalPaymentQuery.Where("gamesvr = ?", serverParam)
		}

		var payingPlayerCount int64
		payingPlayerQuery.Distinct("roleid").Count(&payingPlayerCount)

		var totalPayment int64
		totalPaymentQuery.Select("coalesce(sum(money), 0)").Row().Scan(&totalPayment)

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
