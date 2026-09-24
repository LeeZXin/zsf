# boolindex 定向检索索引

把百万级的「定向规则」常驻内存，给定一份用户画像，一次求出所有命中的规则 ID。

```go
import "github.com/LeeZXin/zsf/indexer/boolindex"
```

包名取自**布尔表达式索引**（Boolean Expression Index，VLDB09 那篇论文的标题）。

这是**反向检索**，和搜索引擎的方向相反：搜索引擎是「一个查询对全部文档求值」，
这里是「每条规则带着自己的条件，对同一个请求求值」。广告定向、规则引擎都属于这一类。

```
规则（广告）           请求（用户画像）
  1001: 北京/上海           city: beijing
        且 25-40 岁   ──▶   age:  30        ──▶  命中的规则 ID: [1001, 1007]
        且 非竞品人群        tag:  beauty
```

## 该不该用这个包

| 场景 | 选择 |
|---|---|
| 规则十万级以上，或查询 QPS 高，条件以枚举 / 数值范围为主 | 用这个包 |
| 规则只有几千条 | 直接 `map` + 集合求交更简单，别引入 |
| 需要全文检索、模糊匹配、打分排序 | 不是这个包，用搜索引擎 |
| 需要「字段之间比较」（`a > b`） | 不支持，业务侧预计算成枚举字段 |

## 快速上手

```go
ix := boolindex.NewIndexer()

// 一条规则可以有多个定向包，包之间是「或」，任一包命中即命中
if err := ix.AddRule(1001,
    boolindex.Targeting{
        "city": boolindex.In("beijing", "shanghai"),
        "age":  boolindex.Between(25, 40),
        "tag":  boolindex.NotIn("competitor"),
    },
    boolindex.Targeting{
        "city": boolindex.In("hangzhou"),
    },
); err != nil {
    return err // 定向配置非法，必须处理：忽略它 = 这条规则静默不生效
}

if err := ix.Build(); err != nil { // 编译：排序倒排链、建数值区间索引
    return err
}

hits := ix.Match(boolindex.Profile{
    "city": {"beijing"},
    "age":  {"30"},
    "tag":  {"beauty"},
})
// hits = [1001]
```

## 硬约束（AI 助手 / 新同事先看这一节）

**必须：**

1. 顺序是 `NewIndexer` → `AddRule`（可反复）→ `Build` → `Match`。
2. 所有错误都在 `AddRule` / `Build` 处理完；`Match` 没有 error 返回值。
3. 规则更新 = **全量重建一份新 `Indexer`，再原子替换指针**；查询侧只读，不要加锁保护 `Match`。
4. 数值字段的画像取值必须是**十进制整数字符串**（`"30"`）；小数自行放大成整数（`"3000"` 表示 30.00）。
5. 只带排除条件的定向包会命中所有人——写这类规则时确认这是本意。
6. 新代码的错误信息用 `"boolindex: ..."` 前缀；测试只用标准库 `testing`。

**禁止：**

1. ❌ 手写 `Condition` 的字段（`Condition{In: ...}`）——它没有导出字段，只能用构造器。
2. ❌ 在 `Build` 之后调 `AddRule`（返回 `ErrBuilt`）；也**不要**为了「增量添加」去改索引内核。
3. ❌ 并发调用 `AddRule` / `Build`（构建期单线程）；只有 `Build` 之后的 `Match` 可并发。
4. ❌ 让 `Match` 之前没有 `Build`（会 panic，这是刻意的 fail-fast）。
5. ❌ 同一个字段上混用枚举条件（`In`/`NotIn`）和数值范围条件（`Between` 等）——`AddRule` 会报错。
6. ❌ 把频繁变化的数据（预算、频控、审核状态、时段）塞进索引——放在召回之后过滤。

需要照抄的代码在「快速上手」「更新规则」「JSON 配置」三节，都是完整可编译的骨架。

## 常见错误对照

