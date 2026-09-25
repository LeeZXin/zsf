# zsf 微服务开发规范（给 AI 的提示词）

> **用法**：用 zsf 开发或维护服务时，把本文件全文作为上下文（或放进项目 `CLAUDE.md`）。
> **范围**：只写 zsf 框架自身能力与基于它的推荐约定。错误码取值、登录态实现、缓存内容、业务模块由各服务自建，本规范只给模式。
> **基线**：`github.com/LeeZXin/zsf`，Go 1.27；HTTP 用 Gin，ORM 用 xorm，日志用 zerolog。

---

## 0. 总则

1. **组装式框架**：zsf 提供 lifecycle / config / logger / http / xormdb / 工具集，**不接管业务结构**。
2. **fail-fast**：基础设施缺失（应用名为空、配置拉不到、密钥解析失败、端口非法、证书缺失）一律 `logger.Logger.Fatal()`；框架**不捕获、不回滚、不重试**。`OnApplicationStart` 里失败必须自己 Fatal。
3. **HTTP 状态码恒 200**，成败看响应体 `code`（0 成功）。只有绑定失败 400/413、鉴权 401/403、限流熔断 429/503、非业务错误 500 才非 200。
4. **分清强制与推荐**。强制（不照做跑不起来/行为不对）：`resources/` 配置布局、`lifecycle.Object` 四方法、路由入口必须 `func(*gin.Engine)`、响应体 `{code,message,data}`、错误用 `*bizerr.Err`。推荐（框架不检查）：目录分层、仓储四件套、DTO 校验风格、注释规范。
5. **`start.AddInit` 与 `quit.Add*ShutdownHook` 都绑在 `lifecycle.Run` 上**：`start.GetInit()` 与 `quit.Wait()` 的唯一调用方就是它。不调 `Run` → 钩子一个都不执行、`quit.Stopping()` 永不关闭、不响应 SIGINT/SIGTERM，**连框架自身的初始化都起不来**（env、static 配置、logger、jwt、xorm 引擎、dynamic、cron、gopool 全是 `AddInit` 钩子）。
6. **可选能力按需接入**（§11），默认不进 main.go。

---

## 1. 工程骨架

### 1.1 依赖

```bash
go get github.com/LeeZXin/zsf@zsf/v0.1.0
go work use ../zsf    # 与 zsf 源码协同开发时
```

### 1.2 推荐目录

```
<service>/
├── main.go                      # 注册钩子 + 组装对象 + lifecycle.Run
├── resources/                   # 静态配置（强制目录名：static 按它找配置）
│   ├── application.yaml         # 所有环境共用
│   ├── cluster-{cluster}.yaml   # 可选，按 SF_CLUSTER 覆盖
│   └── application-{env}.yaml   # 按 SF_ENV 覆盖，优先级最高
├── base/                        # 服务内公共库：errcode、共享仓储、共享缓存、公共枚举
└── internal/<module>/
    ├── api/<resource>api/       # api.go（路由 + handler）；internal/{doc,dto,service}.go
    └── internal/
        ├── core/<能力>/          # 不感知仓储的纯能力：协议、算法、状态机
        └── repository/<资源>repo/ # 仓储四件套
```

`internal/` 挡住跨模块乱引用；handler 与业务实现分居两包，handler 想写厚都难；`core/` 放"不认识数据库"的能力，好复用也好测。

### 1.3 main.go

```go
func main() {
	start.AddInit(promhelper.EnablePushTask) // 需要指标推送时才加
	lifecycle.Run(lifecycle.WithObjectsList(httpServer, workers))
}

// 装配集中在这一处
func httpServer() []lifecycle.Object {
	return []lifecycle.Object{server.NewDefaultServer(/* … */)}
}
```

- `WithObjectsList(builders ...func() []Object)` / `WithObjects(builders ...func() Object)`：**覆盖语义**（多次调用只有最后一次生效）。**装配必须集中在 main 一处**，禁止各包 `init` 各自追加——否则"进程由哪些对象组成"散落各处。
- 两者的 builder **都在 `Run` 内、`start` 钩子之后被调用**（配置已就绪），没有"惰性/立即"之分；区别只是签名（一组 vs 单个）与调用次序（`WithObjects` 先）。
- 对象构造完统一按 `Order()` 升序启动，注册顺序只影响构造顺序。
- `WithPidFile(path)`：写 PID 文件，供外部编排探活。
- `Run` 序列：写 PID → 执行 start 钩子 → 构造对象 → 按 `Order()` 升序 `OnApplicationStart` → 注册逆序关闭钩子 → `AfterInitialize` → 阻塞 `quit.Wait()`。**必须在 main goroutine 调用**。

### 1.4 启动钩子 `start.AddInit`

```go
func init() {
	start.AddInit(func() { database.Sync(new(Model)) }, 4)
}
```

- `AddInit(fn func(), order ...int)`：order 越小越先；同 order 按注册顺序稳定保序；省略 = 10000（排在所有显式 order 之后）。
- **order 是控制模块间加载顺序的唯一手段，有顺序要求就必须显式写**：跨包 `init()` 顺序由 import 图决定（隐式，重构 import 就可能变），"我先注册所以先跑"是错觉。有依赖就给不同值：

  ```go
  start.AddInit(loadSecrets, 3) // 先：依赖 static 配置
  start.AddInit(syncTables, 4)  // 后：依赖 secrets
  start.AddInit(warmCache, 5)   // 再后：依赖表已建好
  ```

- 框架钩子全用**负数**，业务用正数，保证框架初始化先于业务：

