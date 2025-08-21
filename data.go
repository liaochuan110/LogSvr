package main

import (
	"gorm.io/gorm"
)

// OnlineNum 在线人数数据结构
type OnlineNum struct {
	gorm.Model
	GameSvrID int `gorm:"column:gamesvr_id;type:int;not null"`
	OnlineNum int `gorm:"column:online_num;type:int;not null"`
}

// Player 玩家数据结构
type Player struct {
	gorm.Model
	RoleID  string `gorm:"column:roleid;type:varchar(50);not null"`
	Name    string `gorm:"column:name;type:varchar(100);not null"`
	Level   int    `gorm:"column:level;type:int;not null"`
	GameSvr int    `gorm:"column:gamesvr;type:int;not null"`
}

// 设置表名
func (OnlineNum) TableName() string {
	return "online_num"
}

func (Player) TableName() string {
	return "player"
}
