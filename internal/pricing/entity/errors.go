package entity

import "errors"

// ErrRuleSnapshotNotFound 表示订单缺少下单时锁定的计价规则快照，因而无法正式计价。
var ErrRuleSnapshotNotFound = errors.New("pricing rule snapshot not found")