| order | 归属 | 内容 |
| --- | --- | --- |
| -10 / -9 / -8 | zsf | env / `config/static` 加载 yaml / logger |
| -7 / -6 / -5 | zsf | cronutil+gopool / jwt / grpc client |
| -4 / -3 | zsf | xormdb 建引擎 / `config/dynamic` |
| 正数 | 业务 | 配置自检、建表靠前（3~4）；缓存预热、事件订阅随后 |
| 0 | 框架 | `http/server`、`grpc/server` 的 `Object.Order()` |
| <0 | 业务 | 需抢在监听前完成的自定义对象 |
| 100 | 业务 | 后台 worker（排在 HTTP 服务之后） |

- **读配置的逻辑必须包进 `AddInit`**，不能裸写在 `init()` 里（执行顺序不保证在 config 加载之后，读到的是零值）。
- 钩子失败请自行 Fatal。

### 1.5 关闭：`quit` 四级钩子

```
应用（Application）：停 cron 新任务、各 Object.OnApplicationShutdown
高优先级（High）：gopool.Close、chunktask.Sync、关 gRPC/NATS 客户端
低优先级（Low）：数据库连接池、选主租约
最终（Final）：日志/指标冲刷、远端 client 关闭
```

- `quit.Stopping() <-chan struct{}` 收到信号后**立即**关闭，早于任何钩子：**所有后台循环必须 select 它（或自己的 ctx）**，否则进程退不掉。
- 资源按性质挂 `AddApplicationShutdownHook` / `AddHighPriorityShutdownHook` / `AddLowPriorityShutdownHook` / `AddFinalShutdownHook`；同级串行不并发，单个钩子自控超时、出错记日志继续。
- 信号监听与 `Stopping()` 的关闭都发生在 `quit.Wait()` 里 → 同样绑在 `lifecycle.Run` 上（见 §0.5）。

---

## 2. 一条请求的完整写法（推荐结构，照抄再改）

以"订单"资源为例。

### 2.1 `internal/order/api/orderapi/api.go` —— 路由 + 极薄 handler

```go
// Package orderapi 提供订单相关的 HTTP API：列表、详情、创建、删除。
package orderapi

import (
	"<service>/internal/order/api/orderapi/internal"

	"github.com/LeeZXin/zsf/http/ginutil"
	"github.com/LeeZXin/zsf/insession"
	"github.com/gin-gonic/gin"
)

// Api 注册订单模块路由。
//
//   - GET    /api/order        — 分页列表
//   - GET    /api/order/detail — 详情
//   - POST   /api/order        — 创建
//   - DELETE /api/order        — 删除（超管）
func Api(e *gin.Engine) {
	group := e.Group("/api/order", insession.AuthFilter())
	{
		group.GET("", list)
		group.GET("/detail", detail)
		group.POST("", create)
		group.DELETE("", insession.IsSuperFilter, deleteOrder)
	}
}

func list(c *gin.Context) {
	var req internal.ListReq
	if ginutil.ShouldBindQuery(&req, c) {
		ret, total, err := internal.List(c, req)
		if err != nil {
			ginutil.Error(err, c)
			return
		}
		ginutil.Page2Success(ret, total, c)
	}
}

func create(c *gin.Context) {
	var req internal.SaveReq
	if ginutil.ShouldBindJSON(&req, c) {
		if err := internal.Create(c, req, insession.GetOperator(c)); err != nil {
			ginutil.Error(err, c)
			return
		}
		ginutil.DefaultSuccess(c)
	}
}
```

- 入口固定 `func Api(e *gin.Engine)`（隐式满足 `gin.OptionFunc`，直接传给 `server.RegisterRouter`）；**没有** `var Api = &router.Api{...}` 这种写法。一个 api 包可导出多个入口（`Api` + `WS` / `JobApi` / `McpApi`…），按端口/信任域分开挂。
- 路径统一 `/api/<小驼峰资源名>`；列表 `""`、详情 `/detail`；动作用 POST/PUT + 子路径。**`Api` 上方必须用注释列出全部端点**。
- 鉴权用中间件表达：网关注入头模式挂 `insession.AuthFilter()`（登录）与 `insession.IsSuperFilter`（超管）；免鉴权路由不挂 filter 并在注释写明原因。
- handler 固定三步，**不写业务逻辑、不写 else**：绑定（失败自动 400/413 短路）→ 调 `internal.XXX` → `ginutil` 输出。`c` 直接当 `context.Context` 传给 service。

### 2.2 `internal/dto.go` —— 请求/响应 + 声明式校验

```go
package internal

import (
	"<service>/base/errcode"

	"github.com/LeeZXin/zsf/http/bizerr"
	"github.com/LeeZXin/zsf/http/ginutil"
	"github.com/LeeZXin/zsf/utils/dto"
	"github.com/LeeZXin/zsf/utils/validateutil"
)

// ListReq 订单分页查询。
type ListReq struct {
	Status string `json:"status"` // 可选，按状态筛选
	ginutil.Page2Req
}

func (r *ListReq) Validate() *bizerr.Err {
	if !validateutil.AllowEmptyStrLenLe32(&r.Status) || !r.Page2Req.IsValid() {
		return errcode.CodeInvalidParameter
	}
	return nil
}

// SaveReq 创建/更新订单。
type SaveReq struct {
	OrderId string `json:"orderId"` // 更新时必填，创建时服务端生成
	Title   string `json:"title"`
	Amount  int64  `json:"amount"` // 单位：分
}

func (r *SaveReq) Validate() *bizerr.Err {
	rules := validateutil.Rules{
		{Field: &r.Title, Validate: validateutil.NotEmptyStrLenLe128},
	}
	if !rules.Do() || r.Amount <= 0 {
		return errcode.CodeInvalidParameter
	}
	return nil
}

// Order 订单响应结构。
type Order struct {
	OrderId string `json:"orderId"`
	Title   string `json:"title"`
	dto.AuditFields // 时间已格式化为 "2006-01-02 15:04:05" 字符串
}

// md2Order 仓储模型 → 响应 DTO。
func md2Order(m orderrepo.Model) Order {
	return Order{OrderId: m.OrderId, Title: m.Title, AuditFields: dto.NewAuditFields(m.AuditFields)}
}
```