| 症状 | 真正的原因 |
|---|---|
| `unknown field In in struct literal` | `Condition` 没有导出字段，改用 `boolindex.In(...)` |
| panic `Match called before Build` | 忘了调 `Build()` |
| `ErrBuilt` | `Build` 之后又 `AddRule`，要新建一份 `Indexer` |
| 数值条件怎么都不命中 | 画像该字段传了非整数字符串（`"30.5"`、`"1e3"`）——**静默不命中，不报错** |
| 命中比预期多 | 定向包只带排除条件（命中所有人），或画像缺了正向条件对应的字段 |
| 命中比预期少 | 画像缺字段时 `In` 判负；确认画像该字段确实有值 |
| 改过配置的广告仍按旧定向投 | 走分层更新时忘了把该 ruleID `Upsert` 进增量层 |
| `json.Unmarshal` 后规则变宽了 | 键名写错（`not_in` vs `notIn`）——解码拒绝未知键，会报错而不是静默丢 |

## 四个类型

| 类型 | 含义 |
|---|---|
| `Field` | 定向字段名，对应画像里的一个维度（地域、兴趣、设备…） |
| `Condition` | 单字段上的条件。**没有导出字段，只能由构造器产生** |
| `Targeting` | 一个定向包：`map[Field]Condition`，字段之间是「且」 |
| `Profile` | 一次请求的画像：`map[Field][]string`，同字段多值之间是「或」 |

条件构造器：

```go
boolindex.In("beijing", "shanghai")     // 取值落在集合内
boolindex.NotIn("competitor")           // 取值落在集合内即判不满足
boolindex.Between(25, 40)               // [25, 40]
boolindex.GreaterThan(500)              // (500, +∞)
boolindex.AtLeast(500)                  // [500, +∞)
boolindex.LessThan(40)                  // (-∞, 40)
boolindex.AtMost(40)                    // (-∞, 40]

boolindex.In("a").WithNotIn("x")             // 追加，返回新条件
boolindex.Between(25, 40).WithMin(30, false) // 收紧下界（求交集，不是替换）
```

回读用 `Enum()` 和 `Range()`：

```go
in, notIn, ok := cond.Enum()   // ok=false 表示不是枚举条件
lo, hi, ok := cond.Range()     // ok=false 表示不是数值范围条件；lo > hi 表示区间为空
```

## 语义（这几条最容易记反）

| 情形 | 判定 |
|---|---|
| `In` 非空，画像**缺**该字段 | **不满足** |
| `In` 非空，画像有值但无交集 | 不满足 |
| `NotIn` 非空，画像**缺**该字段 | **满足**（空交集） |
| `NotIn` 非空，画像有值且在排除集合里 | 不满足 |
| 同一字段同时命中 `In` 与 `NotIn` | 排除优先，判负 |

**只带排除条件的定向包会命中所有人**，除非画像明确带上被排除的取值。
业务上通常等价于「除竞品外全投」，但也可能是漏配了正向条件——上线前用
`Stats().Unconditional` 数一遍这类包有多少个。

同一个字段写多个取值是「或」的关系，数值字段同理（画像多个数值只要有一个落在区间里就算满足）。

## 生命周期与并发

```
NewIndexer → AddRule(可反复调用) → Build → Match(可并发调用)
```

- **所有错误在 `AddRule` / `Build` 暴露**，`Match` 不返回 error，查询路径上没有失败点。
- `Build` 之后索引**不可变**，`AddRule` 返回 `ErrBuilt`；`Match` 可以多 goroutine 并发调用（内部只读，无锁）。
- `Build` 之前调用 `Match` 会 panic——这是「忘了 Build」的编程错误，静默返回空会导致一条都不投。
- `AddRule` 是**两阶段提交**：整个定向包全部校验通过才写入，中途失败不会留下半个包（那会造成静默错投）。

## 更新规则

索引 Build 之后不可变，所以规则变更靠**整体替换快照**，不是原地改。

### 做法一：全量重建 + 原子替换（先从这里开始）

```go
var cur atomic.Pointer[boolindex.Indexer]

// 重建
next := boolindex.NewIndexer()
for _, ad := range allAds {
    if err := next.AddRule(ad.ID, ad.Packs...); err != nil {
        logger.Warn().Err(err).Int64("adId", ad.ID).Msg("广告配置非法，跳过")
        continue
    }
}
if err := next.Build(); err != nil {
    return err // 刷新失败保留旧索引
}

// 原子替换，查询侧 cur.Load() 无锁
cur.Store(next)

hits := cur.Load().Match(profile)
```

