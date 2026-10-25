package model

import "time"

const adminsMongoCollectionName = "admins"

// AdminsDocument 是 admins 集合（管理端账号）的 MongoDB 文档模型。
type AdminsDocument struct {
	ID             string    `bson:"_id"`
	Username       string    `bson:"username"`
	PasswordHash   string    `bson:"password_hash"`
	RealName       string    `bson:"real_name"`
	PhoneEnc       string    `bson:"phone_enc"`
	Email          string    `bson:"email"`
	RoleIDs        []string  `bson:"role_ids"`
	DeptID         string    `bson:"dept_id"`
	DataScope      string    `bson:"data_scope"`
	CityCodes      []string  `bson:"city_codes"`
	PartnerID      string    `bson:"partner_id"`
	Status         string    `bson:"status"`
	LastLoginAt    time.Time `bson:"last_login_at"`
	LastLoginIP    string    `bson:"last_login_ip"`
	LoginFailCount int64     `bson:"login_fail_count"`
	LockedUntil    time.Time `bson:"locked_until"`
	MFAEnabled     bool      `bson:"mfa_enabled"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (AdminsDocument) MongoCollectionName() string {
	return adminsMongoCollectionName
}

const rolesMongoCollectionName = "roles"

// RolesDocument 是 roles 集合（角色）的 MongoDB 文档模型。
type RolesDocument struct {
	ID            string   `bson:"_id"`
	Code          string   `bson:"code"`
	Name          string   `bson:"name"`
	PermissionIDs []string `bson:"permission_ids"`
	DataScope     string   `bson:"data_scope"`
	Status        string   `bson:"status"`
	Remark        string   `bson:"remark"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (RolesDocument) MongoCollectionName() string {
	return rolesMongoCollectionName
}

const permissionsMongoCollectionName = "permissions"

// PermissionsDocument 是 permissions 集合（权限点）的 MongoDB 文档模型。
type PermissionsDocument struct {
	ID       string `bson:"_id"`
	Code     string `bson:"code"`
	Name     string `bson:"name"`
	Module   string `bson:"module"`
	Type     string `bson:"type"`
	ParentID string `bson:"parent_id"`
	Sort     int64  `bson:"sort"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (PermissionsDocument) MongoCollectionName() string {
	return permissionsMongoCollectionName
}

const auditLogsMongoCollectionName = "audit_logs"

// AuditLogsDocument 是 audit_logs 集合（操作日志）的 MongoDB 文档模型。
type AuditLogsDocument struct {
	ID         string                 `bson:"_id"`
	AdminID    string                 `bson:"admin_id"`
	AdminName  string                 `bson:"admin_name"`
	Module     string                 `bson:"module"`
	Action     string                 `bson:"action"`
	TargetType string                 `bson:"target_type"`
	TargetID   string                 `bson:"target_id"`
	Before     map[string]interface{} `bson:"before"`
	After      map[string]interface{} `bson:"after"`
	IP         string                 `bson:"ip"`
	UserAgent  string                 `bson:"user_agent"`
	Result     string                 `bson:"result"`
	FailReason string                 `bson:"fail_reason"`
	OperatedAt time.Time              `bson:"operated_at"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (AuditLogsDocument) MongoCollectionName() string {
	return auditLogsMongoCollectionName
}

const systemConfigsMongoCollectionName = "system_configs"

// SystemConfigsDocument 是 system_configs 集合（系统配置）的 MongoDB 文档模型。
type SystemConfigsDocument struct {
	ID          string `bson:"_id"`
	ConfigKey   string `bson:"config_key"`
	ConfigValue string `bson:"config_value"`
	Group       string `bson:"group"`
	Name        string `bson:"name"`
	Description string `bson:"description"`
	ValueType   string `bson:"value_type"`
	Editable    bool   `bson:"editable"`
	CityCode    string `bson:"city_code"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (SystemConfigsDocument) MongoCollectionName() string {
	return systemConfigsMongoCollectionName
}

const dictsMongoCollectionName = "dicts"

// DictsDocument 是 dicts 集合（数据字典）的 MongoDB 文档模型。
type DictsDocument struct {
	ID     string `bson:"_id"`
	Type   string `bson:"type"`
	Code   string `bson:"code"`
	Label  string `bson:"label"`
	Value  string `bson:"value"`
	Sort   int64  `bson:"sort"`
	Status string `bson:"status"`
	Remark string `bson:"remark"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DictsDocument) MongoCollectionName() string {
	return dictsMongoCollectionName
}

const filesMongoCollectionName = "files"

// FilesDocument 是 files 集合（文件资源）的 MongoDB 文档模型。
type FilesDocument struct {
	ID          string `bson:"_id"`
	FileName    string `bson:"file_name"`
	FileURL     string `bson:"file_url"`
	FileSize    int64  `bson:"file_size"`
	MimeType    string `bson:"mime_type"`
	BizType     string `bson:"biz_type"`
	BizID       string `bson:"biz_id"`
	UploaderID  string `bson:"uploader_id"`
	IsEncrypted bool   `bson:"is_encrypted"`
	StorageType string `bson:"storage_type"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (FilesDocument) MongoCollectionName() string {
	return filesMongoCollectionName
}
