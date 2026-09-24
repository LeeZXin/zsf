package boolindex

import (
	"errors"
	"fmt"
	"slices"
	"sync"
)

// LayeredIndex 是「全量 + 增量」两层索引的组合快照，对应广告系统的常规做法：
// 全量索引定时（小时级）重建，日常的广告上下线只进增量层，查询时把两层合并。
//
// 全量层是 Indexer，构造后不可变；增量层同样是一个 Indexer，只装「自上次全量以来
// 变化过的规则」，所以它很小、可以秒级重建。两层之间靠一份 ruleID 集合做遮蔽：
//
//   - 某个 ruleID 进了增量层，它就完全以增量层的判定为准，全量层里同 ID 的结果被丢弃。
//     少了这一步，改过定向的广告在「旧配置命中、新配置不命中」时会被错误投放。
//   - 被删除的 ruleID 也登记在增量层（只是它没有任何记录），于是全量层里它的结果同样被丢弃。
//     所以删除不需要单独的墓碑集合，它就是「登记了但没有记录」。
//
// 结果去重后按 ruleID 升序返回。构造后不可变，可多 goroutine 并发查询；
// 更新时用新的快照整体替换指针（atomic.Pointer 或 cacheutil.Locked）。
type LayeredIndex struct {
	base  *Indexer
	delta *Indexer

	// owned 是被增量层接管的 ruleID：更新过的 + 删除的。
	// 全量层的命中结果里落在这个集合上的一律丢弃。
	owned map[uint64]struct{}

	// deleted 是被删除的 ruleID 数，只用于统计
	deleted int
}

// LayeredStats 是两层索引的规模统计。
//
// DeltaRules 与 Deleted 都只反映「相对当前基线」的变化量：
// 换了新基线重新 Build 之后，旧增量自动作废，这两个数归零。
type LayeredStats struct {
	BaseRules  int // 全量层规则数
	DeltaRules int // 增量层规则数（更新过的）
	Deleted    int // 自上次全量以来删除的规则数
}

// NewLayeredIndex 构造一份只有全量层、没有增量的快照。
// 全量重建完成后用它，调用方始终持有同一类型，不必区分「有没有增量」。
func NewLayeredIndex(base *Indexer) *LayeredIndex {
	if base == nil {
		panic("boolindex: nil base index")
	}
	return &LayeredIndex{base: base}
}

// Match 返回两层合并后的命中 ruleID，按升序去重。
func (l *LayeredIndex) Match(p Profile) []uint64 {
	return l.MatchInto(p, nil)
}

// MatchInto 与 Match 相同，但把结果追加到 dst 上，便于复用结果切片。
// 只有本次追加的部分会被排序去重，dst 里原有内容不受影响。
func (l *LayeredIndex) MatchInto(p Profile, dst []uint64) []uint64 {
	start := len(dst)

	if l.delta == nil {
		dst = l.base.MatchInto(p, dst)
	} else {
		// 增量层的结果先落袋：它接管的 ruleID 要从全量结果里剔掉
		dst = l.delta.MatchInto(p, dst)
		mark := len(dst)
		dst = l.base.MatchInto(p, dst)
		dst = filterOwned(dst, mark, l.owned)
	}

	out := dst[start:]
	slices.Sort(out)
	out = slices.Compact(out)
	return dst[:start+len(out)]
}

// Stats 返回两层规模统计。
func (l *LayeredIndex) Stats() LayeredStats {
	s := LayeredStats{BaseRules: l.base.Stats().Rules, Deleted: l.deleted}
	if l.delta != nil {
		s.DeltaRules = l.delta.Stats().Rules
	}
	return s
}

// filterOwned 原地剔除 dst[mark:] 里落在 owned 上的元素。
// 就地压缩，不额外分配；owned 为空时直接返回。
func filterOwned(dst []uint64, mark int, owned map[uint64]struct{}) []uint64 {
	if len(owned) == 0 {
		return dst
	}
	w := mark
	for _, id := range dst[mark:] {
		if _, hit := owned[id]; hit {
			continue
		}
		dst[w] = id
		w++
	}
	return dst[:w]
}

