package timeutil

import "time"

var (
	// Forever 表示"永不失效"的远期时间常量（2199-12-31，本地时区），
	// 用于与过期时间比较或作为时间占位值。
	Forever = time.Date(2199, time.December, 31, 0, 0, 0, 0, time.Local)
)

// GetWeekday 周日变7
func GetWeekday(t time.Time) int {
	weekday := t.Weekday()
	if weekday == time.Sunday {
		return 7
	}
	return int(weekday)
}

// ToDateUnix 将 t 截断到当天 00:00（保留 t 的时区）后返回 Unix 时间戳。
func ToDateUnix(t time.Time) int64 {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location()).Unix()
}

// ToMinuteUnix 将 t 截断到整分（秒与纳秒清零，保留 t 的时区）后返回 Unix 时间戳。
func ToMinuteUnix(t time.Time) int64 {
	year, month, day := t.Date()
	return time.Date(year, month, day, t.Hour(), t.Minute(), 0, 0, t.Location()).Unix()
}

// ToHourUnix 将 t 截断到整点（分、秒、纳秒清零，保留 t 的时区）后返回 Unix 时间戳。
func ToHourUnix(t time.Time) int64 {
	year, month, day := t.Date()
	return time.Date(year, month, day, t.Hour(), 0, 0, 0, t.Location()).Unix()
}
