package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(r *gin.Engine, db *gorm.DB) {
	// 提供静态页面
	r.GET("/", func(c *gin.Context) {
		c.File("templates/index.html")
	})

	// 在线人数上报接口
	r.POST("/onlineNum", func(c *gin.Context) {
		var data struct {
			GameSvrID int `json:"gamesvrID" binding:"required"`
			OnlineNum int `json:"onlineNum" binding:"required"`
		}
		if err := c.ShouldBindJSON(&data); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := db.Create(&OnlineNum{
			GameSvrID: data.GameSvrID,
			OnlineNum: data.OnlineNum,
		}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})

	// 玩家登录接口
	r.POST("/login", func(c *gin.Context) {
		var data struct {
			RoleID  string `json:"roleid" binding:"required"`
			Name    string `json:"name" binding:"required"`
			Level   int    `json:"level" binding:"required"`
			GameSvr int    `json:"gamesvr" binding:"required"`
		}
		if err := c.ShouldBindJSON(&data); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := db.Create(&Player{
			RoleID:  data.RoleID,
			Name:    data.Name,
			Level:   data.Level,
			GameSvr: data.GameSvr,
		}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})

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

	// 获取今天活跃玩家人数
	r.GET("/getactivateplayer", func(c *gin.Context) {
		var count int64
		startOfDay := time.Now().Truncate(24 * time.Hour)
		db.Model(&Player{}).Where("created_at >= ?", startOfDay).Distinct("roleid").Count(&count)
		c.JSON(http.StatusOK, gin.H{"active_player_count": count})
	})
}
