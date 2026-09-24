// Package session 有状态对话会话：自动管理消息历史、工具循环、用量累计，
// 支持历史裁剪/摘要压缩、快照持久化、中断与内置 steering（HITL）。
// 各 client 包提供带默认 Adapter 的便捷构造。
package session

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"uuid"

	"github.com/LeeZXin/zsf/llm/engine"
	"github.com/LeeZXin/zsf/llm/event"
	"github.com/LeeZXin/zsf/llm/hitl"
	"github.com/LeeZXin/zsf/llm/tool"
	"github.com/bytedance/sonic"

	"github.com/LeeZXin/zsf/utils/jsonutil"
)

// ErrSessionClosed 会话已关闭。
var ErrSessionClosed = errors.New("llm: session closed")

// ErrFailInjectSteer steering 注入失败：会话执行中调用 Send 插话时
// 内置 steering 通道已满（引擎未消费且 ctx 未取消）。
var ErrFailInjectSteer = errors.New("llm: fail to inject steer")

// steerBufferSize 内置 steering 通道缓冲大小。
const steerBufferSize = 16

// Snapshot 会话快照：历史与统计的完整形态，用于跨进程持久化/恢复。
// History 为消息历史的 JSON 序列化，由 GetSnapshot 生成，
// 经 Config.Snapshot 传入可在 New 时恢复会话状态。
// JSON 键为 camelCase，
// 消费方（如 tinyagent 会话落盘）直接序列化本结构。
type Snapshot struct {
	// History 消息历史 JSON，反序列化目标类型为会话的消息类型 M。
	History []byte `json:"history"`
	// Rounds 已完成的对话轮数。
	Rounds int64 `json:"rounds"`
	// AccumulatedTokens 累计消耗 token 总量。
	AccumulatedTokens uint64 `json:"accumulatedTokens"`
	// Usage 累计用量细分。
	Usage event.Usage `json:"usage"`
	// ContextTokens 快照时上下文 token 估算（恢复时以重算值为准）。
	ContextTokens uint64 `json:"contextTokens"`
	// Summary 历史压缩产生的累积摘要，随 system prompt 注入下一轮。
	Summary string `json:"summary"`
	// LastActive 最后活动时间，随快照保存与恢复。
	LastActive time.Time `json:"lastActive"`
}

// Config 会话配置。
type Config[M any] struct {
	// ID 会话唯一标识，空则由 New 自动生成
	// 持久化/恢复场景传入原 ID 可保持跨进程标识一致。
	ID string
	// Prompt system prompt，会话首条消息（由引擎注入，裁剪时始终保留）。
	Prompt string
	// MaxHistoryRounds 保留的最大历史轮次（一轮 = 一条 user 消息起的消息序列），
	// <=0 表示不按轮次裁剪。
	MaxHistoryRounds int
	// MaxHistoryTokens 历史消息总 token 估算上限（uint64），超出时触发裁剪，0 不限制。
	// 裁剪优先按 MaxHistoryRounds 执行；两者均为 0 时从不裁剪。
	MaxHistoryTokens uint64
	// Compress 上下文压缩回调：把被裁剪的旧消息压缩为一段摘要，
	// 摘要累积后随 system prompt 注入后续轮次（不进入消息列表）。
	// nil 时使用默认实现：经本会话 adapter 触发一次无工具总结
	//（提示词 DefaultCompressPrompt）；返回空摘要视为无需注入。
	// 只想直接丢弃被裁剪消息时，显式传入返回空摘要的空实现。
	// 返回错误时降级：丢弃被裁剪消息继续（不注入摘要，不打断本轮）。
	Compress func(ctx context.Context, dropped []M) (string, error)
	// OnMessageAppend 本轮成功完成后触发：参数为本轮新增的完整消息增量
	// （含思考链、工具消息与 steering 注入的 user 消息），用于增量持久化/审计。
	// 轮内压缩后原历史前缀可能已不在 out 中，增量按 orig 最长后缀匹配计算。
	// 消息在回调返回后不再被框架修改，可安全异步消费（如丢给落库 worker）。
	// 注意：持锁路径内同步调用，回调内不得调用 GetSnapshot（死锁）、
	// 不宜长时间阻塞。
	OnMessageAppend func([]M)
	// TokenCounter token 估算函数，为 nil 时按字符集粗略估算（ASCII 按 4 字符/token，其余按 2 字符/token）。
	TokenCounter func(text string) uint64
	// SteerTimeout 内置 steering 通道每轮工具调用后的等待时长，0 表示非阻塞检查一次。
	// 仅当 Steer 未显式配置时生效。
	SteerTimeout time.Duration
	// Snapshot 会话快照：New 时校验并恢复（历史反序列化失败会 panic，
	// 属于初始化期配置错误）。恢复后 ContextTokens 按恢复的历史重算。
	Snapshot *Snapshot
	// 以下为会话级运行默认值（Send 时组装为引擎 engine.RunOptions，roundOpts 可逐轮并入）：

	// MaxToolRounds 工具调用最大轮次，<=0 时由引擎取 DefaultMaxToolRounds。
	MaxToolRounds int
	// Steer 每轮工具调用后的 steering 钩子；nil 时挂载内置通道实现
	//（steerChan + SteerTimeout）。
	Steer engine.SteerFunc
	// Stream 默认流式开关，Send 的 roundOpts 可逐轮覆盖。
	Stream bool
	// Events 会话级事件接收器：会话生命周期事件（round_* / steer_injected 等）
	// 与每轮引擎/工具事件统一经其分发（SessionID 自动补全）；
	// 本轮接收器随 Send 的 roundOpts 传入并经 event.MultiSink 并入。
	Events event.Sink
}

