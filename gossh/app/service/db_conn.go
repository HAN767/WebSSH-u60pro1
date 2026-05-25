package service

import (
	"gossh/app/config"
	"gossh/app/model"
	"gossh/gin"
	"gossh/gorm"
	"log/slog"
)

type DbConnConf struct {
	DbFile string `form:"db_file" binding:"required,min=1,max=255" json:"db_file"`
	DbType string `form:"db_type" binding:"required,oneof=sqlite" json:"db_type"`
}

func DbConnCheck(c *gin.Context) {
	if config.DefaultConfig.IsInit {
		c.JSON(200, gin.H{"code": 1, "msg": "系统已经完成初始化配置"})
		return
	}

	var dbConf DbConnConf
	if err := c.ShouldBind(&dbConf); err != nil {
		c.JSON(200, gin.H{"code": 1, "msg": err.Error()})
		return
	}
	err := DbConnTestCheck(dbConf)
	if err != nil {
		c.JSON(200, gin.H{"code": 1, "msg": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "msg": "连接成功"})
}

func DbConnTestCheck(dbConf DbConnConf) error {
	slog.Info("DB link check", "db_type", dbConf.DbType, "db_file", dbConf.DbFile)
	if dbConf.DbType != "sqlite" {
		return model.ErrUnsupportedDB
	}

	Db, err := model.GetSqliteDb(dbConf.DbFile)
	if err != nil {
		return err
	}
	defer closeTestDB(Db)
	return Db.Exec("select 1=1;").Error
}

func closeTestDB(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		slog.Warn("close test db skipped", "err_msg", err.Error())
		return
	}
	if err := sqlDB.Close(); err != nil {
		slog.Warn("close test db failed", "err_msg", err.Error())
	}
}
