// 本文件提供数据库操作的通用工具函数，当前包含 Upsert（更新或插入）操作，
// 支持 MySQL 唯一键冲突时的重试逻辑。
package model

import "github.com/LeeZXin/zsf/utils/xormutil"

// Upsert 执行更新或插入操作。
// 优先尝试更新，如果更新未影响任何记录则执行插入；
// 插入时如果发生 MySQL 唯一键冲突，再次尝试更新。
// 参数:
//   - update: 更新操作的函数，返回是否更新成功（bool）和错误
//   - insert: 插入操作的函数，返回错误
//
// 返回值:
//   - bool: 操作是否成功
//   - error: 操作过程中的错误信息
func Upsert(update func() (bool, error), insert func() error) (bool, error) {
	ok, err := update()
	if err != nil {
		return false, err
	}
	if !ok {
		err = insert()
		if err != nil {
			// 插入时如果发生MySQL唯一键冲突（duplicate entry），
			// 说明并发条件下其他协程已插入成功，再次尝试更新
			if xormutil.IsMysqlDuplicatedEntryError(err) {
				return update()
			}
			return false, err
		}
	}
	return true, nil
}