// Session 有状态的对话会话：自动管理消息历史、工具循环、用量累计，
// 并支持历史裁剪/摘要压缩、中断、空闲过期与序列化恢复。
// 同一会话内的 Send 串行执行（互斥），可安全并发调用。
// M 为厂商消息类型，通过各 client 包的 New 便捷构造。
type Session[M any] struct {
	// id 会话唯一标识。
	id      string
	adapter engine.Adapter[M]
	config  Config[M]

	mu      sync.Mutex
	history []M
	// summary 历史压缩产生的摘要（累积，快照元数据）。
	// 注入下一轮时写成 user 压缩片段，不拼进 system prompt。
	summary string
	// ExtraPrompt 附加 system prompt：每次 Send 组装消息时拼接在基础
	// Prompt 之后（见 effectivePrompt）。存 string；无锁读写
	// （atomic.Value），工具回调等会话持锁路径内经 SetExtraPrompt 替换安全。
	ExtraPrompt atomic.Value
	closed      bool
	// fragments 忙时待注入的上下文片段（InjectFragment 在抢不到主锁时写入）。
	fragments []string
	fragMu    sync.Mutex
	// lastActiveNano 最后活动时间（UnixNano），无锁，回调路径内可安全读取。
	lastActiveNano atomic.Int64
	// 以下统计均为 atomic 无锁存储：在 OnEvent 回调
	// （send 持主锁路径内）可安全读取。
	// round 已完成的对话轮数。
	round atomic.Int64
	// contextTokens 当前历史上下文的 token 估算量（每轮 Send 后更新，含裁剪后）。
	contextTokens atomic.Uint64
	// accumulatedTokens 会话生命周期内累计消耗的 token 总量（usage 之和，裁剪不影响累计）。
	accumulatedTokens atomic.Uint64
	// usagePrompt / usageCompletion / usageTotal usage 累计分量。
	usagePrompt     atomic.Uint64
	usageCompletion atomic.Uint64
	usageTotal      atomic.Uint64

	// steerChan 内置 steering 输入通道：会话执行期间调用 Send 的消息
	// 会进入该通道，由引擎在每轮工具调用后消费注入下一轮。
	// 永不被 close：注入与 Close 并发时不会 panic，多余消息随 session 回收。
	// 缓冲满时 injectSteer 立即失败（返回 ErrFailInjectSteer），不阻塞调用方。
	steerChan chan string

	// runCancelFunc 当前执行轮的取消函数（send 开始时存入、结束时清空）。
	// 用 atomic.Pointer 而非 atomic.Value：Value.Store(nil) 会 panic。
	runCancelFunc atomic.Pointer[context.CancelFunc]
	// roundEvents 当前执行轮的本轮观察器（send 开始时存入、结束时清空）。
	// 无锁存取：emit 可能被 injectSteer（其他协程）、引擎/工具事件（持锁路径）
	// 并发调用；所有事件（含 round_* / steer_injected）统一双发到
	// 会话级观察器与本轮观察器。
	// 经 holder 包装存储：EventSink 是接口，atomic.Value 同样禁止 Store(nil)。
	roundEvents atomic.Pointer[roundEventsHolder]
}