- **命名**：请求 `XxxReq`（`ListReq` / `SaveReq` / `IdReq`），响应直接用名词，列表项 `XxxItem`；同一 `SaveReq` 服务两种场景时给 `ValidateCreate()` / `ValidateUpdate()`。
- **只用 `json` tag**：`ShouldBindQuery` / `ShouldBindMultipartForm` 内部按 **json tag** 映射（query 参数名也取自 json tag），写 `form` tag 会绑不上；不要用 `binding` / `validate` tag。
- **校验**实现 `Validate() *bizerr.Err`，用 `validateutil.Rules{{Field: &r.X, Validate: ...}}.Do()`；长度档位固定 32/64/128/256/512/1024/2048/65535（`NotEmptyStrLenLeNN` / `AllowEmptyStrLenLeNN` / `AllowEmptyStrLen(n)`），另有 `IsIpV4` / `IsEmail` / `IsUrl` / `IsUUID` / `SlicesContains`。**这些函数原地 TrimSpace，必须传指针。**
- **分页**请求嵌 `ginutil.Page2Req`（`pageNum` 从 1 起、`pageSize` ≤ 100），校验必须调 `IsValid()`；大数据集可用 `ginutil.CursorReq`。
- **时间字段**用 `zsf/utils/dto` 的 `AuditFields` / `TimeFields` / `OperatorFields`（响应侧）与 `zsf/utils/model` 同名字段（模型侧），不要自己写。
- 映射函数固定 `md2Xxx`，批量用 `listutil.MapNe(list, md2Order)`。

### 2.3 `internal/service.go` —— 业务逻辑（函数式，不定义结构体）

```go
// List 分页查询订单。
func List(ctx context.Context, req ListReq) ([]Order, int64, *bizerr.Err) {
	if berr := req.Validate(); berr != nil {
		return nil, 0, berr
	}
	ctx, closer := database.Context(ctx)
	defer closer.Close()
	list, total, err := orderrepo.Repository(ctx).List(orderrepo.ListReq{
		PageNum: req.PageNum, PageSize: req.PageSize, Status: req.Status,
	})
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("查询订单列表失败")
		return nil, 0, errcode.CodeInternalError
	}
	if len(list) == 0 {
		return []Order{}, total, nil // 空列表返回 []T{}，不要 nil
	}
	return listutil.MapNe(list, md2Order), total, nil
}

// Create 创建订单。
func Create(ctx context.Context, req SaveReq, operator insession.Operator) *bizerr.Err {
	if berr := req.Validate(); berr != nil {
		return berr
	}
	ctx, closer := database.Context(ctx)
	defer closer.Close()
	repo := orderrepo.Repository(ctx)
	if b, err := repo.ExistByTitle(req.Title); err != nil {
		logger.Ctx(ctx).Err(err).Msg("校验订单标题是否重复失败")
		return errcode.CodeInternalError
	} else if b {
		return bizerr.NewBizErr(errcode.CodeDuplicateError.Code, "订单标题已存在")
	}
	if err := repo.Insert(orderrepo.SaveReq{
		OrderId: idutil.RandomUUID(), // 业务 ID 在业务层生成
		Title:   req.Title,
		Amount:  req.Amount,
		Creator: operator.Account,
	}); err != nil {
		logger.Ctx(ctx).Err(err).Msg("创建订单失败")
		return errcode.CodeInternalError
	}
	return nil
}
```

- **函数式**，无 service 结构体。签名固定：

  ```go
  func List(ctx context.Context, req ListReq) ([]T, int64, *bizerr.Err)
  func Detail(ctx context.Context, req IdReq) (T, *bizerr.Err)
  func Create(ctx context.Context, req SaveReq, operator insession.Operator) (T, *bizerr.Err)
  func Update(ctx context.Context, req SaveReq, operator insession.Operator) *bizerr.Err
  func Delete(ctx context.Context, req IdReq) *bizerr.Err
  ```

  错误一律 `*bizerr.Err`；写操作才追加 `operator`；流式响应（SSE/WebSocket）才直接收 `*gin.Context`。
- 第二段固定 `ctx, closer := database.Context(ctx); defer closer.Close()`，**不要跨慢调用持有会话**。
- **业务 ID 在业务层生成，仓储层绝不生成**：`idutil.RandomUUID()` / `idutil.GenPlusSnowflakeId()`。
- **事务**（跨多表写）用 `database.WithTx`，事务内的仓储必须用 `txCtx` 构造：

  ```go
  err = database.WithTx(ctx, func(txCtx context.Context) error {
      if err := orderrepo.Repository(txCtx).Insert(req); err != nil {
          return err
      }
      return orderrepo.Repository(txCtx).InsertLog(logReq)
  })
  ```

- "记录不存在"统一 `CodeNotFoundError`；**写操作必须先做存在性检查，不允许静默成功**。

### 2.4 注册进服务

把模块的 `Api` 加进 `server.RegisterRouter(...)` 一处即可（见 §1.3 的装配）。

---

## 3. HTTP 层：`zsf/http/server` + `zsf/http/ginutil`

### 3.1 `NewDefaultServer` 的 Option

默认内置 panic recovery（记真实 panic 并返回 500），不用自己加。

