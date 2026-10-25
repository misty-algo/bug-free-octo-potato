package repo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
)

// 证据读取跨订单域集合：轨迹在 trip_points，消息与虚拟号内嵌在 orders，
// 由 dispatch_tasks 的任务编号关联。这些集合属订单域，本域只读、不建表。

const (
	colTripPoints = "trip_points"
	colOrders     = "orders"
	colDispatch   = "dispatch_tasks"
)

type MongoSafetyEvidenceReader struct{ db *mongo.Database }

func NewMongoSafetyEvidenceReader(db *mongo.Database) *MongoSafetyEvidenceReader {
	return &MongoSafetyEvidenceReader{db: db}
}

func (r *MongoSafetyEvidenceReader) TracePoints(ctx context.Context, taskNo string, from, to time.Time, limit int) ([]entity.TracePointView, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cursor, err := r.db.Collection(colTripPoints).Find(ctx, bson.M{
		"taskNo":      taskNo,
		"collectedAt": bson.M{"$gte": from, "$lte": to},
	}, options.Find().SetSort(bson.D{{Key: "seq", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := make([]entity.TracePointView, 0, limit)
	for cursor.Next(ctx) {
		var p struct {
			Seq         int64     `bson:"seq"`
			CollectedAt time.Time `bson:"collectedAt"`
			Loc         struct {
				Coordinates []float64 `bson:"coordinates"`
			} `bson:"loc"`
			Accuracy float64 `bson:"accuracy"`
		}
		if err := cursor.Decode(&p); err != nil {
			return out, err
		}
		lng, lat := 0.0, 0.0
		if len(p.Loc.Coordinates) == 2 {
			lng, lat = p.Loc.Coordinates[0], p.Loc.Coordinates[1]
		}
		out = append(out, entity.TracePointView{
			Seq: p.Seq, CollectedAt: p.CollectedAt, Lng: lng, Lat: lat, Accuracy: p.Accuracy,
		})
	}
	return out, cursor.Err()
}

func (r *MongoSafetyEvidenceReader) Messages(ctx context.Context, taskNo string, limit int) ([]entity.MessageView, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var task struct {
		ID bson.ObjectID `bson:"_id"`
	}
	err := r.db.Collection(colDispatch).FindOne(ctx, bson.M{"taskNo": taskNo}).Decode(&task)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cursor, err := r.db.Collection(colOrders).Find(ctx, bson.M{"dispatchId": task.ID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := make([]entity.MessageView, 0, limit)
	for cursor.Next(ctx) {
		var order struct {
			Messages []struct {
				Seq  int64     `bson:"seq"`
				From string    `bson:"from"`
				Text string    `bson:"text"`
				T    time.Time `bson:"t"`
			} `bson:"messages"`
		}
		if err := cursor.Decode(&order); err != nil {
			return out, err
		}
		for _, m := range order.Messages {
			out = append(out, entity.MessageView{Seq: m.Seq, From: m.From, Text: m.Text, At: m.T})
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, cursor.Err()
}

func (r *MongoSafetyEvidenceReader) VirtualCall(ctx context.Context, taskNo string) (*entity.VirtualCallView, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var task struct {
		ID bson.ObjectID `bson:"_id"`
	}
	err := r.db.Collection(colDispatch).FindOne(ctx, bson.M{"taskNo": taskNo}).Decode(&task)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var order struct {
		VirtualCall struct {
			Number    string     `bson:"number"`
			ExpiresAt time.Time  `bson:"expiresAt"`
			RevokedAt *time.Time `bson:"revokedAt"`
			Simulated bool       `bson:"simulated"`
		} `bson:"virtualCall"`
	}
	err = r.db.Collection(colOrders).FindOne(ctx, bson.M{"dispatchId": task.ID, "virtualCall.number": bson.M{"$exists": true}}).Decode(&order)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &entity.VirtualCallView{
		Number: order.VirtualCall.Number, ExpiresAt: order.VirtualCall.ExpiresAt,
		RevokedAt: order.VirtualCall.RevokedAt, Simulated: order.VirtualCall.Simulated,
	}, nil
}

var _ entity.SafetyEvidenceReader = (*MongoSafetyEvidenceReader)(nil)