// roundEventsHolder roundEvents 的存储包装：让 atomic.Pointer 能以 nil
// 表示"当前无本轮观察器"。
type roundEventsHolder struct {
	sink event.Sink
}

// New 构建会话。各 client 包提供带默认 adapter 的便捷构造。
//
// 内置 steering：config.Steer 未显式配置时，会话自动挂载内置通道，
// 会话执行期间调用 Send 的消息会作为 steering 注入，
// 每轮工具调用后按 SteerTimeout 等待新输入。
func New[M any](adapter engine.Adapter[M], config Config[M]) *Session[M] {
	id := config.ID
	if id == "" {
		id = strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	s := &Session[M]{
		id:        id,
		adapter:   adapter,
		config:    config,
		steerChan: make(chan string, steerBufferSize),
	}
	userSteer := config.Steer
	if userSteer == nil {
		// 未显式配置 Steer 时挂载内置通道：会话执行期间 Send 的消息
		// 经 steering 注入，每轮工具调用后按 SteerTimeout 等待新输入。
		// 同一通道三处使用：injectSteer 写入、HITL 工具（ctx 注入）消费、
		// Steer 钩子在每轮工具后消费。
		userSteer = engine.NewChanSteerFunc(s.steerChan, config.SteerTimeout)
	}
	// 忙时 InjectFragment 的片段经 Steer 并入下一轮 LLM 调用（工具
	// 回调持主锁，不能再抢 s.mu 写 history）。
	s.config.Steer = func(ctx context.Context) []string {
		return append(s.drainFragments(), userSteer(ctx)...)
	}
	if s.config.Compress == nil {
		// 缺省压缩实现：经本会话 adapter 对裁剪的旧消息触发总结。
		s.config.Compress = s.defaultCompress()
	}

	// 快照恢复：校验成功才加载，失败按初始化期配置错误处理（panic）。
	if config.Snapshot != nil {
		s.restoreSnapshot(config.Snapshot)
	}
	return s
}

// restoreSnapshot 校验并恢复快照状态。调用方须保证 snapshot 非 nil。
// 历史反序列化失败 panic；统计按快照恢复，ContextTokens 以重算值为准。
func (s *Session[M]) restoreSnapshot(snapshot *Snapshot) {
	var history []M
	if err := sonic.Unmarshal(snapshot.History, &history); err != nil {
		panic(fmt.Sprintf("llm: restore session snapshot: %v", err))
	}
	s.history = history
	s.summary = snapshot.Summary
	// 旧快照把摘要放在 Summary、不在历史里：恢复时补一条片段，
	// 不再把摘要拼进 system（effectivePrompt 不含 summary）。
	if s.summary != "" && !historyHasCompactFragment(s.adapter, s.history) {
		s.history = append([]M{s.adapter.ConvertToUserMessage(FormatCompactFragment(s.summary))}, s.history...)
	}
	s.round.Store(snapshot.Rounds)
	s.accumulatedTokens.Store(snapshot.AccumulatedTokens)
	s.usagePrompt.Store(uint64(snapshot.Usage.PromptTokens))
	s.usageCompletion.Store(uint64(snapshot.Usage.CompletionTokens))
	s.usageTotal.Store(uint64(snapshot.Usage.TotalTokens))
	s.contextTokens.Store(s.estimateTokens(history))
	if !snapshot.LastActive.IsZero() {
		s.lastActiveNano.Store(snapshot.LastActive.UnixNano())
	}
}

// Send 向会话发送一条用户消息，行为取决于会话状态：
//   - 会话空闲：开始一轮对话（追加消息、运行完整工具循环），
//     返回本轮 assistant 的最终回复文本；
//   - 会话执行中：消息注入 steering 通道（工具等待确认可读到、
//     引擎在每轮工具调用后消费），注入成功返回空响应，
//     缓冲满且 ctx 未取消时返回 ErrFailInjectSteer。
//
// events 仅作用于本次调用（提问轮次）的本轮观察器，多轮请求可各传各的
// 观察者，不互相污染；经 event.MultiSink 自动并入会话级观察器（config.Events）；
// 注入路径忽略。nil 表示本轮无独立观察器。
// 会话已关闭时返回 ErrSessionClosed。
//
// 锁语义：本轮结束事件（round_completed / round_failed）在解锁后发出，
// 其 sink 可安全调用 GetSnapshot 做事件驱动持久化（若下一轮已开始则阻塞至
// 其结束，不会死锁）。代价是并发 Send 下结束事件可能与下一轮的开始事件
// 交错到达（事件携带 Round 字段可区分）——轮次本身仍由主锁严格串行。
func (s *Session[M]) Send(ctx context.Context, userMsg string, events event.Sink) (string, error) {
	if s.mu.TryLock() {
		if s.closed {
			s.mu.Unlock()
			return "", ErrSessionClosed
		}
		holder := &roundEventsHolder{sink: events}
		resp, err, end := s.sendLocked(ctx, holder, userMsg)
		s.mu.Unlock()
		if end.Type != "" {
			s.emit(end)
		}
		// 结束事件发出后才清空本轮观察器（结束事件仍可达本轮 sink）；
		// CAS 防并发 Send 抢占锁后新挂载的观察器被误清。
		s.roundEvents.CompareAndSwap(holder, nil)
		return resp, err
	}
	// 抢不到锁，大概率不会closed 说明busy
	ok, err := s.injectSteer(ctx, userMsg)
	if err != nil {
		return "", err
	}
	if ok {
		return "", nil
	}
	return "", ErrFailInjectSteer
}

// ID 返回会话唯一标识（NewSession 时由配置指定或自动生成）。
func (s *Session[M]) ID() string {
	return s.id
}

// IsBusy 返回会话是否正在执行一轮对话（Send 进行中，
// 含模型调用、工具循环、steering 等待与压缩）。
// 无锁，回调路径内可安全调用。
func (s *Session[M]) IsBusy() bool {
	if s.mu.TryLock() {
		defer s.mu.Unlock()
		return false
	}
	return true
}

// injectSteer 注入 steering 并等待引擎消费：
// 缓冲有空间时立即返回；满时阻塞，直到引擎消费、会话结束本轮（返回
// ErrSteerDiscarded 供调用方重试）或 ctx 取消。
func (s *Session[M]) injectSteer(ctx context.Context, userMsg string) (bool, error) {
	select {
	case s.steerChan <- userMsg:
		s.emit(event.Event{Type: event.SteerInjected, Content: userMsg})
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	default:
		return false, nil
	}
}

// emit 唯一事件分发点：补全 SessionID 与时间戳，双发到会话级观察器
// （config.Events，全局审计）与当前执行轮的本轮观察器（随 Send 传入）。
// 并发安全：injectSteer（其他协程）、引擎/工具事件（持锁路径）均经此分发，
// 观察器实现需并发安全。
func (s *Session[M]) emit(ev event.Event) {
	sink := event.MultiSink(s.config.Events, s.currentRoundEvents())
	if sink == nil {
		return
	}
	ev.SessionID = s.id
	ev.Timestamp = time.Now()
	sink.Emit(ev)
}

// currentRoundEvents 返回当前执行轮的本轮观察器（send 执行期间有效，否则 nil）。
// 无锁，回调路径内可安全调用。
func (s *Session[M]) currentRoundEvents() event.Sink {
	v := s.roundEvents.Load()
	if v == nil {
		return nil
	}
	return v.sink
}

// sendLocked 执行一轮对话，调用方需持主锁。第三个返回值为本轮结束事件
// （round_completed / round_failed，零值表示本轮未结束——由 Send 在会话已关闭
// 时短路，不会走到这里），由 Send 在解锁后 emit：结束事件 sink 处于锁外，
// 可安全调用 GetSnapshot 做事件驱动持久化。
// holder 为本轮观察器包装：sendLocked 挂载，Send 在结束事件发出后 CAS 清空。
func (s *Session[M]) sendLocked(ctx context.Context, holder *roundEventsHolder, userMsg string) (string, error, event.Event) {
	// 中断感知：Cancel 或 Close 后新的 Send 直接失败。
	// 注入 steering 通道：工具可在当前调用内等待用户确认（HITL）。
	runCtx, cancelFunc := context.WithCancel(ctx)
	runCtx = hitl.WithSteerChan(runCtx, s.steerChan)
	// 本轮观察器生效期：所有事件（round_* / 引擎 / 工具，含 approval_requested）
	// 经 emit 统一双发到会话级与本轮观察器。
	s.roundEvents.Store(holder)
	// 注入事件接收器（emit 本身）：工具与引擎的事件经 ctx 取用，
	// 统一补 SessionID 并双发。
	if s.config.Events != nil || holder.sink != nil {
		runCtx = event.WithSink(runCtx, event.SinkFunc(s.emit))
	}
	s.runCancelFunc.Store(&cancelFunc)
	defer func() {
		cancelFunc()
		s.runCancelFunc.Store(nil)
	}()
	// 空闲路径未写入 history 的忙时片段（上一轮收尾后残留）并入历史。
	for _, f := range s.drainFragments() {
		s.history = append(s.history, s.adapter.ConvertToUserMessage(f))
	}
	// 轮次
	round := s.round.Load() + 1
	s.emit(event.Event{Type: event.RoundStarted, Round: round})
	// 构造本轮消息：system（prompt 非空时）+ 历史 + 本轮 user 消息。
	// 失败/中断时不修改 s.history（未完成的对话不进历史）；
	// 工具执行的副作用无法撤销，usage 保留（token 已真实消耗）。
	messages := make([]M, 0, len(s.history)+2)
	prompt := s.effectivePrompt()
	if len(prompt) > 0 {
		messages = append(messages, s.adapter.ConvertToSystemMessage(prompt))
	}
	messages = append(messages, s.history...)
	messages = append(messages, s.adapter.ConvertToUserMessage(userMsg))
	// 组装本轮引擎运行选项：MaxToolRounds / Steer / Stream 取会话级配置，
	// Events 经 event.MultiEventSink 并入本轮观察器。
	opts := &engine.RunOptions[M]{
		MaxToolRounds:  s.config.MaxToolRounds,
		Steer:          s.config.Steer,
		Stream:         s.config.Stream,
		Events:         event.MultiSink(s.config.Events, holder.sink),
		Tools:          s.adapter.DefaultTools(),
		CompactContext: s.compactContext,
	}
	origHistory := s.history
	origSummary := s.summary
	origCtxTok := s.contextTokens.Load()
	_, out, usages, err := engine.RunChatCompletion(runCtx, s.adapter, messages, opts)
	for _, u := range usages {
		s.accumulateUsage(u)
	}
	if err != nil {
		// 轮内压缩可能已改 summary；失败轮不进历史，摘要一并回滚以免与旧消息重复。
		s.summary = origSummary
		s.contextTokens.Store(origCtxTok)
		for _, f := range s.drainFragments() {
			s.history = append(s.history, s.adapter.ConvertToUserMessage(f))
		}
		// 结束事件由 Send 在解锁后发出（见 Send 锁语义注释）。
		return "", err, event.Event{Type: event.RoundFailed, Round: round, Err: err.Error()}
	}
	s.round.Add(1)
	s.lastActiveNano.Store(time.Now().UnixNano())
	// 历史不含 system：下轮由 effectivePrompt 重新注入。
	// MessageRole 能识别 system 时按角色剥离；测试用 string adapter
	// 不区分 role，则按本轮是否注入过 system 剥掉首条。
	if len(out) > 0 && s.adapter.MessageRole(out[0]) == roleSystem {
		out = out[1:]
	} else if len(prompt) > 0 && len(out) > 0 {
		out = out[1:]
	}
	if s.config.OnMessageAppend != nil {
		// 轮内压缩后 origHistory 可能不再是 out 的前缀：用最长 orig 后缀匹配。
		if delta := s.messagesDelta(origHistory, out); len(delta) > 0 {
			s.config.OnMessageAppend(delta)
		}
	}
	out = s.adapter.ClearReasoningContent(out) // 清空思考链，防止进入下轮对话
	s.history, err = s.trim(runCtx, out)       // out不包含提示词部分
	if err != nil {
		return "", err, event.Event{Type: event.RoundFailed, Round: round, Err: err.Error()}
	}
	s.contextTokens.Store(s.estimateTokens(s.history))
	usage := s.TotalUsage()
	return s.adapter.MessageContent(s.lastAssistantMessage()), nil, event.Event{
		Type:  event.RoundCompleted,
		Round: s.round.Load(),
		Usage: &usage,
	}
}

// Cancel 中断当前正在执行的 Send（取消其底层请求）。
// 无正在执行的调用时是安全的空操作。
// 注意：不持有主锁，Send 阻塞在模型请求期间也可被取消。
func (s *Session[M]) Cancel() {
	if cancelFunc := s.runCancelFunc.Load(); cancelFunc != nil {
		(*cancelFunc)()
	}
}

// SetExtraPrompt 替换附加 system prompt，下一轮消息组装时生效。
// 无锁（atomic.Value）：工具回调等会话持锁路径内可安全调用，不会死锁。
// 运行期记忆/技能/授权变更应走 InjectFragment，避免改 ExtraPrompt 打崩前缀缓存。
func (s *Session[M]) SetExtraPrompt(prompt string) {
	s.ExtraPrompt.Store(prompt)
}

// InjectFragment 注入一条模型可见的 user 片段（压缩摘要、中断说明、
// 记忆/技能/授权更新）。空闲时直接追加历史；执行中写入 pending，
// 由 Steer 在下一轮 LLM 调用前并入（工具回调持主锁，不能再抢 s.mu）。
func (s *Session[M]) InjectFragment(content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	if s.mu.TryLock() {
		s.history = append(s.history, s.adapter.ConvertToUserMessage(content))
		s.mu.Unlock()
		return
	}
	s.fragMu.Lock()
	s.fragments = append(s.fragments, content)
	s.fragMu.Unlock()
}

func (s *Session[M]) drainFragments() []string {
	s.fragMu.Lock()
	defer s.fragMu.Unlock()
	if len(s.fragments) == 0 {
		return nil
	}
	out := s.fragments
	s.fragments = nil
	return out
}

// Clear 清空会话的对话上下文：移除全部消息历史与压缩摘要。
// 统计（轮数、累计用量）保留（会话生命周期统计语义）；
// 空闲计时与运行状态不受影响。
func (s *Session[M]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = nil
	s.summary = ""
	s.contextTokens.Store(0)
}

// Close 关闭会话：中断当前执行，此后 Send 返回 ErrSessionClosed。
// 中断部分同 Cancel，不持有主锁。
// session_closed 事件在解锁后发出（同 Send 的结束事件语义）：
// 其 sink 可安全调用 GetSnapshot 取最终状态。
func (s *Session[M]) Close() {
	s.Cancel()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.emit(event.Event{Type: event.SessionClosed})
}

// accumulateUsage 累加一轮 usage 到原子统计，调用方需持主锁（与历史更新同一临界区）。
func (s *Session[M]) accumulateUsage(u event.Usage) {
	s.usagePrompt.Add(uint64(u.PromptTokens))
	s.usageCompletion.Add(uint64(u.CompletionTokens))
	s.usageTotal.Add(uint64(u.TotalTokens))
	s.accumulatedTokens.Add(uint64(u.TotalTokens))
}

// Rounds 返回已完成的对话轮数。无锁，回调路径内可安全调用。
func (s *Session[M]) Rounds() int {
	return int(s.round.Load())
}

// TotalUsage 返回累计 token 用量（细分 prompt/completion/total）。无锁，回调路径内可安全调用。
func (s *Session[M]) TotalUsage() event.Usage {
	return event.Usage{
		PromptTokens:     int(s.usagePrompt.Load()),
		CompletionTokens: int(s.usageCompletion.Load()),
		TotalTokens:      int(s.usageTotal.Load()),
	}
}

// AccumulatedTokens 返回会话生命周期内累计消耗的 token 总量，
// 历史裁剪不影响累计值。无锁，回调路径内可安全调用。
func (s *Session[M]) AccumulatedTokens() uint64 {
	return s.accumulatedTokens.Load()
}

// CurrentContextTokens 返回当前历史上下文的 token 估算量，
// 每轮 Send（含裁剪）后更新。无锁，回调路径内可安全调用。
func (s *Session[M]) CurrentContextTokens() uint64 {
	return s.contextTokens.Load()
}

// GetSnapshot 返回会话快照（历史 + 全部统计），用于持久化；
// 经 Config.Snapshot 传入可在 New 时恢复会话状态。
// 注意：需要主锁。轮次执行中的事件（round_started、工具事件等）在
// 持主锁路径内同步分发，其回调中调用 GetSnapshot 会死锁；
// round_completed / round_failed / session_closed 在解锁后分发，
// 其回调中可安全调用（若下一轮已开始则阻塞至其结束，不会死锁）。
func (s *Session[M]) GetSnapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{
		History:           jsonutil.MarshalIgnoreErr(s.history),
		Rounds:            s.round.Load(),
		AccumulatedTokens: s.accumulatedTokens.Load(),
		Usage:             s.TotalUsage(),
		ContextTokens:     s.contextTokens.Load(),
		Summary:           s.summary,
		LastActive:        s.LastActive(),
	}
}