| Option | 说明 |
| --- | --- |
| `WithName(name)` | 日志标识，建议必给 |
| `WithHttpPort(port)` / `WithHost(host)` | 端口（不传回退 `http.port`，都无效 Fatal）；只绑内网/本机时给 host |
| `AddFilters(...gin.HandlerFunc)` / `RegisterRouter(...gin.OptionFunc)` | 中间件；路由注册函数（各模块的 `Api` 就是 `func(*gin.Engine)`） |
| `WithMaxBodySize(size)` / `WithReadTimeout` / `WithWriteTimeout` / `WithIdleTimeout` | 请求体上限（超限 413）、各类超时 |
| `DisableGzip()` / `WithGzipExcludeRegex(patterns...)` | **二进制协议路径必须排除**（被 gzip 后客户端解析不了） |
| `EnableHttps(cert, key)` / `WithGetCertificate(fn)` | 证书路径按 `resources/` 解析，写相对路径；动态证书与前者互斥 |
| `EnableActuator()` / `EnablePromApi()` / `EnablePProf()` | health+gc / `/metrics` / `/api/debug/pprof/*` |
| `WithNewRegistrarFunc(f)` / `WithNoRoute` / `WithNoMethod` / `WithHtmlGlob` / `EnableStatic()` | 注册中心（默认不注册）、404/405、模板、静态目录 |

`OnApplicationShutdown` 顺序：置 draining（health 立刻 503）→ 注销注册中心 → 等 5s 摘流 → `Shutdown`（超时取 `http.shutdown-timeout`，默认 30s）。

### 3.2 内置 filter

```go
server.TraceIdFilter      // 必须最外层：沿用上游 X-Trace-Id，缺失才生成；注入 ctx 与 rpc.Header
server.PrometheusFilter   // 用路由模板 FullPath 做标签（未匹配为 unmatched），避免维度爆炸
server.VueFilter(embed, prefix) / VueFilterWithNextCondition(...)     // 前端 SPA 回退
server.FlowLimitFilter(resource) / CircuitBreakerFilter(resource) / SentinelFilter(opts...)
```

### 3.3 响应与绑定

```go
ginutil.DefaultSuccess(c)             // {"code":0}
ginutil.DataSuccess(data, c)          // {"code":0,"data":{...}}
ginutil.Page2Success(list, total, c)  // {"code":0,"data":{"data":[],"total":N}}
ginutil.Error(bizErr, c)              // {"code":10002,"message":"…"}，HTTP 200
ginutil.HandleErr(err, c)             // 业务错→上面；非业务错→500
```

- **`c *gin.Context` 永远在参数最后**（`DataSuccess(data, c)`），极易写反。
- 字段是 `code` / `message` / `data`（**不是 `msg`**）；分页响应 `ginutil.Page2Resp[T]{data,pageNum,total}` / `CursorResp[T]`。
- 绑定：`ShouldBind`（按 Content-Type 自动）/ `ShouldBindJSON` / `ShouldBindQuery` / `ShouldBindMultipartForm`，失败自动 400（超大 413），**不要自己写错误响应**。
- **例外**：对接外部协议不要套这层包装（OIDC 返回 RFC 6749/6750 原形、OpenAI/Anthropic 兼容端点返回上游协议原形），直接 `c.JSON` / `c.Data`。

---

## 4. 错误处理

### 4.1 载体 `zsf/http/bizerr`

```go
type Err struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	Internal bool   `json:"internal"` // 置位后计入 Sentinel 熔断错误率
}
func NewBizErr(code int, format string, args ...any) *Err // 唯一构造器
func IsBizErr(err error) bool
```

### 4.2 错误码：各服务自建 `base/errcode`

zsf 只定义载体，**码值由各服务自行规划号段**。推荐基线：

```go
var (
	CodeInternalError    = &bizerr.Err{Code: 10001, Message: "Internal Error"}
	CodeInvalidParameter = &bizerr.Err{Code: 10002, Message: "Invalid Parameter"}
	CodeDuplicateError   = &bizerr.Err{Code: 10003, Message: "Duplicate elements."}
	CodeOperationFailed  = &bizerr.Err{Code: 10004, Message: "Operation Failed"}
	CodeNotFoundError    = &bizerr.Err{Code: 10005, Message: "Not Found Error"}
)
```

**三段式错误处理**：

1. 参数校验失败 → 直接 `return req.Validate()`。
2. 底层 error → `logger.Ctx(ctx).Err(err).Msg("<中文动作>失败")` + `return errcode.CodeInternalError`，**绝不把内部细节透给前端**。
3. 业务语义失败 → 复用码 + 自定义中文消息：`bizerr.NewBizErr(errcode.CodeDuplicateError.Code, "订单标题已存在")`。

**禁止散写裸数字码**（`&bizerr.Err{Code: 10003, ...}`），码值集中在 `base/errcode`。

---

## 5. 数据访问：`zsf/xormdb`

### 5.1 会话与事务

```go
ctx, closer := database.Context(ctx) // 用后必 Close
defer closer.Close()

database.WithTx(ctx, func(txCtx context.Context) error { ... })
database.GetSession(ctx) / NewSession(ctx) / MustGetSession(ctx) // 后者不在会话内直接 panic
```

- 引擎在 `init`（order -4）按配置自建，**不要自己建引擎**，退出时框架关连接池。
- 配置键：`xorm.driver`（`mysql` / `sqlite3`）、`xorm.datasource`、`xorm.show-sql`、`xorm.slow-sql-duration`；sqlite3 未配 datasource 时默认 `data/sqlite3.db`（相对 `static.Dir()`）。
- 会话经 context 传递；`WithTx` 嵌套时内层自动并入外层事务。**同一 session 不能多 goroutine 并发使用**（xorm 限制）。

### 5.2 建表：无 migration，用 `Sync`

```go
func init() {
	start.AddInit(func() { database.Sync(new(Model)) }, 4)
}
```

- **只在 SIT / debug 环境或 sqlite3 驱动下生效，生产 MySQL 是空操作** → 生产改表要人工 DDL，**动生产表结构前先提醒使用者**。
- 废弃表用 `database.DropTables(names...)`（同样环境门控）。

