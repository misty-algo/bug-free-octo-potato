package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/repo"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

// driver-schema 是车辆资料域的迁移入口：
//
//	go run ./cmd/driver-schema          # 只校验
//	go run ./cmd/driver-schema -apply   # 建集合与索引
func main() {
	apply := flag.Bool("apply", false, "create missing collections and indexes")
	flag.Parse()
	if err := config.InitGlobal("configs/config.yaml"); err != nil {
		panic(err)
	}
	cfg := config.MustGlobal()
	client, err := mongox.NewClient(cfg.MongoDB.MongoxConfig())
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer client.Disconnect(context.Background())
	db := mongox.Database(client, cfg.MongoDB.Database)
	if err := repo.ValidateMongoDriverSchema(ctx, db, *apply); err != nil {
		fmt.Println("driver schema:", err)
		return
	}
	fmt.Println("driver schema ok")
}