// LastActive 返回最后活动时间（最近一次成功完成的 Send 时间）。
// 无已完成的轮次时返回零值 time.Time（而非 Unix epoch）——
// 零值有明确的「从未活动」语义，序列化后 IsZero 判定仍成立
// （time.Unix(0,0) 是 1970 年，会被错误地当成「非常久远」）。
// 无锁，回调路径内可安全调用。
func (s *Session[M]) LastActive() time.Time {
	if n := s.lastActiveNano.Load(); n != 0 {
		return time.Unix(0, n)
	}
	return time.Time{}
}

// lastAssistantMessage 返回历史中最后一条 assistant 消息，调用方需持锁。
func (s *Session[M]) lastAssistantMessage() M {
	var zero M
	for _, v := range slices.Backward(s.history) {
		if s.adapter.MessageRole(v) == "assistant" {
			return v
		}
	}
	return zero
}

// trim 历史裁剪：超出 MaxHistoryRounds 或 MaxHistoryTokens 时，
// 丢弃旧消息（经 Compress 压缩成摘要累积注入 system prompt）。
// 轮次裁剪保留最近 N 个 user 起的消息；token 裁剪不拆 tool pair，
// 当前一轮超限时保留最后一条 user、丢掉该轮较早的工具轨迹。
func (s *Session[M]) trim(ctx context.Context, messages []M) ([]M, error) {
	if s.config.MaxHistoryRounds <= 0 && s.config.MaxHistoryTokens <= 0 {
		return messages, nil
	}
	// 轮次裁剪：从后往前保留最近 N 个 user 消息起的全部消息。
	if s.config.MaxHistoryRounds > 0 {
		keepFrom := messages
		users := 0
		for i, v := range slices.Backward(messages) {
			if s.adapter.MessageRole(v) == tool.User {
				users++
				if users == s.config.MaxHistoryRounds {
					keepFrom = messages[i:]
					break
				}
			}
		}
		cut := len(messages) - len(keepFrom)
		cut = s.alignKeptStart(messages, cut)
		if cut > 0 && cut < len(messages) {
			newMessages, err := s.drop(ctx, messages[:cut], messages[cut:])
			if err != nil {
				return messages, err
			}
			messages = newMessages
		}
	}
	// token 裁剪：整体仍超限时腾窗口。合法切点不拆 tool pair；
	// 当前一轮本身超限时保留最后一条 user，丢掉该轮较早的工具轨迹。
	if s.config.MaxHistoryTokens > 0 {
		dropped, kept := s.clipToTokenLimit(messages, s.config.MaxHistoryTokens)
		if len(dropped) > 0 {
			newMessages, err := s.drop(ctx, dropped, kept)
			if err != nil {
				return messages, err
			}
			messages = newMessages
		}
	}
	return messages, nil
}

