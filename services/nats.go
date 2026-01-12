package services

import (
	"log"

	"github.com/nats-io/nats.go"
)

var NC *nats.Conn

func InitNats(url string) {
	if url == "" {
		url = nats.DefaultURL
	}

	// 设置重连选项
	opts := []nats.Option{
		// 关键选项：在初始连接失败时，启动重连逻辑
		nats.RetryOnFailedConnect(true),
	}

	// nats.Connect() 现在会在后台循环重试，直到连接成功
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		log.Fatal("nats connect error:", err)
	}
	NC = nc
}
