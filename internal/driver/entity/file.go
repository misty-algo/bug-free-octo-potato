package entity

import "time"

// 文件元数据状态机：
//
//	UPLOADING → READY → ATTACHED          正常提交链路
//	READY/失败 → DELETING → DELETED       受控清理链路
//
// 未完成文件不能进入申请；提交与清理共享同一行上的状态抢占。
const (
	FileStateUploading = "UPLOADING"
	FileStateReady     = "READY"
	FileStateAttached  = "ATTACHED"
	FileStateDeleting  = "DELETING"
	FileStateDeleted   = "DELETED"
	FileStateFailed    = "FAILED"
)

// FileRecord 是文件元数据。对象删除由这里的删除任务驱动，
// 外部对象操作永远在数据库事务之外执行。
type FileRecord struct {
	FileID           string     `bson:"fileId"`
	OwnerID          string     `bson:"ownerId"`
	Purpose          string     `bson:"purpose"`
	State            string     `bson:"state"`
	Provider         string     `bson:"provider"`
	Bucket           string     `bson:"bucket"`
	Key              string     `bson:"key"`
	ContentType      string     `bson:"contentType"`
	Size             int64      `bson:"size"`
	Checksum         string     `bson:"checksum"`
	AttachedTo       string     `bson:"attachedTo"`
	DeleteTaskID     string     `bson:"deleteTaskId"`
	FailureReason    string     `bson:"failureReason"`
	CreatedAt        time.Time  `bson:"createdAt"`
	StagingExpiresAt *time.Time `bson:"stagingExpiresAt"`
	DeletedAt        *time.Time `bson:"deletedAt"`
}

// FileDeleteTask 记录外部对象删除任务，允许幂等重试。
type FileDeleteTask struct {
	TaskID    string     `bson:"taskId"`
	FileID    string     `bson:"fileId"`
	Key       string     `bson:"key"`
	Provider  string     `bson:"provider"`
	State     string     `bson:"state"`
	Attempts  int        `bson:"attempts"`
	CreatedAt time.Time  `bson:"createdAt"`
	DoneAt    *time.Time `bson:"doneAt"`
}

const (
	DeleteTaskPending = "PENDING"
	DeleteTaskDone    = "DONE"
)
