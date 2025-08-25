package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger 日志系统
type Logger struct {
	mu       sync.RWMutex
	file     *os.File
	logger   *log.Logger
	logDir   string
	fileName string
}

// 全局日志实例
var appLogger *Logger

// 启动定时清理任务和日志系统
func init() {
	// 初始化日志系统
	initLogger()

	// 启动定时清理任务
	go startDailyCleanup()

	// 启动日志轮转任务
	go startLogRotation()
}

// startDailyCleanup 启动每天0点清理缓存的任务
func startDailyCleanup() {
	for {
		now := time.Now()
		// 计算下一个0点的时间
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		// 等待到下一个0点
		time.Sleep(next.Sub(now))

		// 清空玩家缓存
		playerCache.mu.Lock()
		playerCache.cache = make(map[string]*Player)
		playerCache.mu.Unlock()

		// 记录日志
		if appLogger != nil {
			appLogger.Info("玩家缓存已清空")
		}
	}
}

// startLogRotation 启动日志轮转任务
func startLogRotation() {
	for {
		now := time.Now()
		// 计算下一个0点的时间
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		// 等待到下一个0点
		time.Sleep(next.Sub(now))

		// 轮转日志文件
		if appLogger != nil {
			appLogger.Info("开始轮转日志文件")
			appLogger.rotateLogFile()
			appLogger.Info("日志文件轮转完成")
		}
	}
}

// initLogger 初始化日志系统
func initLogger() {
	// 创建log目录
	logDir := "../log"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		panic(fmt.Sprintf("创建日志目录失败: %v", err))
	}

	appLogger = &Logger{
		logDir: logDir,
	}

	// 创建今天的日志文件
	appLogger.rotateLogFile()
}

// rotateLogFile 轮转日志文件
func (l *Logger) rotateLogFile() {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 关闭旧文件
	if l.file != nil {
		l.file.Close()
	}

	// 生成新的文件名
	now := time.Now()
	fileName := fmt.Sprintf("logsvr_%s.log", now.Format("20060102"))
	filePath := filepath.Join(l.logDir, fileName)

	// 创建新文件
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(fmt.Sprintf("创建日志文件失败: %v", err))
	}

	l.file = file
	l.fileName = fileName
	l.logger = log.New(file, "", log.LstdFlags)

	// 写入日志文件头
	l.logger.Printf("=== 日志文件开始 - %s ===", now.Format("2006-01-02 15:04:05"))
}

// WriteLog 写入日志
func (l *Logger) WriteLog(level, message string) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.logger != nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		l.logger.Printf("[%s] [%s] %s", timestamp, level, message)
	}
}

// Info 信息日志
func (l *Logger) Info(message string) {
	l.WriteLog("INFO", message)
}

// Warning 警告日志
func (l *Logger) Warning(message string) {
	l.WriteLog("WARNING", message)
}

// Error 错误日志
func (l *Logger) Error(message string) {
	l.WriteLog("ERROR", message)
}

// Debug 调试日志
func (l *Logger) Debug(message string) {
	l.WriteLog("DEBUG", message)
}
