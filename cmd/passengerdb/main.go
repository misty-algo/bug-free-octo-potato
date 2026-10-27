// Command passengerdb 初始化乘客端业务域的 MongoDB 集合与索引，并灌入最小种子数据。
//
// 用途：把乘客端业务域在目标库上一次性建好，避免手工在 Navicat 里点。
//
// 运行方式：
//   MONGODB_PASSWORD=xxx go run ./cmd/passengerdb -uri mongodb://127.0.0.1:27017 -user root -db huaxiaozhu
//
// 密码只从命令行参数或环境变量读取，MUST NOT 写进仓库里的配置文件。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/model"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

func main() {
	uri := flag.String("uri", "mongodb://127.0.0.1:27017", "MongoDB 连接串")
	user := flag.String("user", "", "MongoDB 用户名")
	password := flag.String("password", "", "MongoDB 密码")
	database := flag.String("db", "huaxiaozhu", "数据库名")
	flag.Parse()

	// 连接信息优先从环境变量读，避免出现在 shell 历史里，
	// 也避免明文出现在命令参数里。
	if value := strings.TrimSpace(os.Getenv("MONGODB_URI")); value != "" {
		*uri = value
	}
	if value := strings.TrimSpace(os.Getenv("MONGODB_USERNAME")); value != "" {
		*user = value
	}
	if value := strings.TrimSpace(os.Getenv("MONGODB_PASSWORD")); value != "" {
		*password = value
	}
	if value := strings.TrimSpace(os.Getenv("MONGODB_DATABASE")); value != "" {
		*database = value
	}

	cfg := mongox.DefaultConfig()
	cfg.URI = *uri
	cfg.Username = *user
	cfg.Password = *password
	cfg.Database = *database
	cfg.AppName = "passengerdb"
	cfg.ConnectTimeout = 10 * time.Second
	cfg.ServerSelectionTimeout = 10 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := mongox.NewClient(cfg)
	if err != nil {
		fail("connect mongodb: %v", err)
	}
	defer func() {
		_ = client.Disconnect(context.Background())
	}()
	if err := mongox.Ping(ctx, client); err != nil {
		fail("ping mongodb: %v", err)
	}
	db := mongox.Database(client, *database)
	fmt.Printf("connected: %s database: %s\n", *uri, *database)

	if err := ensureCollections(ctx, db); err != nil {
		fail("ensure collections: %v", err)
	}
	if err := ensureIndexes(ctx, db); err != nil {
		fail("ensure indexes: %v", err)
	}
	if err := seedQuickEntries(ctx, db); err != nil {
		fail("seed quick entries: %v", err)
	}
	fmt.Println("乘客端集合与索引初始化完成")
}

// ensureCollections 创建乘客端用到的全部集合。
func ensureCollections(ctx context.Context, db *mongo.Database) error {
	names := []string{
		model.CollectionAccounts,
		model.CollectionChannelBindings,
		model.CollectionSessions,
		model.CollectionVerificationCodes,
		model.CollectionEstimates,
		model.CollectionAddresses,
		model.CollectionOrders,
		model.CollectionOrderStatusLogs,
		model.CollectionQuickEntries,
		model.CollectionUsedQuickEntries,
		"passenger_coupons",
		"passenger_drivers",
	}
	for _, name := range names {
		if err := db.CreateCollection(ctx, name); err != nil {
			// 集合已存在不是错误。
			if !isNamespaceExists(err) {
				return fmt.Errorf("create collection %s: %w", name, err)
			}
			continue
		}
		fmt.Printf("created collection: %s\n", name)
	}
	return nil
}

