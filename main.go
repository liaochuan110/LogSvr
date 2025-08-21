package main

import (
	"fmt"
	"log"
	"os"

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
	configData, err := os.ReadFile("config.yaml")
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

	// 自动迁移表结构
	db.AutoMigrate(&OnlineNum{}, &Player{})

	r := gin.Default()

	// 注册接口
	RegisterRoutes(r, db)

	// 使用配置文件中的端口启动服务
	port := config.Server.Port
	r.Run(fmt.Sprintf(":%d", port))
}