// DefaultCompressPrompt 默认压缩提示词（结构化 9 段，对齐 Claude Code 的
// 压缩总结结构）：保留后续轮次真正需要的语义（目标、文件与代码、
// 错误与修正、待办、当前进展），丢弃过程噪音。
const DefaultCompressPrompt = `你是对话历史的压缩器。下面是被裁剪出上下文的对话历史，请将其压缩为一份结构化摘要，供后续对话轮次替代被裁剪的历史。

严格基于给定对话内容，不得臆造或补充新信息；直接输出摘要正文，不要前缀或额外解释。按以下段落组织，无相关内容的段写「无」：

1. 目标与意图：用户的全部显式请求与意图变化
2. 关键技术点：涉及的技术概念、框架、命名约定、设计约束
3. 文件与代码：检查/修改/创建过的文件、关键代码片段及其重要性
4. 错误与修正：出现过的错误、修复方式；用户明确纠正过的做法必须原样保留（后续不得再犯）
5. 问题解决：已解决的问题与排查中的事项
6. 用户全部消息：非工具结果的用户消息要点（防止意图漂移）
7. 待办事项：用户明确要求但尚未完成的任务
8. 当前进展：压缩前正在处理的内容与进度
9. 下一步：与最近请求直接相关的下一步，并附用户原话；没有则写「无」

输出格式示例：
1. 目标与意图：
   - ...
2. 关键技术点：
   - ...`

