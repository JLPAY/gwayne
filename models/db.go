package models

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/JLPAY/gwayne/pkg/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"k8s.io/klog/v2"
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
	// 先处理 terminal_command_rule 表的迁移（如果表已存在且列类型不匹配）
	if err := migrateTerminalCommandRuleTable(); err != nil {
		klog.Warningf("Failed to migrate terminal_command_rule table: %v", err)
		// 如果迁移失败，尝试删除表重新创建
		if err := DB.Exec("DROP TABLE IF EXISTS terminal_command_rule").Error; err != nil {
			klog.Warningf("Failed to drop terminal_command_rule table: %v", err)
		}
	}

	err := DB.AutoMigrate(
		&User{},
		&Cluster{},
		&AIBackend{},
		&TerminalCommandRule{},
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

// migrateTerminalCommandRuleTable 迁移 terminal_command_rule 表
func migrateTerminalCommandRuleTable() error {
	// 检查表是否存在
	var tableExists bool
	err := DB.Raw("SELECT COUNT(*) > 0 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'terminal_command_rule'").Scan(&tableExists).Error
	if err != nil {
		return err
	}

	if !tableExists {
		// 表不存在，直接返回，让 AutoMigrate 创建
		return nil
	}

	// 检查 rule_type 列的类型
	var columnType string
	err = DB.Raw("SELECT DATA_TYPE FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'terminal_command_rule' AND column_name = 'rule_type'").Scan(&columnType).Error
	if err != nil {
		// 列不存在，直接返回
		return nil
	}

	// 如果列类型不是 int 或 tinyint，需要转换
	if columnType != "int" && columnType != "tinyint" && columnType != "smallint" && columnType != "mediumint" && columnType != "bigint" {
		// 先删除表中的数据（因为类型转换可能导致数据丢失）
		klog.Info("Converting terminal_command_rule.rule_type column from string to int, clearing existing data")
		if err := DB.Exec("DELETE FROM terminal_command_rule").Error; err != nil {
			return fmt.Errorf("failed to clear terminal_command_rule table: %v", err)
		}
		// 修改列类型
		if err := DB.Exec("ALTER TABLE terminal_command_rule MODIFY COLUMN rule_type INT DEFAULT 0").Error; err != nil {
			return fmt.Errorf("failed to alter rule_type column: %v", err)
		}
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
