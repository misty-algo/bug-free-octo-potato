package repo

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BwCloudWeGo/bw-cli/internal/user/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/user/model"
	"github.com/BwCloudWeGo/bw-cli/pkg/crypto"
)

// MongoAccountRepository 实名认证落线上 accounts 集合（P20）。
// 新写入只存三件套；存量明文在映射层兼容读，不回写。
type MongoAccountRepository struct {
	db *mongo.Database
}

// NewMongoAccountRepository 创建 accounts 集合仓储。
func NewMongoAccountRepository(db *mongo.Database) *MongoAccountRepository {
	return &MongoAccountRepository{db: db}
}

var _ entity.RealNameAccountRepository = (*MongoAccountRepository)(nil)
var _ entity.AccountSecurityRepository = (*MongoAccountRepository)(nil)

// GetRealNameAuth 读取账号实名子文档；未认证返回 (nil, nil)。
func (r *MongoAccountRepository) GetRealNameAuth(ctx context.Context, accountID string) (*entity.RealNameAuth, error) {
	var doc model.AccountDocument
	err := r.db.Collection(model.CollectionAccounts).FindOne(ctx, bson.M{"_id": accountID}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	if doc.RealNameAuth == nil {
		return nil, nil
	}
	return realNameFromDoc(doc.RealNameAuth), nil
}

// SaveRealNameAuth 覆盖写账号实名子文档（即迁移清除存量明文字段）。
func (r *MongoAccountRepository) SaveRealNameAuth(ctx context.Context, accountID string, auth *entity.RealNameAuth) error {
	_, err := r.db.Collection(model.CollectionAccounts).UpdateOne(ctx,
		bson.M{"_id": accountID},
		bson.M{"$set": bson.M{"realNameAuth": realNameToDoc(auth)}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

// FindValidHolderByIDNo 返回持有同证件的其他有效主账号 ID。
// 新写入比对 idNoHash 字段；存量明文用 idNoPlain 兜底匹配（不落库）。
func (r *MongoAccountRepository) FindValidHolderByIDNo(ctx context.Context, idNoHash, idNoPlain, excludeAccountID string) (string, error) {
	filter := bson.M{
		"_id": bson.M{"$ne": excludeAccountID},
		"$or": []bson.M{
			{"realNameAuth.idNoHash": idNoHash},
			{"realNameAuth.idNo": idNoPlain},
		},
		"realNameAuth.state": bson.M{"$ne": entity.RealNameStateFailed},
	}
	var doc model.AccountDocument
	err := r.db.Collection(model.CollectionAccounts).FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", nil
		}
		return "", err
	}
	return doc.ID, nil
}

// GetAccountSecurity 读取绑定列表/协议版本/实名状态；无记录返回 (nil, nil)。
func (r *MongoAccountRepository) GetAccountSecurity(ctx context.Context, accountID string) (*entity.AccountSecurity, error) {
	var doc model.AccountDocument
	err := r.db.Collection(model.CollectionAccounts).FindOne(ctx, bson.M{"_id": accountID}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	sec := &entity.AccountSecurity{ProtocolVer: doc.ProtocolVer, RealNameState: entity.RealNameStateNone}
	if doc.RealNameAuth != nil {
		sec.RealNameState = realNameFromDoc(doc.RealNameAuth).State
	}
	for _, b := range doc.ChannelBindings {
		sec.Bindings = append(sec.Bindings, entity.ChannelBinding{
			Channel:    b.Channel,
			ChannelUID: b.ChannelUID,
			BoundAt:    b.BoundAt,
		})
	}
	return sec, nil
}

// SaveProtocolVer 记录协议同意版本（同版本幂等由用例层保证）。
func (r *MongoAccountRepository) SaveProtocolVer(ctx context.Context, accountID, protocolVer string) error {
	_, err := r.db.Collection(model.CollectionAccounts).UpdateOne(ctx,
		bson.M{"_id": accountID},
		bson.M{"$set": bson.M{"protocolVer": protocolVer}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

// realNameFromDoc 映射到实体并做存量兼容兜底：
// 明文 name/idNo 只用于补齐 Mask/Hash 字段（内存比对/展示），不外泄到出参。
func realNameFromDoc(doc *model.RealNameAuthDoc) *entity.RealNameAuth {
	auth := &entity.RealNameAuth{
		NameEnc:       doc.NameEnc,
		NameHash:      doc.NameHash,
		NameMask:      doc.NameMask,
		IDNoEnc:       doc.IDNoEnc,
		IDNoHash:      doc.IDNoHash,
		IDNoMask:      doc.IDNoMask,
		State:         doc.State,
		SubmitCount:   doc.SubmitCount,
		SubmitDay:     doc.SubmitDay,
		IdemKeyHashes: doc.IdemKeyHashes,
		AuthAt:        doc.AuthAt,
	}
	if auth.State == "" {
		// 线上存量无 state 字段按「已认证」兼容（D5）。
		auth.State = entity.RealNameStatePassed
	}
	if auth.NameHash == "" && doc.Name != "" {
		auth.NameHash = crypto.SHA256Hex(doc.Name)
	}
	if auth.NameMask == "" && doc.Name != "" {
		auth.NameMask = crypto.MaskName(doc.Name)
	}
	if auth.IDNoHash == "" && doc.IDNo != "" {
		auth.IDNoHash = crypto.SHA256Hex(doc.IDNo)
	}
	if auth.IDNoMask == "" && doc.IDNo != "" {
		auth.IDNoMask = crypto.MaskIDNo(doc.IDNo)
	}
	return auth
}

// realNameToDoc 只写三件套与状态字段，不回写姓名/证件真值。
func realNameToDoc(auth *entity.RealNameAuth) *model.RealNameAuthDoc {
	return &model.RealNameAuthDoc{
		NameEnc:       auth.NameEnc,
		NameHash:      auth.NameHash,
		NameMask:      auth.NameMask,
		IDNoEnc:       auth.IDNoEnc,
		IDNoHash:      auth.IDNoHash,
		IDNoMask:      auth.IDNoMask,
		State:         auth.State,
		SubmitCount:   auth.SubmitCount,
		SubmitDay:     auth.SubmitDay,
		IdemKeyHashes: auth.IdemKeyHashes,
		AuthAt:        auth.AuthAt,
	}
}