// defaultCompress 默认压缩实现：使用会话自身的 adapter 对被裁剪的
// 旧消息触发一次无工具、非流式的总结，返回摘要文本。
// 总结失败时返回错误（由 drop 降级处理）；空结果返回空摘要（视为无需注入）。
func (s *Session[M]) defaultCompress() func(ctx context.Context, dropped []M) (string, error) {
	return func(ctx context.Context, dropped []M) (string, error) {
		if len(dropped) == 0 {
			return "", nil
		}
		messages := []M{
			s.adapter.ConvertToSystemMessage(DefaultCompressPrompt),
			s.adapter.ConvertToUserMessage(joinDroppedMessages(s.adapter, dropped)),
		}
		res, err := s.adapter.Complete(ctx, messages, s.adapter.CompressModel(), nil)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(res.Content), nil
	}
}

// joinDroppedMessages 将被裁剪的消息拼接为 "role: content" 逐行文本，
// 作为压缩总结的输入。
func joinDroppedMessages[M any](adapter engine.Adapter[M], dropped []M) string {
	var b strings.Builder
	for _, m := range dropped {
		b.WriteString(adapter.MessageRole(m))
		b.WriteString(": ")
		content := adapter.MessageContent(m)
		if len(content) > maxDroppedMessageChars {
			content = content[:maxDroppedMessageChars] + fmt.Sprintf("\n[... truncated %d chars]", len(content)-maxDroppedMessageChars)
		}
		b.WriteString(content)
		b.WriteByte('\n')
	}
	return b.String()
}

