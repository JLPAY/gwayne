package initial

import (
	"fmt"
	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"regexp"
)

func InitDb() {
	// ensure database exist
	err := ensureDatabase()
	if err != nil {
		panic(err)
	}

	models.InitDB()
}

func ensureDatabase() error {
	// 验证数据库名称，防止 SQL 注入
	dbName := config.Conf.DataBase.DBName
	if !isValidDatabaseName(dbName) {
		return fmt.Errorf("invalid database name: %s (only alphanumeric and underscore allowed)", dbName)
	}

	// 构建数据库连接字符串（不指定数据库名，确保是连接到 MySQL 服务）
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/",
		config.Conf.DataBase.DBUser,
		config.Conf.DataBase.DBPassword,
		config.Conf.DataBase.Host,
		config.Conf.DataBase.Port,
		//"mysql", // 使用 "mysql" 数据库作为默认数据库
	)

	// 使用 GORM 连接数据库
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // 禁用 SQL 日志
		//Logger: logger.Default.LogMode(logger.Info), // 显示 SQL 日志
	})
	if err != nil {
		//fmt.Println("Error connecting to DB:", err) // 打印详细错误信息
		return fmt.Errorf("failed to connect to MySQL: %w", err)
	}

	// 检查数据库是否存在
	var result int64
	err = db.Raw("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", dbName).Scan(&result).Error
	if err != nil {
		return fmt.Errorf("failed to check if database exists: %w", err)
	}

	if result == 0 {
		// 数据库不存在，创建数据库
		log.Println("Database does not exist, creating database...")

		// 使用原生 SQL 创建数据库（数据库名已通过白名单验证）
		err = db.Exec(fmt.Sprintf("CREATE DATABASE %s CHARACTER SET utf8 COLLATE utf8_general_ci;", dbName)).Error
		if err != nil {
			return fmt.Errorf("failed to create database: %w", err)
		}
		log.Println("Database created successfully.")
	}

	return nil
}

// isValidDatabaseName 验证数据库名称只包含字母、数字和下划线
func isValidDatabaseName(name string) bool {
	if name == "" {
		return false
	}
	validName := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	return validName.MatchString(name)
}
