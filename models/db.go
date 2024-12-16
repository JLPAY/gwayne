package models

import (
	"fmt"
	"github.com/JLPAY/gwayne/pkg/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"k8s.io/klog/v2"
	"log"
	"os"
	"time"
)

// DB 数据库链接单例
var DB *gorm.DB

// 初始化数据库
func InitDB() {
	switch config.Conf.DataBase.Driver {
	case "mysql":
		DB = ConnMysql()
		/*case "sqlite3":
		DB = ConnSqlite()*/
	}

	// 自动迁移模型
	if err := dbAutoMigrate(); err != nil {
		klog.Exitf("failed to migrate: %v", err)
	}

	// 检查初始数据是否存在
	if err := insertInitialData(DB); err != nil {
		klog.Exitf("failed to insert initial data: %v", err)
	}
}

func ConnMysql() *gorm.DB {
	// 初始化GORM日志配置
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             time.Second, // Slow SQL threshold
			LogLevel:                  logger.Info, // Log level
			IgnoreRecordNotFoundError: true,        // Ignore ErrRecordNotFound error for logger
			Colorful:                  false,       // Disable color
		},
	)

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&collation=utf8mb4_general_ci&parseTime=True&loc=Local&timeout=10000ms",
		config.Conf.DataBase.DBUser,
		config.Conf.DataBase.DBPassword,
		config.Conf.DataBase.Host,
		config.Conf.DataBase.Port,
		config.Conf.DataBase.DBName,
	)
	// 隐藏密码
	showDsn := fmt.Sprintf("%s:******@tcp(%s:%d)/%s?charset=utf8mb4&collation=utf8mb4_general_ci&parseTime=True&loc=Local&timeout=10000ms",
		config.Conf.DataBase.DBUser,
		config.Conf.DataBase.Host,
		config.Conf.DataBase.Port,
		config.Conf.DataBase.DBName,
	)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		// 禁用外键(指定外键时不会在mysql创建真实的外键约束)
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   newLogger,
		//Logger:                                   logger.Default.LogMode(logger.Silent), // 关闭日志
	})
	if err != nil {
		klog.Exitf("初始化mysql数据库异常: %v", err)
	}
	// 开启mysql日志
	if config.Conf.DataBase.LogMode {
		db.Debug()
	} else {
		// 禁用 SQL 日志，设置 Silent 模式
		db.Logger = db.Logger.LogMode(logger.Silent)
	}
	klog.Infof("初始化mysql数据库完成! dsn: %s", showDsn)
	return db
}

// 自动迁移表结构
func dbAutoMigrate() error {
	err := DB.AutoMigrate(
		&User{},
		&Cluster{},
		/*&model.Role{},
		&model.Group{},
		&model.Menu{},
		&model.Api{},
		&model.OperationLog{},
		&model.FieldRelation{},*/
	)
	if err != nil {
		return err
	}
	return nil
}

// 插入初始化数据
func insertInitialData(db *gorm.DB) error {
	// 检查是否有用户数据
	var count int64
	if err := db.Model(&User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to count users: %v", err)
	}

	// 如果没有用户数据，则执行插入初始数据
	if count == 0 {
		// 启动事务
		tx := db.Begin()
		if tx.Error != nil {
			return fmt.Errorf("failed to start transaction: %v", tx.Error)
		}

		// 执行 SQL 插入语句
		for _, insertSql := range InitialData {
			if err := tx.Exec(insertSql).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to insert initial data: %v", err)
			}
		}

		// 提交事务
		if err := tx.Commit().Error; err != nil {
			return fmt.Errorf("failed to commit transaction: %v", err)
		}

		klog.Info("Database initialized successfully!")
	}

	return nil
}
