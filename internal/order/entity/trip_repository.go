package entity

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TripRepository persists the D06 trip commands. The deployed MongoDB is
// standalone, so every write is a single-document atomic update guarded by the
// document version; convergence across documents relies on the idempotent
// command record instead of a transaction.
type TripRepository interface {
	TaskByNo(context.Context, string) (*DispatchTask, error)
	TaskByID(context.Context, bson.ObjectID) (*DispatchTask, error)
	OrderByNo(context.Context, string) (*RideOrder, error)
	OrdersByIDs(context.Context, []bson.ObjectID) ([]RideOrder, error)
	FindCommand(context.Context, CommandKey) (*CommandResult, error)

	// PassengerPhoneTail 是受信任身份适配器：只把手机号尾号四位交给行程域，
	// 不返回完整号码，也不让行程域接触号码明文存储位置。
	PassengerPhoneTail(context.Context, bson.ObjectID) (string, error)

	SaveVerify(context.Context, VerifyMutation) error
	SaveStart(context.Context, StartMutation) error
	SaveEnd(context.Context, EndMutation) error

	// SaveSample 以 taskNo+sessionId+seq 唯一去重，返回会话最新水位。
	SaveSample(context.Context, SampleMutation) (TripSession, error)
	Session(context.Context, string, string) (*TripSession, error)
}
