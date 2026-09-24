// Package internal 是 leaderelection 的内部实现包：定义租约表
// （leader_election_lease）对应的 xorm 模型。
package internal

// LeaseTable 租约表名：每个 bizKey 一行租约记录。
const (
	LeaseTable = "leader_election_lease"
)

// Lease 租约表模型：BizKey 唯一标识一项业务锁（唯一索引，一行一条），
// Leader 记录当前租约持有者标识，Expired 为租约过期时间戳（毫秒，
// 时间基准为 MySQL 的 UNIX_TIMESTAMP(NOW(3))，见 leaderelection.Campaign）。
type Lease struct {
	// Id 主键ID，自增。
	Id      int64  `json:"id" xorm:"pk autoincr bigint comment('id')"`
	BizKey  string `json:"bizKey" xorm:"varchar(32) unique comment('业务key')"`
	Leader  string `json:"leader" xorm:"varchar(64) comment('获取者')"`
	Expired int64  `json:"expired" xorm:"bigint comment('过期时间戳，毫秒级')"`
}

// TableName 返回表名（xorm 模型接口），与 LeaseTable 一致。
func (*Lease) TableName() string {
	return LeaseTable
}