### 5.3 仓储四件套（`internal/<module>/internal/repository/<资源>repo/`）

```go
// interface.go —— 只放接口 + 工厂
type IRepository interface {
	Insert(SaveReq) error
	Update(SaveReq) error
	GetByOrderId(string, []string) (Model, bool, error)
	List(ListReq) ([]Model, int64, error)
	ExistByTitle(string) (bool, error)
	DeleteByOrderId(string) (bool, error)
}
func Repository(ctx context.Context) IRepository { return &repository{ctx: ctx} }

// model.go —— 表结构
const TableName = "order"

type Model struct {
	Id                int64  `json:"id" xorm:"pk autoincr bigint comment('id')"`
	OrderId           string `json:"orderId" xorm:"varchar(64) unique notnull comment('订单id')"`
	Title             string `json:"title" xorm:"varchar(128) comment('标题')"`
	model.AuditFields `xorm:"extends"`
}

func (*Model) TableName() string    { return TableName }
func (*Model) TableComment() string { return "订单" }

// dto.go —— 入参，不带 json tag（纯内部传输对象）
type SaveReq struct {
	OrderId string
	Title   string
	Operator string
}
type ListReq struct {
	model.PageFields // PageNum / PageSize / Cols
	Status           string
}

// impl.go —— 首行注册建表，其余全是 xorm 链
func init() { start.AddInit(func() { database.Sync(new(Model)) }, 4) }

type repository struct{ ctx context.Context }

func (r *repository) List(req ListReq) ([]Model, int64, error) {
	session := database.GetSession(r.ctx).
		Limit(req.PageSize, (req.PageNum-1)*req.PageSize).
		Cols(req.Cols...).
		Desc("updated")
	if req.Status != "" {
		session.And("status = ?", req.Status)
	}
	ret := make([]Model, 0)
	total, err := session.FindAndCount(&ret)
	return ret, total, err
}

func (r *repository) Update(req SaveReq) error {
	_, err := database.GetSession(r.ctx).
		Where("order_id = ?", req.OrderId).
		Cols("title", "updater"). // 白名单列，避免误清零值字段
		Update(&Model{Title: req.Title, Updater: req.Operator})
	return err
}
```

- 五件套职责不混：`interface.go`（接口 + 工厂）/ `impl.go`（实现，首行 `Sync`）/ `model.go` / `dto.go`（入参，无 tag）/ `doc.go`（可选，写清读写边界）。
- 方法签名：`Insert/Update/Delete` 返 `error`；`Get` 返 `(Model, bool, error)`；`List` 返 `([]Model, int64, error)`；判存在用 `Exist`。**仓储只返回裸 error，不返回 `*bizerr.Err`，不含业务判断。**
- 用 xorm 链式 API；**JOIN 用 `Join(...)` + `Select` + 匿名 struct 标签**，不拼裸 SQL（`Exec` 只用于原子守卫/DDL）。跨 repo 引用表名用导出的 `TableName` 常量。
- 分页统一 `Limit(pageSize, (pageNum-1)*pageSize)` + `FindAndCount`；列表支持 `Cols(req.Cols...)` 裁剪与 `Desc("updated")` 排序。**更新必须用 `Cols(...)` 白名单。**
- 每个字段都要 xorm tag 且**必须带 `comment('中文')`**，行尾写中文注释；索引写成 `index(idx_表_列)` / `unique(uniq_表_列)`；表名加业务前缀避免撞名。

---

## 6. 分层与模块边界（推荐）

- `base/`（服务内公共库）：错误码、多模块共享的仓储、共享缓存、公共枚举。所有模块可 import。（审计/时间/分页字段不用自己写：模型侧 `zsf/utils/model`、响应侧 `zsf/utils/dto`。）
- `api/<resource>api/internal/` 可 import 本模块的 `internal/repository/*`、`internal/core/*`。
- **跨模块**只允许 (a) 走 `base/`；(b) import 对方 **api 包导出的转发函数**（如 `internal/<对方模块>/api/<资源>api/lookup.go` 里包一层转发），**禁止直接 import 别人的 `internal/repository`**。
- `internal/core/<能力>/` 默认**不感知仓储层**；确需读本模块 repository（数据面/缓存装配）必须在 `doc.go` 写明。领域枚举统一放 `core/constants`，**禁止散写字面量**。
- 需要通知其他模块数据变更时用**事件**（§7.5）而不是直接调对方写路径。

---

## 7. 横切能力

### 7.1 配置

**静态**（启动后不可变）：

```go
import "github.com/LeeZXin/zsf/config/static"

static.GetInt("http.port") / GetString("jwt.secret") / GetBool(...) / GetDuration(...)
static.Dir() // resources/ 所在目录；数据文件、仓库目录等相对路径都基于它解析
```

- 优先级 `application-{env}.yaml` > `cluster-{cluster}.yaml` > `application.yaml`；key 缺失静默返回零值。读取 API：`GetString/GetInt/GetInt64/GetBool/GetFloat64/GetDuration/GetIntSlice/GetStringSlice/GetStringMap/GetStringMapString/GetMapSlice/Get/Exists`。
- **键名全小写点号分层，多词段用 `-`**（`prom.push.url`、`xorm.slow-sql-duration`）。
- 环境变量：`SF_ENV`（默认 sit，prd 必须显式设置）、`SF_CLUSTER`、`APPLICATION_NAME`（优先级高于 yaml 的 `application.name`，**两者都为空直接 Fatal**，容器部署最常踩）。
- 必需的 key 在启动钩子里校验，为空即 Fatal。

**动态**（Nacos 热更新，按需）：

