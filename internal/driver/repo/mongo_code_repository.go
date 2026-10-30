package repo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	dbmodel "github.com/BwCloudWeGo/bw-cli/internal/driver/model"
)

// codeRepo 司机登录验证码仓储：一个号码一条记录，按 _id=手机号 覆盖写。
type codeRepo struct {
	db  *mongo.Database
	log *zap.Logger
}

func (r *codeRepo) coll() *mongo.Collection { return r.db.Collection(dbmodel.CodeCollectionName) }

func (r *codeRepo) Save(ctx context.Context, code *entity.VerificationCode) error {
	if code == nil {
		return entity.ErrInvalidCode
	}
	doc := dbmodel.VerificationCodeDocument{
		ID:        code.Phone,
		Phone:     code.Phone,
		Code:      code.Code,
		SentAt:    code.SentAt,
		ExpiresAt: code.ExpiresAt,
		Used:      code.Used,
		FailCount: code.FailCount,
	}
	_, err := r.coll().ReplaceOne(ctx, bson.M{"_id": doc.ID}, doc, options.Replace().SetUpsert(true))
	logRepoOp(r.log, "code.Save", code.Phone, err)
	return err
}

func (r *codeRepo) Load(ctx context.Context, phone string) (*entity.VerificationCode, error) {
	normalized, err := entity.NormalizePhone(phone)
	if err != nil {
		return nil, err
	}
	var doc dbmodel.VerificationCodeDocument
	err = r.coll().FindOne(ctx, bson.M{"_id": normalized}).Decode(&doc)
	if isNoDocuments(err) {
		return nil, nil
	}
	if err != nil {
		logRepoOp(r.log, "code.Load", normalized, err)
		return nil, err
	}
	return &entity.VerificationCode{
		Phone:     doc.Phone,
		Code:      doc.Code,
		SentAt:    doc.SentAt,
		ExpiresAt: doc.ExpiresAt,
		Used:      doc.Used,
		FailCount: doc.FailCount,
	}, nil
}
