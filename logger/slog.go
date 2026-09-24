package logger

import (
	"context"
	"log/slog"
	"strings"

	"github.com/rs/zerolog"
)

/*
SlogHandler 将 slog.Record 转换为 zerolog 事件。

字段：
  - attrs:  WithAttrs 累积的日志字段（每级 WithAttrs 产生新实例）
  - groups: WithGroup 累积的组名，作为字段键前缀
*/
type SlogHandler struct {
	attrs  []slog.Attr
	groups []string
}

/*
Enabled 判断级别是否开启：slog 级别换算为 zerolog 级别后与全局级别比较。

级别数值越大越严重，zerolog 级别枚举满足 >= 比较（Debug=0 < Info=1 < Warn=2 < Error=3）。
*/
func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return convertSlogLevel(level) >= zerolog.GlobalLevel()
}

/*
Handle 输出一条 slog 记录：经 Ctx(ctx) 取带 traceId 的 Logger，
先写入 WithAttrs 累积字段，再写入本次记录字段，最后以 r.Message 输出。
*/
func (h *SlogHandler) Handle(ctx context.Context, r slog.Record) error {
	evt := Ctx(ctx).WithLevel(convertSlogLevel(r.Level))
	for _, a := range h.attrs {
		appendAttr(evt, h.groups, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		appendAttr(evt, h.groups, a)
		return true
	})
	evt.Msg(r.Message)
	return nil
}

/*
WithAttrs 返回携带指定字段的新 handler，不修改原实例。

注意：底层切片必须拷贝，避免后续 append 覆盖共享 backing array。
*/
func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	h2 := *h
	h2.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &h2
}

/*
WithGroup 返回追加组名的新 handler，组名作为后续字段键的前缀。
*/
func (h *SlogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.groups = append(append([]string{}, h.groups...), name)
	return &h2
}

/*
appendAttr 将单个 slog.Attr 写入 zerolog 事件。

  - LogValuer：先 Resolve 再按解析后的 Kind 处理
  - 空 Attr（键与值均为零值）：按 slog.Handler 契约忽略，在 Resolve 之后判断
  - Group：递归展平，Key 为空（内联组）时不追加前缀
  - 其余 Kind 转为 zerolog 对应类型字段，保证 Loki/JSON 输出类型正确
*/
func appendAttr(e *zerolog.Event, groups []string, a slog.Attr) {
	if a.Value.Kind() == slog.KindLogValuer {
		a = slog.Attr{Key: a.Key, Value: a.Value.Resolve()}
	}
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		gs := groups
		if a.Key != "" {
			gs = append(append([]string{}, groups...), a.Key)
		}
		for _, sub := range a.Value.Group() {
			appendAttr(e, gs, sub)
		}
		return
	}

	key := a.Key
	if len(groups) > 0 {
		key = strings.Join(append(append([]string{}, groups...), a.Key), ".")
	}
	switch a.Value.Kind() {
	case slog.KindBool:
		e.Bool(key, a.Value.Bool())
	case slog.KindInt64:
		e.Int64(key, a.Value.Int64())
	case slog.KindUint64:
		e.Uint64(key, a.Value.Uint64())
	case slog.KindFloat64:
		e.Float64(key, a.Value.Float64())
	case slog.KindString:
		e.Str(key, a.Value.String())
	case slog.KindDuration:
		e.Dur(key, a.Value.Duration())
	case slog.KindTime:
		e.Time(key, a.Value.Time())
	default: // KindAny
		e.Interface(key, a.Value.Any())
	}
}

/*
convertSlogLevel 将 slog 级别映射为 zerolog 级别。

slog 级别为连续数值（Debug=-4 Info=0 Warn=4 Error=8），
按区间落入对应 zerolog 级别；高于 Error 的自定义级别统一按 Error 处理。
*/
func convertSlogLevel(level slog.Level) zerolog.Level {
	switch {
	case level < slog.LevelInfo:
		return zerolog.DebugLevel
	case level < slog.LevelWarn:
		return zerolog.InfoLevel
	case level < slog.LevelError:
		return zerolog.WarnLevel
	default:
		return zerolog.ErrorLevel
	}
}