// maxDroppedMessageChars 送给摘要模型的单条消息上限，避免压缩请求自身溢出。
const maxDroppedMessageChars = 2000

// drop 压缩（或丢弃）被裁剪的消息，并拼接保留部分。调用方需持锁。
// 过程经事件流分发 compress_started / compress_completed（Before/After 语义合一）。
// 摘要累积到 s.summary（快照元数据），并以 user 片段插入保留区之前——
// 不进 system prompt，避免改前缀打崩缓存。
// 压缩失败降级：丢弃被裁剪消息继续、不注入摘要（错误已随 compress_completed
// 事件发出）——摘要服务的故障不打断对话。
func (s *Session[M]) drop(ctx context.Context, dropped, kept []M) ([]M, error) {
	if s.config.Compress == nil {
		return kept, nil
	}
	s.emit(event.Event{Type: event.CompressStarted, DroppedCount: len(dropped), KeptCount: len(kept)})
	begin := time.Now()
	summary, err := s.config.Compress(ctx, dropped)
	duration := time.Since(begin)
	compressEvent := event.Event{Type: event.CompressCompleted, Duration: duration, Content: summary}
	if err != nil {
		compressEvent.Err = err.Error()
	}
	s.emit(compressEvent)
	if err != nil {
		// 压缩失败降级（Claude Code 同款失败保护）：丢弃被裁剪消息继续，
		// 不注入摘要——摘要服务的故障不得打断对话（失败信息已随事件发出）。
		return kept, nil
	}
	if summary == "" {
		return kept, nil
	}
	if s.summary == "" {
		s.summary = summary
	} else {
		s.summary = s.summary + "\n\n" + summary
	}
	fragment := s.adapter.ConvertToUserMessage(FormatCompactFragment(summary))
	return append([]M{fragment}, kept...), nil
}