**100 万条规则重建约 0.9 秒、索引约 70MB**，重建期间旧索引继续服务，切换是瞬间的。
大多数场景这一条就够了。

### 做法二：全量 + 增量分层（要求秒级生效、不想整体重建）

对应广告系统里「全量定时重建、增量只装变化的单元、检索时两层合并」的常规做法：

```go
// 这两个是一对，进程内只能有一份（放包级变量或服务结构体字段都行）：
//   cur 给查询侧读，db 给变更登记与增量构建，两边各建一个会让一半变更静默不生效
var (
    cur atomic.Pointer[boolindex.LayeredIndex]
    db  = boolindex.NewDeltaBuilder()
)

// 启动与全量重建（小时级）：把新基线交给 Build，旧增量会自动作废
layered, err := db.Build(从库里全量建索引())
var skipped *boolindex.SkippedRulesError
if errors.As(err, &skipped) {
    logger.Warn().Any("rules", skipped.Rules).Msg("部分广告配置非法，已跳过")
} else if err != nil {
    return err
}
cur.Store(layered) // err 非空时快照依然可用，不要丢掉

// 广告变更到达时（binlog / 消息 / 接口回调）
db.Upsert(adID, packs...)
db.Delete(adID)

// 秒/分钟级：沿用当前基线刷新增量层，不必自己存着基线
layered, err = db.Refresh()
cur.Store(layered)

// 查询侧无锁
hits := cur.Load().Match(profile)
```

增量层做两件事：**更新过的规则以增量层为准**（全量层里同 ID 的旧结果被遮蔽），
**删除的规则登记为「接管但没有记录」**（全量层里它的结果同样被遮蔽）。
少了第一条遮蔽，改过定向的广告在「旧配置命中、新配置不命中」的画像上会被**静默错投**。

注意增量层带着**遮蔽权**，所以它只对「当前那一份基线」有效。基线换代（全量重建）之后
旧增量就没意义了——继续留着会用旧版本把新基线里已经改对的规则遮回去。
`Build` 检测到传进来的 base 换了，会**自动作废**累积的增量，不需要（也没有）手动清空这一步。
换来的是：全量重建那条路径只要把新的 base 交给同一个 `db` 即可，没有「忘写一行就投错」的可能。

`db` 必须**进程内唯一、长生命周期**：它累积的是「自上次全量以来变化过的规则」，账期是小时级的，
要跨请求跨 goroutine 活着。变更到达的路径和定时刷新的路径必须看到同一个实例——
各建一个的话，登记在 A 上的变更永远进不了 B 建出来的快照，症状是「部分广告改了不生效」。

实测（100 万条规则，跑 `BenchmarkLayeredMatch` / `BenchmarkDeltaBuild` 可复现）：

| 增量层占比 | 全量重建 | **增量重建** | 索引内存 | 查询 p50 |
|---|---|---|---|---|
| 0%（单层） | 0.9 s | — | 69.3 MB | 353 µs |
| **5%** | 0.9 s | **0.048 s** | 74.5 MB | 409 µs |
| 20% | 0.9 s | **0.228 s** | 87.8 MB | 510 µs |

**增量重建耗时与变更数成正比，没有固定收益**。所以增量层控制在 10% 以内才划算；
涨到 20% 以上建议直接全量重建（省下的时间有限，查询反而慢 25%、内存多 13MB）。

非法规则**不会让整批变更失败**：它被跳过、其余照常生效，并保留全量层里的旧版本继续投放
（一条配错的广告不该让它直接下线，也不该阻塞其它广告的上下线）。
错误通过 `*SkippedRulesError` 上报，**调用方必须记录**，否则这条广告会静默沿用旧配置。

## JSON 配置

`Condition` 没有导出字段，所以提供了 `MarshalJSON` / `UnmarshalJSON`，线格式与构造器一一对应：

```json
{"city": {"in": ["beijing", "shanghai"]}}
{"age":  {"gte": 25, "lte": 40}}
{"arpu": {"gt": 500}}
{"tag":  {"notIn": ["competitor"]}}
```

整份 `Targeting` 可以直接序列化/反序列化（JSON 字段名小驼峰，与仓库约定一致）。

