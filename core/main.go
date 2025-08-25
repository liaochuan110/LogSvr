package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 配置文件结构
type Config struct {
	Database struct {
		Mysql struct {
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
			Dbname   string `yaml:"dbname"`
		} `yaml:"mysql"`
	} `yaml:"database"`
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
}

var db *gorm.DB

func main() {
	// 读取配置文件
	configData, err := os.ReadFile("../config/.config.yaml")
	if err != nil {
		log.Fatalf("读取配置文件失败: %v", err)
	}

	var config Config
	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		log.Fatalf("解析配置文件失败: %v", err)
	}

	// 初始化MySQL连接
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		config.Database.Mysql.User,
		config.Database.Mysql.Password,
		config.Database.Mysql.Host,
		config.Database.Mysql.Port,
		config.Database.Mysql.Dbname)

	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	appLogger.Info("数据库连接成功")

	// 自动迁移表结构
	db.AutoMigrate(&OnlineNum{}, &Player{}, &PayReport{})

	// 初始化用户管理器
	InitUserManager(db)

	// 从数据库加载今日充值数据，预热缓存
	payRankCache.LoadTodayPayData(db)

	r := gin.Default()

	// 注册接口
	RegisterRoutes(r, db)

	// 记录服务器启动日志
	if appLogger != nil {
		appLogger.Info("服务器启动成功")
	}

	// 启动每日0点重置缓存的定时器
	go startDailyResetTimer()

	// 使用配置文件中的端口启动服务
	port := config.Server.Port
	log.Printf("服务器启动在端口: %d", port)
	r.Run(fmt.Sprintf(":%d", port))
}

// 每天0点定时重置
func startDailyResetTimer() {
	for {
		// 计算下一个0点
		now := time.Now()
		next := now.Add(time.Hour * 24)
		next = time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, next.Location())
		t := time.NewTimer(next.Sub(now))
		<-t.C
		// 0点执行清理
		payRankCache.ClearCache()
	}
}