```go
value := dynamic.Register[string]("dataId", dynamic.DefaultDecoder, func(v string) { /* 变更回调 */ })
v := value.Get() // 并发安全
```

- 共用 namespace / group（`nacos.namespace` / `nacos.server-addr` / `nacos.group`）；`Register` 同步拉取，拉不到 Fatal（配置必须先建好、Nacos 必须可用）；回调在 SDK 监听 goroutine 内同步执行，**不要做耗时操作**。
- **不需要热更新的配置一律用 static**，不要为了"高级"上动态配置。

### 7.2 日志

```go
logger.Ctx(ctx).Err(err).Msg("查询订单列表失败")     // 有 ctx 首选，自动带 traceId
logger.Ctx(ctx).Info().Msgf("后台任务恢复：重入队%d", n)
logger.Logger.Error().Err(err).Msg("refreshCache: list failed") // 无 ctx 场景（init、后台循环）
logger.Logger.Fatal().Msg("domain.base is empty")   // 启动期致命
```

- 消息写中文，错误消息是中文动宾短语 + "失败"；结构化字段名用英文小驼峰。**traceId 由框架注入，不要自己打**（`logger.CopyTraceId(ctx)` 用于异步 goroutine 延续链路）。
- `logger.Ctx(ctx)` 每次新建 Logger，高频路径开头取一次复用。
- **Error 级别自动打点成 `log_error_total{level}`** → 可预期的业务失败（校验不通过、未登录）**不能记 Error**，否则污染告警。
- 输出：SIT/debug 同时 stdout + 轮转文件，PRD 只写文件；可选 Loki 推送与 error webhook。

### 7.3 鉴权与身份

- **(a) 网关背后的内网服务（推荐）**：网关注入 `X-Account` / `X-User-Name` / `X-Super` / `X-Project` / `X-Client-Ip` / `X-Trace-Id`，服务只读头：`insession.AuthFilter(skipPaths...)`（无账号 401，skipPaths 前缀匹配）、`insession.IsSuperFilter`（403）、`insession.GetOperator(c)` → `{Account, ClientIp}`。

  > **部署硬约束**：头模式服务只能暴露在网关背后的内网（或只绑内网地址），否则任何人都能伪造 `X-Account` / `X-Super: 1` 提权。

- **(b) 自签会话**：`zsf/jwt`，算法固定 HS256，Claims 只有 `account` + 标准过期字段，密钥 `jwt.secret`（prd 未配置直接 Fatal）：

  ```go
  token, err := jwt.GenerateToken(account, time.Now().Add(24*time.Hour))
  claims, err := jwt.ValidateToken(token)
  ```

  推荐在 filter 里解析一次、`c.Set(key, user)`，handler 用 `MustGet` 读（避免重复解析）；`MustGet` 取不到即 panic（说明路由忘挂 filter），由 recovery 记日志返回 500，这是有意的 fail-fast。
- **(c) 对外 API Key 端点**：与登录态无关，自己实现鉴权，挂独立端口，别和门户登录混在一个信任域。

### 7.4 缓存

**`cacheutil.Locked` 全量重建 + 原子替换 + 事件通知刷新**：

```go
var cache = cacheutil.NewLocked(map[string]*Item{})

func init() {
	start.AddInit(func() {
		refresh()
		queue.Subscribe(topicChange, func(_ any) { refresh() })
	}, 4)
}

// refresh 全量加载，构建新 map 原子替换；查询失败保留旧缓存。
func refresh() {
	ctx, closer := database.Context(context.Background())
	defer closer.Close()
	list, err := itemrepo.Repository(ctx).ListAll(nil)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("refreshItemCache: list failed")
		return // 保留旧缓存
	}
	newCache := make(map[string]*Item, len(list))
	for _, m := range list {
		newCache[m.Key] = &Item{/* … */}
	}
	cache.Set(newCache)
}

func Get(key string) *Item { return cache.Get()[key] }
```

- 缓存 key 用业务自然键，**不拼前缀**；**必须幂等**（能重放、不依赖上一次缓存内容）。
- **不要为"消息可能丢"加定时兜底**：进程内队列的发布方与订阅方同进程、不过网络，只有订阅者积压满缓冲（默认 1024）才丢，低频变更碰不到；分布式那套定时对账不要搬进单进程服务。
- `cacheutil.ExpirableCache`（按 key 过期 + 回源 + singleflight + 负缓存）只在确实需要"按 key 过期 + 回源"时用，**不要拿它替换全量重建模式**。
- 检索索引（`zsf/indexer/boolindex`）沿用同一套刷新模式：全量重建 + `atomic.Pointer` 原子替换，刷新失败保留旧快照；要求秒级生效又不想整体重建时，用它的「全量 + 增量分层」。索引 Build 后不可变，**不存在原地增删改**。
- **持有累积状态的长生命周期对象（缓存、索引、变更累积器）必须进程内唯一、由同一处装配**。两个实例各持一半状态时不会有任何报错，只会「部分数据静默不生效」——这类问题极难排查，宁可把它做成包级单例也不要在多处 new。

### 7.5 进程内事件：`zsf/pubsub/pubsub`

```go
queue := pubsub.New(1024)                       // 每订阅者缓冲深度，<=0 用默认 100
queue.Publish("user-change", struct{}{})        // 可投多条：Publish(topic, msg1, msg2)
sub := queue.Subscribe("user-change", func(msg any) { /* 刷新 */ })
queue.Unsubscribe(sub)
```

- **发布非阻塞：订阅者缓冲满则丢弃该消息**。单进程低频变更下是可靠路径；订阅侧仍按**幂等全量重建**写（重复刷新无害、重启可重放）。
- 每个订阅者独立缓冲与消费 goroutine，回调按注册顺序执行；回调 panic 被捕获记日志，不影响后续消息。
- 主题常量集中放服务内 `base/` 的一个文件里，**每个常量的注释写清「发布方 / 订阅方」**。
- **仅适用于单进程部署**；需要跨进程/可靠投递才考虑 `zsf/mq/natsmq` 或 `zsf/pubsub/grpc`（默认不接入）。

