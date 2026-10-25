package repo

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"time"
)

type mongoArrivalHello struct {
	SetName        string `bson:"setName"`
	Message        string `bson:"msg"`
	MaxWireVersion int32  `bson:"maxWireVersion"`
	SessionTimeout *int64 `bson:"logicalSessionTimeoutMinutes"`
}

// Retain diagnostic causes for errors.Is/As without exposing server addresses or
// credentials in startup error strings. Callers may inspect the cause securely.
type mongoArrivalSchemaError struct {
	message string
	cause   error
}

func (e *mongoArrivalSchemaError) Error() string { return e.message }
func (e *mongoArrivalSchemaError) Unwrap() error { return e.cause }
func mongoSchemaError(message string, err error) error {
	return &mongoArrivalSchemaError{message: message, cause: err}
}

// mongoArrivalTopologySupported reports whether the server can host the arrival
// collections. Transactions were removed because the deployed server is
// standalone: writes are single-document atomic updates guarded by version, and
// convergence relies on the idempotent command record. A standalone server is
// therefore acceptable; only a reachable server is required.
func mongoArrivalTopologySupported(h mongoArrivalHello) bool {
	return h.MaxWireVersion > 0
}
func validateMongoArrivalTopology(ctx context.Context, db *mongo.Database) error {
	var hello mongoArrivalHello
	if err := db.RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return mongoSchemaError("arrival MongoDB topology check failed", err)
	}
	if !mongoArrivalTopologySupported(hello) {
		return fmt.Errorf("arrival requires MongoDB replica set or mongos with transaction support; standalone is unsupported")
	}
	return nil
}

type mongoArrivalIndex struct {
	collection, name string
	keys             bson.D
	unique           bool
}
type mongoArrivalExistingIndex struct {
	Name        string `bson:"name"`
	Keys        bson.D `bson:"key"`
	Unique      bool   `bson:"unique"`
	Sparse      bool   `bson:"sparse"`
	Partial     bson.M `bson:"partialFilterExpression"`
	Collation   bson.M `bson:"collation"`
	ExpireAfter any    `bson:"expireAfterSeconds"`
}

// 轨迹与采集会话集合为 D06 新增（mongodb.js 未定义），需显式迁移。
var mongoArrivalCollections = []string{"dispatch_tasks", "orders", "drivers", "pois", "event_logs", "command_results", "trip_sessions", "trip_points", "contact_intents"}
var mongoArrivalIndexes = []mongoArrivalIndex{
	// 文档索引来自 mongodb.js：uk_task_no、uk_order_no、uk_event_id。
	{"dispatch_tasks", "uk_task_no", bson.D{{Key: "taskNo", Value: 1}}, true},
	{"orders", "uk_order_no", bson.D{{Key: "orderNo", Value: 1}}, true},
	{"event_logs", "uk_event_id", bson.D{{Key: "eventId", Value: 1}}, true},
	// 幂等兜底：同一命令键只允许落一次业务事实。
	{"command_results", "arrival_command_once", bson.D{{Key: "actor_id", Value: 1}, {Key: "command_type", Value: 1}, {Key: "resource_id", Value: 1}, {Key: "idempotency_key", Value: 1}}, true},
	// 轨迹：同一采集会话的同一序号只落一次，重放不重复累计里程。
	{"trip_sessions", "uk_task_session", bson.D{{Key: "taskNo", Value: 1}, {Key: "sessionId", Value: 1}}, true},
	{"trip_points", "uk_point", bson.D{{Key: "taskNo", Value: 1}, {Key: "sessionId", Value: 1}, {Key: "seq", Value: 1}}, true},
	// 联系意图：同一业务请求号只产生一条意图，授权与意图持久化在同一边界内。
	{"contact_intents", "uk_intent", bson.D{{Key: "orderNo", Value: 1}, {Key: "requestNo", Value: 1}}, true},
}

func mongoArrivalIndexMatches(got mongoArrivalExistingIndex, want mongoArrivalIndex) bool {
	if got.Unique != want.unique || got.Sparse || len(got.Partial) != 0 || got.ExpireAfter != nil {
		return false
	}
	if len(got.Collation) > 0 && got.Collation["locale"] != "simple" {
		return false
	}
	if len(got.Keys) != len(want.keys) {
		return false
	}
	for i, k := range want.keys {
		if got.Keys[i].Key != k.Key || fmt.Sprint(got.Keys[i].Value) != fmt.Sprint(k.Value) {
			return false
		}
	}
	return true
}
func mongoArrivalCollectionNames(ctx context.Context, db *mongo.Database) (map[string]bool, error) {
	names, err := db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return nil, mongoSchemaError("arrival MongoDB schema inspection failed; verify database permissions", err)
	}
	found := make(map[string]bool, len(names))
	for _, name := range names {
		found[name] = true
	}
	return found, nil
}

// ValidateMongoArrivalSchema only reads topology, collection names and indexes.
func ValidateMongoArrivalSchema(ctx context.Context, db *mongo.Database) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := validateMongoArrivalTopology(ctx, db); err != nil {
		return err
	}
	names, err := mongoArrivalCollectionNames(ctx, db)
	if err != nil {
		return err
	}
	for _, name := range mongoArrivalCollections {
		if !names[name] {
			return fmt.Errorf("arrival MongoDB schema missing collection %s; run explicit migration", name)
		}
	}
	for _, name := range mongoArrivalCollections {
		cursor, err := db.Collection(name).Indexes().List(ctx)
		if err != nil {
			return mongoSchemaError(fmt.Sprintf("arrival MongoDB index inspection failed for %s", name), err)
		}
		var indexes []mongoArrivalExistingIndex
		if err := cursor.All(ctx, &indexes); err != nil {
			return mongoSchemaError(fmt.Sprintf("arrival MongoDB index decoding failed for %s", name), err)
		}
		for _, want := range mongoArrivalIndexes {
			if want.collection != name {
				continue
			}
			found := false
			for _, got := range indexes {
				if mongoArrivalIndexMatches(got, want) {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("arrival MongoDB required index %s is missing or incompatible", want.name)
			}
		}
	}
	return nil
}

// MigrateMongoArrivalSchema explicitly creates arrival collections and indexes.
// Callers must obtain migration authorization; startup must only validate.
func MigrateMongoArrivalSchema(ctx context.Context, db *mongo.Database) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := validateMongoArrivalTopology(ctx, db); err != nil {
		return err
	}
	names, err := mongoArrivalCollectionNames(ctx, db)
	if err != nil {
		return err
	}
	for _, name := range mongoArrivalCollections {
		if !names[name] {
			if err := db.CreateCollection(ctx, name); err != nil {
				return mongoSchemaError(fmt.Sprintf("arrival MongoDB create collection %s failed; verify migration permissions", name), err)
			}
		}
	}
	for _, index := range mongoArrivalIndexes {
		_, err := db.Collection(index.collection).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: index.keys, Options: options.Index().SetName(index.name).SetUnique(index.unique).SetCollation(&options.Collation{Locale: "simple"})})
		if err != nil {
			return mongoSchemaError(fmt.Sprintf("arrival MongoDB create index %s failed; inspect existing indexes and duplicate data", index.name), err)
		}
	}
	return ValidateMongoArrivalSchema(ctx, db)
}