// ensureIndexes 建立查询路径上的索引。
func ensureIndexes(ctx context.Context, db *mongo.Database) error {
	specs := []struct {
		collection string
		keys       bson.D
		name       string
		unique     bool
		sparse     bool
	}{
		// 账号：手机号哈希唯一索引，等值查询与唯一约束。
		{model.CollectionAccounts, bson.D{{Key: "phone_hash", Value: 1}}, "phone_hash_1", true, true},
		// 渠道绑定：渠道加渠道标识唯一，一个渠道标识只能绑一个账号。
		{model.CollectionChannelBindings, bson.D{{Key: "channel", Value: 1}, {Key: "channel_uid", Value: 1}}, "channel_uid_1", true, false},
		// 会话：按账号查会话，退出登录时按账号批量撤销。
		{model.CollectionSessions, bson.D{{Key: "account_id", Value: 1}, {Key: "revoked_at", Value: 1}}, "account_revoked_1", false, false},
		// 验证码：按号码查最新一条。
		{model.CollectionVerificationCodes, bson.D{{Key: "phone_hash", Value: 1}}, "phone_hash_1", false, false},
		// 估价单：单号唯一，下单时按单号查。
		{model.CollectionEstimates, bson.D{{Key: "no", Value: 1}}, "no_1", true, false},
		// 地址：按账号与类型查地址簿。
		{model.CollectionAddresses, bson.D{{Key: "account_id", Value: 1}, {Key: "type", Value: 1}, {Key: "use_count", Value: -1}}, "account_type_count_1", false, false},
		// 订单：订单号唯一，幂等键唯一，按账号查列表与进行中订单。
		{model.CollectionOrders, bson.D{{Key: "order_no", Value: 1}}, "order_no_1", true, false},
		{model.CollectionOrders, bson.D{{Key: "account_id", Value: 1}, {Key: "idempotency_key", Value: 1}}, "account_idempotency_1", true, true},
		{model.CollectionOrders, bson.D{{Key: "account_id", Value: 1}, {Key: "state", Value: 1}, {Key: "created_at", Value: -1}}, "account_state_created_1", false, false},
		{model.CollectionOrders, bson.D{{Key: "estimate_no", Value: 1}}, "estimate_no_1", false, true},
		// 状态留痕：按订单查回放轨迹。
		{model.CollectionOrderStatusLogs, bson.D{{Key: "order_id", Value: 1}, {Key: "created_at", Value: 1}}, "order_created_1", false, false},
		// 快速入口：按启用状态与排序取配置。
		{model.CollectionQuickEntries, bson.D{{Key: "enabled", Value: 1}, {Key: "sort", Value: 1}}, "enabled_sort_1", false, false},
		// 已用入口：账号加入口唯一，重复点不写两条。
		{model.CollectionUsedQuickEntries, bson.D{{Key: "account_id", Value: 1}, {Key: "entry_id", Value: 1}}, "account_entry_1", true, false},
		// 优惠券：按账号查可用券。
		{"passenger_coupons", bson.D{{Key: "account_id", Value: 1}, {Key: "state", Value: 1}}, "account_state_1", false, false},
		// 司机档案：按账号与订单查司机。
		{"passenger_drivers", bson.D{{Key: "account_id", Value: 1}}, "account_id_1", false, false},
		{"passenger_drivers", bson.D{{Key: "order_id", Value: 1}}, "order_id_1", false, true},
	}

	for _, spec := range specs {
		opts := options.Index().SetName(spec.name)
		if spec.unique {
			opts.SetUnique(true)
		}
		if spec.sparse {
			opts.SetSparse(true)
		}
		indexModel := mongo.IndexModel{Keys: spec.keys, Options: opts}
		if _, err := db.Collection(spec.collection).Indexes().CreateOne(ctx, indexModel); err != nil {
			if isIndexExists(err) {
				continue
			}
			return fmt.Errorf("create index on %s: %w", spec.collection, err)
		}
		fmt.Printf("index: %s %s\n", spec.collection, spec.name)
	}
	return nil
}

// seedQuickEntries 灌入首次快速入口配置。
func seedQuickEntries(ctx context.Context, db *mongo.Database) error {
	collection := db.Collection(model.CollectionQuickEntries)
	now := time.Now().UTC()
	// 生效区间给足跨度，避免配置过期后入口凭空消失。
	startAt := now.Add(-365 * 24 * time.Hour)
	endAt := now.Add(365 * 24 * time.Hour)
	entries := []bson.M{
		{"_id": "scan_ride", "title": "扫码乘车", "icon": "scan", "action": "scan", "target": "", "sort": 1, "enabled": true, "start_at": startAt, "end_at": endAt, "is_deleted": false, "created_at": now, "updated_at": now},
		{"_id": "reserve_ride", "title": "预约用车", "icon": "clock", "action": "navigate", "target": "/reserve", "sort": 2, "enabled": true, "start_at": startAt, "end_at": endAt, "is_deleted": false, "created_at": now, "updated_at": now},
		{"_id": "family_ride", "title": "帮家人叫车", "icon": "family", "action": "navigate", "target": "/family", "sort": 3, "enabled": true, "start_at": startAt, "end_at": endAt, "is_deleted": false, "created_at": now, "updated_at": now},
		{"_id": "airport_pickup", "title": "机场接送", "icon": "plane", "action": "navigate", "target": "/airport", "sort": 4, "enabled": true, "start_at": startAt, "end_at": endAt, "is_deleted": false, "created_at": now, "updated_at": now},
	}
	for _, entry := range entries {
		if _, err := collection.ReplaceOne(ctx, bson.M{"_id": entry["_id"]}, entry, options.Replace().SetUpsert(true)); err != nil {
			return err
		}
	}
	fmt.Printf("quick_entries: %d 条首次快速入口已就位\n", len(entries))
	return nil
}

// isNamespaceExists 判断错误是否是集合已存在。
func isNamespaceExists(err error) bool {
	var commandErr mongo.CommandError
	if ok := asCommandError(err, &commandErr); !ok {
		return false
	}
	return commandErr.Code == 48
}

// isIndexExists 判断错误是否是索引已存在。
func isIndexExists(err error) bool {
	var commandErr mongo.CommandError
	if ok := asCommandError(err, &commandErr); !ok {
		return false
	}
	return commandErr.Code == 85 || commandErr.Code == 86
}

// asCommandError 把错误转成 CommandError。
func asCommandError(err error, target *mongo.CommandError) bool {
	for err != nil {
		if commandErr, ok := err.(mongo.CommandError); ok {
			*target = commandErr
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}

// fail 打印错误并退出。
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
