package model

// CollectionConfigDictionaries 线上 config_dictionaries 集合名。
const CollectionConfigDictionaries = "config_dictionaries"

// ConfigDictionaryDocument 是线上参数字典/配置文档。
// 字段名按线上实测 dictKey/dictValue/state/version（D2：勿凭契约自创字段名）。
// state：草稿/已发布/停用；已发布才生效，本服务只读（P22）。
// 注意：不映射 _id——线上 _id 是 ObjectID，映射成 string 会导致 mongo-driver
// 解码失败（v2 默认不支持 ObjectID→string），而本服务只读 dictKey/dictValue。
type ConfigDictionaryDocument struct {
	Key     string `bson:"dictKey"`
	Value   string `bson:"dictValue"`
	Version int    `bson:"version"`
	State   string `bson:"state"`
}

// MongoCollectionName 返回该文档对应的集合名。
func (ConfigDictionaryDocument) MongoCollectionName() string { return CollectionConfigDictionaries }
