package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"
	"zigbee/config"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func ConnectDB(cfg config.Config) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort)

	// 使用重试和指数退避逻辑进行连接
	const maxRetries = 10
	for i := range maxRetries {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			log.Printf("Error opening database: %v. Retrying in %d seconds...", err, i+1)
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		if err = db.Ping(); err == nil {
			DB = db
			log.Println("Database connection successful!")

			// 配置连接池
			DB.SetMaxOpenConns(25)
			DB.SetMaxIdleConns(10)
			DB.SetConnMaxLifetime(5 * time.Minute)

			return
		}
		log.Printf("Error pinging database: %v. Retrying in %d seconds...", err, i+1)
		db.Close() // 关闭失败的连接
		time.Sleep(time.Duration(i+1) * time.Second)
	}
}