// effectivePrompt 组装实际下发的 system prompt：基础 Prompt + 附加
// ExtraPrompt。压缩摘要走历史片段，不拼进 system（前缀缓存纪律）。
// 调用方需持锁（ExtraPrompt 为无锁原子值，任意路径读取安全）。
func (s *Session[M]) effectivePrompt() string {
	prompt := s.config.Prompt
	if v := s.ExtraPrompt.Load(); v != nil {
		prompt += v.(string)
	}
	return prompt
}

func historyHasCompactFragment[M any](adapter engine.Adapter[M], history []M) bool {
	for _, m := range history {
		if strings.Contains(adapter.MessageContent(m), CompactFragmentOpen) {
			return true
		}
	}
	return false
}

func (s *Session[M]) tokenCounter() func(string) uint64 {
	if s.config.TokenCounter != nil {
		return s.config.TokenCounter
	}
	// 粗略估算：英文约 4 字符/token，中文约 1.5 字/token，取折中。
	return func(text string) uint64 {
		if text == "" {
			return 0
		}
		runes := len([]rune(text))
		if runes == len(text) {
			return uint64((len(text) + 3) / 4)
		}
		return uint64(runes/2 + 1)
	}
}

func (s *Session[M]) estimateTokens(messages []M) uint64 {
	return s.estimateTokensFrom(0, messages)
}

// estimateTokensFrom 估算历史自 from 起的总 token（含少量消息结构开销）。
func (s *Session[M]) estimateTokensFrom(from int, messages []M) uint64 {
	counter := s.tokenCounter()
	total := uint64(len(messages) - from) // 每条消息约 1 token 的结构开销
	for _, m := range messages[from:] {
		total += counter(s.adapter.MessageContent(m))
	}
	return total
}