// SkippedRule 是一条没能进增量层的规则。
type SkippedRule struct {
	RuleID uint64
	Err    error
}

// SkippedRulesError 报告增量构建时被跳过的非法规则。
//
// 它出现时增量索引本身仍然可用——被跳过的规则保留全量层里的旧版本。
// 这个取舍是刻意的：一条配错的广告不该让它直接下线，也不该阻塞其它广告的上下线；
// 但调用方必须把它记下来（至少打日志），否则这条广告会静默沿用旧配置。
type SkippedRulesError struct {
	Rules []SkippedRule
}

func (e *SkippedRulesError) Error() string {
	if len(e.Rules) == 0 {
		return "boolindex: no skipped rules"
	}
	// 只列前几条，规则多的时候不要把日志刷爆
	const head = 3
	ids := make([]uint64, 0, head)
	for i, r := range e.Rules {
		if i == head {
			break
		}
		ids = append(ids, r.RuleID)
	}
	msg := fmt.Sprintf("boolindex: %d delta rules skipped, first:%v, first error: %v",
		len(e.Rules), ids, e.Rules[0].Err)
	return msg
}

// DeltaBuilder 累积「自上次全量以来变化过的规则」，并把它们编译成增量层。
//
// 典型用法（全量按小时重建，增量按秒/分钟重建）：
//
//	// 这两个是一对，进程内只能有一份；放包级变量或服务结构体字段都可以
//	var (
//	    cur atomic.Pointer[boolindex.LayeredIndex] // 查询侧读 cur.Load()，无锁
//	    db  = boolindex.NewDeltaBuilder()          // 变更登记 + 增量构建
//	)
//
//	// 启动与每小时：全量重建，把新基线交给 Build，旧增量自动作废
//	layered, err := db.Build(全量建索引())
//	cur.Store(layered)
//
//	// 广告变更到达时（binlog、消息、接口回调）
//	db.Upsert(adID, packs...)
//	db.Delete(adID)
//
//	// 秒/分钟级：沿用当前基线刷新增量层，不必自己存着基线
//	layered, err = db.Refresh()
//	cur.Store(layered)
//
// Build 每次都用累积的全部变更重新编译增量层，所以调用方要控制频率：
// 变更量不大时秒级重建是毫秒级的开销；变更累积到全量的相当比例时，
// 直接重建全量比继续靠增量更省事。
type DeltaBuilder struct {
	mu sync.Mutex

	// lastBase 是上一次 Build 用的基线。基线是换了还是同一份，
	// 决定了累积的增量还算不算数——见 Build 里的说明。
	lastBase *Indexer

	// changed 记录相对 lastBase 变化过的规则：值非 nil 表示更新/新增，
	// 值为 nil 表示删除。Upsert 会拒绝空 packs，所以 nil 不会被误当成更新。
	changed map[uint64][]Targeting
}

// NewDeltaBuilder 创建一个空的变更累积器。
func NewDeltaBuilder() *DeltaBuilder {
	return &DeltaBuilder{changed: make(map[uint64][]Targeting)}
}

