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

	// === 公开路由（无需认证） ===

	// 静态资源和常见请求处理（避免404错误）
	r.GET("/.well-known/*path", func(c *gin.Context) {
		// Chrome开发者工具相关请求，直接返回204避免日志噪音
		c.Status(http.StatusNoContent)
	})
	r.GET("/favicon.ico", func(c *gin.Context) {
		// 避免favicon.ico的404错误
		c.Status(http.StatusNoContent)
	})

	// 登录页面
	r.GET("/login", func(c *gin.Context) {
		c.File("../templates/login.html")
	})
	appLogger.Info("登录页面路由注册成功: GET /login")

	// 登录接口
	r.POST("/login", LoginHandler)
	appLogger.Info("登录接口注册成功: POST /login")

	// 退出登录接口
	r.POST("/logout", LogoutHandler)
	appLogger.Info("退出登录接口注册成功: POST /logout")

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

		// 同时存入数据库（添加date_int字段）
		currentDateInt := GetCurrentDateInt()
		if err := db.Create(&OnlineNum{
			GameSvrID: data.GameSvrID,
			OnlineNum: data.OnlineNum,
			DateInt:   currentDateInt,
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
			NewPlayer int    `json:"new_player" form:"new_player"` // 改为int类型：0=非新玩家，1=新玩家
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
			// 如果RoleID不在缓存中，写入数据库并缓存数据（添加date_int字段）
			currentDateInt := GetCurrentDateInt()
			player := &Player{
				RoleID:    data.RoleID,
				Name:      data.Name,
				Level:     data.Level,
				GameSvr:   data.GameSvr,
				NewPlayer: data.NewPlayer == 1, // 将int转换为bool：1=true，0=false
				DateInt:   currentDateInt,
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

		// 创建支付记录（添加date_int字段）
		currentDateInt := GetCurrentDateInt()
		payReport := &PayReport{
			RoleID:   data.RoleID,
			Name:     data.Name,
			Level:    data.Level,
			GameSvr:  data.GameSvr,
			Money:    data.Money,
			VipLevel: data.VipLevel,
			DateInt:  currentDateInt,
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

	// === 需要认证的路由 ===
	protected := r.Group("/")
	protected.Use(AuthMiddleware())

	// 主页面（需要登录）
	protected.GET("/", func(c *gin.Context) {
		c.File("../templates/index.html")
	})
	appLogger.Info("主页面路由注册成功: GET / (需要认证)")

	// 用户管理页面
	protected.GET("/users", func(c *gin.Context) {
		c.File("../templates/user_manager.html")
	})
	appLogger.Info("用户管理页面路由注册成功: GET /users (需要认证)")

	// 获取充值排行榜（优化版：使用整型日期字段）
	protected.GET("/pay_rank", func(c *gin.Context) {
		// 获取查询参数
		dateParam := c.Query("date")
		serverParam := c.Query("server")

		// 将日期转换为整型
		dateInt := DateToInt(dateParam)

		// 如果是今天且全服，直接使用缓存
		currentDateInt := GetCurrentDateInt()
		if dateInt == currentDateInt && (serverParam == "" || serverParam == "0") {
			rank := payRankCache.GetRank()
			c.JSON(http.StatusOK, gin.H{"rank": rank})
			return
		}

		// 否则从数据库查询（使用整型日期字段）
		query := db.Model(&PayReport{}).Where("date_int = ?", dateInt)

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

	// 获取在线人数曲线（优化版：使用整型日期字段）
	protected.GET("/today_online", func(c *gin.Context) {
		// 获取查询参数
		dateParam := c.Query("date")
		serverParam := c.Query("server")

		// 将日期转换为整型
		dateInt := DateToInt(dateParam)

		// 1. 构建基础SQL查询（使用整型日期字段）
		baseSQL := `
			SELECT
				minute,
				SUM(online_num) as online_num
			FROM (
				SELECT
					DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:00') as minute,
					gamesvr_id,
					MAX(online_num) as online_num
				FROM online_num
				WHERE date_int = ?`

		args := []interface{}{dateInt}

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

		// 3. 在Go中聚合数据：计算每5分钟内的峰值
		fiveMinuteMap := make(map[string]int) // Key: "15:04", Value: max online num

		for _, row := range perMinuteResults {
			// 将数据库返回的无时区时间字符串解析为UTC时间
			t, err := time.ParseInLocation("2006-01-02 15:04:05", row.Minute, time.UTC)
			if err != nil {
				continue // Skip if format is wrong
			}

			// 向下取整到最近的5分钟时间点 (在UTC下计算)
			minute := t.Minute()
			remainder := minute % 5
			fiveMinIntervalTime := t.Add(time.Duration(-remainder) * time.Minute)
			label := fiveMinIntervalTime.Format("15:04")

			// 如果当前分钟的人数 > map中记录的这个5分钟区间的最大人数，则更新
			if row.OnlineNum > fiveMinuteMap[label] {
				fiveMinuteMap[label] = row.OnlineNum
			}
		}

		// 4. 组装最终返回给前端的数据
		// 根据 dateInt 构建目标日期
		startOfDay, err := DateIntToTime(dateInt)
		if err != nil {
			appLogger.Error(fmt.Sprintf("日期转换错误: %v", err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的日期参数"})
			return
		}

		var finalResults []struct {
			Minute    time.Time `json:"Minute"`
			OnlineNum int       `json:"OnlineNum"`
		}
		for i := 0; i < 288; i++ { // 288 = 24 * 60 / 5
			// 生成从指定日期0点开始的每个5分钟时间点
			t := startOfDay.Add(time.Duration(i*5) * time.Minute)
			label := t.Format("15:04")

			// 从 map中获取这个5分钟区间的峰值
			onlineNum := fiveMinuteMap[label] // 如果map中没有，默认为0

			finalResults = append(finalResults, struct {
				Minute    time.Time `json:"Minute"`
				OnlineNum int       `json:"OnlineNum"`
			}{Minute: t, OnlineNum: onlineNum})
		}

		c.JSON(http.StatusOK, gin.H{"data": finalResults})
	})
	appLogger.Info("获取今天在线人数统计接口注册成功: GET /today_online")

	// 获取活跃玩家人数（优化版：使用整型日期字段）
	protected.GET("/getactivateplayer", func(c *gin.Context) {
		// 获取查询参数
		dateParam := c.Query("date")
		serverParam := c.Query("server")

		// 将日期转换为整型
		dateInt := DateToInt(dateParam)

		// 构建查询（使用整型日期字段）
		query := db.Model(&Player{}).Where("date_int = ?", dateInt)

		// 处理区服筛选
		if serverParam != "" && serverParam != "0" {
			query = query.Where("gamesvr = ?", serverParam)
		}

		var count int64
		query.Distinct("roleid").Count(&count)
		c.JSON(http.StatusOK, gin.H{"active_player_count": count})
	})
	appLogger.Info("获取今天活跃玩家人数接口注册成功: GET /getactivateplayer")

	// 获取新增玩家人数（优化版：使用整型日期字段）
	protected.GET("/getnewplayer", func(c *gin.Context) {
		// 获取查询参数
		dateParam := c.Query("date")
		serverParam := c.Query("server")

		// 将日期转换为整型
		dateInt := DateToInt(dateParam)

		// 构建查询（使用整型日期字段）
		query := db.Model(&Player{}).Where("date_int = ? AND new_player = ?", dateInt, true)

		// 处理区服筛选
		if serverParam != "" && serverParam != "0" {
			query = query.Where("gamesvr = ?", serverParam)
		}

		var count int64
		query.Count(&count)
		c.JSON(http.StatusOK, gin.H{"new_player_count": count})
	})
	appLogger.Info("获取今天新增玩家人数接口注册成功: GET /getnewplayer")

	// 获取支付统计（优化版：使用整型日期字段）
	protected.GET("/get_today_payment_stats", func(c *gin.Context) {
		// 获取查询参数
		dateParam := c.Query("date")
		serverParam := c.Query("server")

		// 将日期转换为整型
		dateInt := DateToInt(dateParam)

		// 构建查询（使用整型日期字段）
		payingPlayerQuery := db.Model(&PayReport{}).Where("date_int = ?", dateInt)
		totalPaymentQuery := db.Model(&PayReport{}).Where("date_int = ?", dateInt)

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

	// === 用户管理接口 ===

	// 创建用户
	protected.POST("/api/users", func(c *gin.Context) {
		var createRequest struct {
			Username    string `json:"username" binding:"required,min=3,max=50"`
			Password    string `json:"password" binding:"required,min=6"`
			DisplayName string `json:"display_name" binding:"required,max=100"`
		}

		if err := c.ShouldBindJSON(&createRequest); err != nil {
			appLogger.Error("创建用户请求参数错误: " + err.Error())
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "请求参数错误",
				"details": err.Error(),
			})
			return
		}

		err := userManager.CreateUser(createRequest.Username, createRequest.Password, createRequest.DisplayName)
		if err != nil {
			appLogger.Error("创建用户失败: " + err.Error())
			c.JSON(http.StatusConflict, gin.H{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "用户创建成功",
		})
	})
	appLogger.Info("创建用户接口注册成功: POST /api/users")

	// 获取用户列表
	protected.GET("/api/users", func(c *gin.Context) {
		users := userManager.ListUsers()
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   users,
			"count":  len(users),
		})
	})
	appLogger.Info("获取用户列表接口注册成功: GET /api/users")

	// 更新用户密码
	protected.PUT("/api/users/:username/password", func(c *gin.Context) {
		username := c.Param("username")
		var updateRequest struct {
			NewPassword string `json:"new_password" binding:"required,min=6"`
		}

		if err := c.ShouldBindJSON(&updateRequest); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "请求参数错误",
			})
			return
		}

		err := userManager.UpdateUserPassword(username, updateRequest.NewPassword)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "密码更新成功",
		})
	})
	appLogger.Info("更新用户密码接口注册成功: PUT /api/users/:username/password")

	// 当前用户修改自己密码的接口
	protected.PUT("/api/current-user/password", func(c *gin.Context) {
		var changeRequest struct {
			CurrentPassword string `json:"current_password" binding:"required"`
			NewPassword     string `json:"new_password" binding:"required,min=6"`
		}

		if err := c.ShouldBindJSON(&changeRequest); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "请求参数错误",
			})
			return
		}

		// 获取当前登录用户
		currentUser, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "获取用户信息失败",
			})
			return
		}

		username := currentUser.(string)

		// 验证当前密码
		_, isValid := userManager.ValidateUser(username, changeRequest.CurrentPassword)
		if !isValid {
			appLogger.Warning("用户修改密码失败: 当前密码验证错误 - 用户=" + username)
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "当前密码错误",
			})
			return
		}

		// 更新密码
		err := userManager.UpdateUserPassword(username, changeRequest.NewPassword)
		if err != nil {
			appLogger.Error("用户修改密码失败: " + err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": "密码修改失败",
			})
			return
		}

		appLogger.Info("用户修改密码成功: " + username)
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "密码修改成功",
		})
	})
	appLogger.Info("当前用户修改密码接口注册成功: PUT /api/current-user/password")

	// 获取当前用户信息接口
	protected.GET("/api/current-user", func(c *gin.Context) {
		// 获取当前登录用户
		currentUser, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "获取用户信息失败",
			})
			return
		}

		username := currentUser.(string)

		// 获取用户详细信息
		user, exists := userManager.GetUser(username)
		if !exists || user == nil {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  "error",
				"message": "用户不存在",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":       "success",
			"username":     user.Username,
			"display_name": user.DisplayName,
			"is_active":    user.IsActive,
			"created_at":   user.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	})
	appLogger.Info("获取当前用户信息接口注册成功: GET /api/current-user")

	// 停用用户
	protected.DELETE("/api/users/:username", func(c *gin.Context) {
		username := c.Param("username")

		err := userManager.DeactivateUser(username)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "用户已停用",
		})
	})
	appLogger.Info("停用用户接口注册成功: DELETE /api/users/:username")

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