- 无界的那一侧省略；`{"gte": 25}` 等价于 `AtLeast(25)`。
- 解码**拒绝未知键**：把 `notIn` 写成 `not_in` 会当场报错，不会静默丢掉条件的一部分。
- 边界必须是整数，小数、超出 int64、字符串都会被拒绝。

## 性能与容量

下面的数字全部由本包的压测产生（`bench_test.go`），可以自己跑一遍复现：

```bash
go test -run '^$' -bench . -benchtime=200x ./indexer/boolindex/ -benchrules=1000000
```

> **是合成数据集的压测，不是线上实测。** 数据形态：100 万条规则、每条 2~3 个条件、
> 6 个枚举维度 + 1 个数值维度、查询命中率约 0.2%，开发机单核（Apple M5）。
> 字段数、条件数、取值基数、命中率任何一个变了，数字都会明显不同——
> 这些只当量级参考，要拍板请拿自己的数据形态压一遍。

| | 单层索引 |
|---|---|
| 全量重建 | 0.9 s |
| 索引内存 | 69.3 MB（73 字节/条） |
| 查询 avg / p50 / p99 | 920 µs / 353 µs / 5.1 ms |
| 单线程 QPS | 约 1090 |
| 每次查询对象分配 | 0 次 |

硬限制：

| 项 | 上限 |
|---|---|
| `ruleID` | `2^44 - 1` |
| 单规则的定向包数 | 256 |
| 单个定向包的正向字段数 | 255 |
| 数值字段的取值 | 十进制整数（int64 范围），小数请自行放大成整数 |

## 不支持的能力

| 不支持 | 替代做法 |
|---|---|
| 字段之间比较（`a > b`） | 业务侧预计算成一个枚举字段 |
| 文本子串匹配（AC 自动机） | 只有画像侧是「一段自由文本」（搜索词、文章内容、APP 名、URL）时才需要；离散值定向用不上，当前不做 |
| 嵌套逻辑 `(A\|B) and (C\|D)` | 手工展开成 DNF（每个组合一个定向包），注意单规则 256 个包的上限 |
| 「不在某个区间内」 | 改用枚举字段的 `NotIn` |
| 增量增删改单条规则 | 见「更新规则」的两种做法 |
| 排序、打分、截断 | 检索是召回层，排序自己接；`MatchInto` 可以复用结果切片 |
| 索引持久化 | 序列化 `Targeting` 配置（JSON）重建，重建是秒级的 |

## 落地还要自己做两层

这个包只负责**召回**：从画像算出候选规则 ID。完整的投放链路还缺两层，漏了会出现
「检索出来了却投不出去」：

1. **正排**：拿到 ruleID 后取广告详情（标题、素材、出价、预算）。按 ruleID 下标直接寻址的
   切片就够，不需要索引。
2. **投放前过滤**：预算是否耗尽、频控、审核状态、投放时段。这些**每次请求都在变**，
   绝对不能进这个索引（否则每秒都要重建），一律放在召回之后做二次过滤。

## 维护者注意

改这个包之前先看这几条，它们都是踩过的坑：

- **`size` 的语义是「有 `In` 的字段个数」**，不是取值个数、也不是条件条数。它烙进 `entryID`
  高位，归并用它判断「这个包要求几个字段同时满足」，改错会导致漏投或错投。见 `entryid.go`。
- **游标必须按字段分组**（一个字段一个 `fieldCursor`，组内每个取值一条子游标）。
  把「字段 × 取值」铺平成一层，会让同一字段的多个取值被当成多个字段，产生假阳性。见 `posting.go`。
- **`need = max(1, size)` 里的 `1` 不是随手写的**：`size=0` 的定向包靠它才能落在排除记录上
  走排除分支，改成 `need = size` 会让「只带排除条件的包」语义失效。
- `cursor.cur` 是 `list[pos]` 的缓存（性能关键）。**任何改动 `pos` 的代码路径都必须同步 `cur`**，
  `TestCursorCacheInvariant` 会盯着。
- `entryID` 的保留位必须恒为 0——这是「正向记录 + 1 跳过整个定向包」不回绕的前提，
  `TestEntryIDLayout` 会断言。
- 改任何检索逻辑后，**必须跑 `TestDifferential` / `TestLayeredDifferential`**：
  它们拿暴力枚举和「变更后重建的单层索引」当唯一真相来源，是唯一能抓住归并层错误的测试。