// Upsert 登记一条规则的当前版本（新增或更新）。同一个 ruleID 多次调用以最后一次为准。
//
// 这里只拦 ruleID 越界与空 packs；定向包本身是否合法要等 Build 才知道，
// 因为字段的匹配方式（枚举/数值）约束依赖整个增量层的字段状态。
func (b *DeltaBuilder) Upsert(id uint64, packs ...Targeting) error {
	if id > maxRuleID {
		return fmt.Errorf("boolindex: ruleID=%d out of range [0, %d]", id, uint64(maxRuleID))
	}
	if len(packs) == 0 {
		return fmt.Errorf("boolindex: ruleID=%d has no targeting pack", id)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.changed[id] = packs
	return nil
}

// Delete 登记一条规则的删除。它在全量层里的结果会被遮蔽掉。
func (b *DeltaBuilder) Delete(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.changed[id] = nil
}

// Refresh 沿用上一次 Build 用的基线，重新编译增量层并产出新快照。
//
// 定时刷新用这个：秒/分钟级只是增量层变新，基线没动，调用方不必自己存着基线再传一遍。
// 全量重建之后改用 Build(newBase)，它会换基线并作废旧增量。
// 还没有设过基线（一次 Build 都没调过）时返回错误。
func (b *DeltaBuilder) Refresh() (*LayeredIndex, error) {
	b.mu.Lock()
	base := b.lastBase
	b.mu.Unlock()
	if base == nil {
		return nil, errors.New("boolindex: no base yet, call Build with a base first")
	}
	return b.Build(base)
}

// Pending 返回累积的变更数量：待更新/新增的条数与待删除的条数。
func (b *DeltaBuilder) Pending() (upserts, deletes int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, packs := range b.changed {
		if packs == nil {
			deletes++
		} else {
			upserts++
		}
	}
	return
}

// Build 把累积的变更编译成增量层，并与 base 组合出新的查询快照。
//
// 非法规则会被跳过，不会让整批变更失败——一条配错的广告不该阻塞其它广告的上下线，
// 也不该让它自己直接下线，所以它在全量层里的旧版本继续生效。
// 只要有规则被跳过，返回的 error 就是 *SkippedRulesError，同时快照照常可用。
//
// 调用方拿到 err 后应当记录/告警，但不要因为 err 非空就丢掉快照：
//
//	layered, err := db.Build(base)
//	var skipped *boolindex.SkippedRulesError
//	if errors.As(err, &skipped) {
//	    logger.Warn().Any("rules", skipped.Rules).Msg("部分广告配置非法，已跳过")
//	} else if err != nil {
//	    return err
//	}
//	cur.Store(layered) // cur 声明为 atomic.Pointer[LayeredIndex]
func (b *DeltaBuilder) Build(base *Indexer) (*LayeredIndex, error) {
	if base == nil {
		panic("boolindex: nil base index")
	}

	b.mu.Lock()
	// 基线换代 = 全量重建过了。累积的增量是「相对旧基线」的差异，
	// 此刻它已经并进新基线；继续留着只有害处——增量层对全量层有遮蔽权，
	// 会把新基线里已经改对的版本按旧版本遮回去，而且不报任何错。
	// 所以这里不做「让调用方记得 Reset」那套，检测到 base 换了就直接作废。
	if b.lastBase != nil && b.lastBase != base {
		b.changed = make(map[uint64][]Targeting)
	}
	b.lastBase = base

	changed := make(map[uint64][]Targeting, len(b.changed))
	for id, packs := range b.changed {
		changed[id] = packs
	}
	b.mu.Unlock()

	if len(changed) == 0 {
		return NewLayeredIndex(base), nil
	}

	// 按 ruleID 升序建索引：增量层的构建结果与 map 遍历顺序无关，便于复现问题
	ids := make([]uint64, 0, len(changed))
	for id := range changed {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	delta := NewIndexer()
	owned := make(map[uint64]struct{}, len(changed))
	var skipped []SkippedRule
	deleted := 0

	for _, id := range ids {
		packs := changed[id]
		if packs == nil {
			// 删除：登记为「接管但没有记录」，全量层里它的结果会被遮蔽
			owned[id] = struct{}{}
			deleted++
			continue
		}
		if err := delta.AddRule(id, packs...); err != nil {
			// 没进增量层就不遮蔽全量层，让广告继续按旧配置投放
			skipped = append(skipped, SkippedRule{RuleID: id, Err: err})
			continue
		}
		owned[id] = struct{}{}
	}
	if err := delta.Build(); err != nil {
		// AddRule 已经做完校验，Build 目前不会失败
		return NewLayeredIndex(base), err
	}
	if len(owned) == 0 {
		delta = nil
	}

	layered := &LayeredIndex{base: base, delta: delta, owned: owned, deleted: deleted}
	if len(skipped) > 0 {
		return layered, &SkippedRulesError{Rules: skipped}
	}
	return layered, nil
}