### 7.6 后台任务

**实现 `lifecycle.Object`，自己起 goroutine + context 取消，注册进 `WithObjectsList`**：

```go
type BillingWorker struct{}

func (w *BillingWorker) Order() int { return 100 } // 排在 HTTP 服务之后

func (w *BillingWorker) OnApplicationStart() {
	billingWorkerCtx, billingWorkerCancel = context.WithCancel(context.Background())
	billingWorkerWG.Add(1)
	go func() {
		defer billingWorkerWG.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-billingWorkerCtx.Done():
				return
			case <-ticker.C:
				RunDue(billingWorkerCtx, time.Now())
			}
		}
	}()
}

func (w *BillingWorker) AfterInitialize() {}

func (w *BillingWorker) OnApplicationShutdown() {
	billingWorkerCancel()
	billingWorkerWG.Wait() // 必须等当前一轮收尾
}

var _ lifecycle.Object = (*BillingWorker)(nil)
```

- 实现放模块内部时，建议由模块 api 包导出 `func XxxWorker() lifecycle.Object` 转发给启动入口，避免 main 依赖模块内部实现。
- 其他调度手段：
  - `cronutil.AddFunc("0 3 * * *", fn)` / `cronutil.AddLoop(interval, fn)`——**5 段 cron（分 时 日 月 周）不含秒**，表达式非法直接 Fatal；常驻循环用 `AddLoop`（先执行再等间隔）。
  - `chunktask.New[T](max, duration, callback)`——攒批刷新，退出前自动 flush。
  - `countlimiter.New().Acquire(scope, limit)` / `.Release(scope)`——进程内并发槽位（in-flight），不是 QPS 限流。
  - `gopool.NewPool(cap)` / `NewPoolWithQueue(workerCap, queueCap)`——有界协程池；**Close 后或队列满时提交的任务被静默丢弃**，`CtxGo` 的 ctx 已取消则跳过。
- 后台循环**一律 select `quit.Stopping()` 或自己的 ctx**。

### 7.7 可观测性

```go
start.AddInit(promhelper.EnablePushTask) // main 里启用指标推送（push 模式）
```

- 指标全 Summary 类型、框架自动打点：`http_server_request_total{request,code}`、`grpc_server_request_total`、`grpc_client_request_total`、`log_error_total{level}`。
- 推送标签 `service_name` + `ip`；`prom.push.url` **必须写完整导入端点**（`http://host:8428/api/v1/import/prometheus`），只写到端口会被 VictoriaMetrics 400 掉，且失败只走标准库 log、不进 zsf 日志；为空直接 Fatal。
- 退出两级冲刷（Fatal/Panic 即时、优雅退出 Final 钩子），避免指标丢失。
- 链路：`TraceIdFilter` 沿用/生成 `X-Trace-Id` 注入 `ctx` 与 `rpc.Header`；网关反代向下游注入 `X-Account`/`X-Super`/`X-Project`/`X-Client-Ip`/`X-Trace-Id`。

### 7.8 测试

- 只用标准库 `testing`（**不引 testify**），`t.Fatalf` 消息写"X = ..., want ..."或中文；逻辑/解析类优先表驱动；测试与被测文件同包，`_test.go` 结尾。
- 用框架配置的测试加 `TestMain` 调 `static.Init()`（否则 viper 为 nil，取值 panic）。
- 数据库测试把 `xorm.driver` 设为 `sqlite3`（`Sync` 自动建表，无需 migration）；构造唯一键并在 `defer` 里清理。
- **异步断言用轮询，不要 `time.Sleep` 硬等**：

  ```go
  func waitFor(t *testing.T, desc string, cond func() bool) {
  	t.Helper()
  	deadline := time.Now().Add(2 * time.Second)
  	for time.Now().Before(deadline) {
  		if cond() {
  			return
  		}
  		time.Sleep(10 * time.Millisecond)
  	}
  	t.Fatalf("timeout waiting for %s", desc)
  }
  ```

- 路由改动建议补"注册不 panic、路径可达、鉴权生效"的测试（`gin.New()` + 手动调 `Api(e)` + `httptest`）。

---

## 8. 命名与注释

- **注释用中文**；包级注释写在 `doc.go` 或 `api.go` 顶部，说明**包职责边界**；导出符号必须有注释，说明**为什么**而非重复函数名。
- **踩过的坑就地写成注释**（第三方行为、编码陷阱、协议约束）——最有价值的一类注释。
- JSON 字段**小驼峰**；序列化统一 `bytedance/sonic`（`jsonutil.MarshalStringIgnoreErr`）。
- `zsf/utils/listutil` 带 `Ne` 后缀 = 无 error 版本（`Map` 返 `([]K, error)`，`MapNe` 返 `[]K`）；链式用 `listutil.NewPipe(data).FilterNe(...).MapNe(...)`，取结果用 `.Data()` / `.Err()`；提前短路用 `FindFirst*` / `Contains*`，不要自己 for 循环。
- **查表禁止 for 循环逐条查**：联查用 SQL JOIN 或批量 `In`；跨模块的名称联查在仓储层一次取回，service 层循环只做纯映射。
- 常用工具（不要重复造）：`idutil`（`RandomUUID` / `RawUUID` / `GenPlusSnowflakeId`）、`jsonutil`、`strutil.RandomStr4Crypto`、`hashutil`、`aesutil`、`timeutil`（`GetStartOfDay` / `ListDaysBetweenTime` 等）、`validateutil`、`xormutil.IsMysqlDuplicatedEntryError`、`model.Upsert`、`container/hashmap`（`NewSyncMap`）、`container/hashset`、`threadutil.RunSafe`、`bpool`；按条件反向检索规则用 `indexer/boolindex`，别自己写倒排。

