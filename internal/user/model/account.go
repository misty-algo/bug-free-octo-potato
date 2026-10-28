package model

import "time"

// CollectionAccounts 线上 accounts 集合名。
const CollectionAccounts = "accounts"

// AccountDocument 是线上 accounts 集合的文档模型（P20 实名认证读写）。
// channelBindings/protocolVer 仅建模不消费，P21 账号安全细化。
type AccountDocument struct {
	ID              string              `bson:"_id"`
	RealNameAuth    *RealNameAuthDoc    `bson:"realNameAuth,omitempty"`
	ChannelBindings []ChannelBindingDoc `bson:"channelBindings,omitempty"`
	ProtocolVer     string              `bson:"protocolVer,omitempty"`
}

// MongoCollectionName 返回该文档对应的集合名。
func (AccountDocument) MongoCollectionName() string { return CollectionAccounts }

// RealNameAuthDoc 实名认证子文档。新写入只存三件套（Enc/Hash/Mask）；
// name/idNo 明文为线上存量兼容读字段，映射层兜底，不再回写。
type RealNameAuthDoc struct {
	Name          string     `bson:"name,omitempty"`
	IDNo          string     `bson:"idNo,omitempty"`
	NameEnc       string     `bson:"nameEnc,omitempty"`
	NameHash      string     `bson:"nameHash,omitempty"`
	NameMask      string     `bson:"nameMask,omitempty"`
	IDNoEnc       string     `bson:"idNoEnc,omitempty"`
	IDNoHash      string     `bson:"idNoHash,omitempty"`
	IDNoMask      string     `bson:"idNoMask,omitempty"`
	State         string     `bson:"state,omitempty"`
	SubmitCount   int        `bson:"submitCount,omitempty"`
	SubmitDay     string     `bson:"submitDay,omitempty"`
	IdemKeyHashes []string   `bson:"idemKeyHashes,omitempty"`
	AuthAt        *time.Time `bson:"authAt,omitempty"`
}

// ChannelBindingDoc 渠道绑定子文档（契约 2.1 channel_binding，wechat/qq/sms）。
type ChannelBindingDoc struct {
	Channel    string    `bson:"channel"`
	ChannelUID string    `bson:"channelUid"`
	BoundAt    time.Time `bson:"boundAt"`
}