---

## 9. 禁止清单（AI 最容易违反的）

1. ❌ 写 `var Api = &router.Api{...}` —— 路由入口只能是 `func Api(e *gin.Engine)`。
2. ❌ 把 `c *gin.Context` 写在 `ginutil.DataSuccess(data, c)` 前面 —— **`c` 永远在最后**。
3. ❌ 响应字段写 `msg` —— 是 `code` / `message` / `data`。
4. ❌ 用 4xx/5xx 表达业务失败 —— **业务失败 HTTP 恒 200**，用 `code`。
5. ❌ DTO 用 `form` / `validate` / `binding` tag —— **只用 `json` tag**（query 也用 json tag 映射）。
6. ❌ 在 handler 里写业务逻辑、开数据库会话、拼 SQL。
7. ❌ service 返回裸 `error` 给 handler —— 统一 `*bizerr.Err`。
8. ❌ 把底层 error 直接透给前端 —— 先记日志，再返回 `errcode.CodeInternalError`。
9. ❌ 在仓储层生成 ID、做业务判断、返回 `*bizerr.Err`。
10. ❌ `Update` 不带 `Cols(...)` 白名单 —— 会把没传的字段清零。
11. ❌ 忘记 `defer closer.Close()`，或跨慢调用持有数据库会话。
12. ❌ 事务里用外层 `ctx` 构造仓储（要用 `txCtx`）。
13. ❌ 跨模块 import 别人的 `internal/repository` —— 走 `base/` 或对方 api 包的导出函数。
14. ❌ 把读配置的逻辑裸写在 `init()` 里 —— 必须包进 `start.AddInit`。
15. ❌ 有先后依赖的初始化钩子省略 order（或给同一个 order）靠"谁先注册谁先跑" —— 跨包 `init()` 顺序由 import 图决定，有依赖必须写不同 order。
16. ❌ 只写 `start.AddInit` / `quit.Add*Hook` 而 main 里没有 `lifecycle.Run` —— 这些钩子没有第二个执行方。
17. ❌ 把 `WithObjects` / `WithObjectsList` 拆在多处调用 —— 覆盖语义，写在别处的会被顶掉。
18. ❌ 后台循环不监听 `quit.Stopping()` / 不 `cancel()` → 进程退不掉；退出不 `wg.Wait()` → 丢数据。
19. ❌ 在非 SIT 环境指望 `database.Sync` 改表 —— 生产是空操作，必须人工 DDL。
20. ❌ 给可预期的业务失败（校验、未登录）打 `Error` 日志 —— 会污染 `log_error_total` 告警。
21. ❌ 自己 new xorm engine / 自己实现 pubsub / 自己写 UUID 生成 —— 框架都有了。
22. ❌ 为进程内事件加"定时兜底刷新"这类分布式可靠性机制 —— 单进程同进程通知，不需要。
23. ❌ 顺手把可选能力（llm / sqlitereplica / autohttps / natsmq / grpc）接进 main.go —— 只有明确需要时才接。

---

## 10. 交付前自检

- [ ] `go build ./...`、`go vet ./...` 通过；新增 `AddInit` 的 order 无冲突，有依赖的写了不同 order。
- [ ] main 里调了 `lifecycle.Run`，新对象加进了那**一处**装配。
- [ ] 新表：`model.go` 有 `TableName`/`TableComment`/中文 `comment`，`impl.go` 的 `init` 里 `database.Sync`。
- [ ] 新接口：路由注释列了端点、挂了正确的鉴权中间件、分页用了 `IsValid()`。
- [ ] 错误路径都走日志 + `base/errcode` 里的码，写操作先做存在性检查。
- [ ] 新缓存/索引是幂等全量重建、失败保留旧快照；**持有累积状态的对象（缓存、索引、变更累积器）是进程内单例**；新 goroutine 都能被 `quit.Stopping()` 或 cancel 终止。
- [ ] 必需的配置项在启动钩子里做了非空校验（为空即 Fatal）。
- [ ] 中文注释写清了"为什么"和踩过的坑。

---

## 11. zsf 可选能力（默认不接入）

| 包 | 解决什么 |
| --- | --- |
| `zsf/http/autohttps` | ACME 证书自动签发 + 热替换（配 `WithGetCertificate`） |
| `zsf/grpc/{client,server}` + `services/{discovery,registry,lb}` + `instance` | gRPC 封装、服务发现与注册、负载均衡（`Dial` 的连接由框架统一关闭，调用方别 Close） |
| `zsf/mq/natsmq` / `zsf/pubsub/grpc` | 跨进程可靠投递 / 多实例广播 |
| `zsf/leaderelection` | 基于 MySQL 租约的选主 |
| `zsf/utils/sentinelutil` + `FlowLimitFilter` / `CircuitBreakerFilter` | Sentinel 限流熔断 |
| `zsf/llm/*` / `zsf/sqlitereplica` | LLM 应用骨架（engine/session/tool/hitl/mcp）/ sqlite 主从复制 |
| `zsf/indexer/boolindex` | 规则条件索引：把百万级「带条件的规则」常驻内存，按一份画像反向查出所有命中的规则（枚举 + 数值范围，含全量+增量分层更新） |
| `zsf/constants` / `zsf/env` / `zsf/instance` | 目录常量、环境（`SF_ENV` / `IsDev` / `LocalIP`）、应用信息 |

**判断标准**：这个能力是不是本次需求的一部分？不是就别接进 main.go 与配置——多引一个包就多一份启动时的 Fatal 风险。
