// Package timeutil 提供时间计算工具：各周期（小时/天/周/月/年）起止点计算、
// 周期区间枚举、时间截断（到整日/整分/整时）与周内序号计算。
// 所有函数均保留入参 t 的时区。
package timeutil

import (
	"time"
)

// GetStartOfHour 返回 t 所在小时的起点（分钟、秒、纳秒均为 0），时区与 t 一致。
func GetStartOfHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
}

// GetEndOfHour 返回 t 所在小时的最后一纳秒（即下一小时起点减 1ns），时区与 t 一致。
func GetEndOfHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, -1, t.Location())
}

// GetStartOfDay 返回 t 所在日期的 00:00，时区与 t 一致。
func GetStartOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// GetEndOfDay 返回 t 所在日期的最后一纳秒（即次日 00:00 减 1ns），时区与 t 一致。
func GetEndOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, -1, t.Location())
}

// GetStartOfMonth 返回 t 所在月份的 1 日 00:00，时区与 t 一致。
func GetStartOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// GetEndOfMonth 返回 t 所在月份的最后一纳秒（即下月 1 日 00:00 减 1ns），时区与 t 一致。
func GetEndOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, -1, t.Location())
}

// GetStartOfWeek 返回 t 所在周的周一 00:00。
// 注意：以周一为一周起点，周日视为第 7 天（中国习惯）。
func GetStartOfWeek(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 { // 周日
		// 中国算7
		wd = 7
	}
	return GetStartOfDay(t.AddDate(0, 0, 1-wd))
}

// GetEndOfWeek 返回 t 所在周的周日最后一纳秒（本周结束点），时区与 t 一致。
// 注意：以周一为一周起点，周日视为第 7 天（中国习惯）。
func GetEndOfWeek(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 { // 周日
		// 中国算7
		wd = 7
	}
	return GetEndOfDay(t.AddDate(0, 0, 7-wd))
}

// GetStartOfYear 返回 t 所在年份的 1 月 1 日 00:00，时区与 t 一致。
func GetStartOfYear(t time.Time) time.Time {
	return time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, t.Location())
}

// GetEndOfYear 返回 t 所在年份的最后一纳秒（即次年 1 月 1 日 00:00 减 1ns），时区与 t 一致。
func GetEndOfYear(t time.Time) time.Time {
	return GetStartOfYear(t).AddDate(1, 0, 0).Add(-time.Nanosecond)
}

// ListMonthsOfYear 返回 t 所在年份 1 月至 12 月的第一天，共 12 个时间点，时区与 t 一致。
func ListMonthsOfYear(t time.Time) []time.Time {
	ret := make([]time.Time, 0, 12)
	start := GetStartOfYear(t)
	ret = append(ret, start)
	for i := 1; i < 12; i++ {
		ret = append(ret, start.AddDate(0, i, 0))
	}
	return ret
}

// ListDaysBetweenTime 返回 t1 与 t2 之间（含两端）的每一天 00:00，结果按时间升序。
// t1、t2 的先后顺序不限，函数内部会各自归一化到当天 00:00 并自动调整顺序。
func ListDaysBetweenTime(t1 time.Time, t2 time.Time) []time.Time {
	t1 = GetStartOfDay(t1)
	t2 = GetStartOfDay(t2)
	if t1.After(t2) {
		t1, t2 = t2, t1
	}
	ret := make([]time.Time, 0, t2.Sub(t1)/(time.Hour*24))
	for !t1.Equal(t2) {
		ret = append(ret, t1)
		t1 = t1.AddDate(0, 0, 1)
	}
	ret = append(ret, t2)
	return ret
}

// ListMonthsBetweenTime 返回 t1 与 t2 之间（含两端）的每个月第一天。
// 注意：若 t1 > t2，结果按时间降序排列；t1 与 t2 同年同月时仅返回一个元素。
func ListMonthsBetweenTime(t1 time.Time, t2 time.Time) []time.Time {
	s1 := GetStartOfMonth(t1)
	s2 := GetStartOfMonth(t2)
	ret := make([]time.Time, 0)
	if s1.Before(s2) {
		for !s1.Equal(s2) {
			ret = append(ret, s1)
			s1 = s1.AddDate(0, 1, 0)
		}
		ret = append(ret, s2)
	} else if s1.After(s2) {
		for !s1.Equal(s2) {
			ret = append(ret, s1)
			s1 = s1.AddDate(0, -1, 0)
		}
		ret = append(ret, s2)
	} else {
		ret = append(ret, s1)
	}
	return ret
}

// ListYearsBetweenTime 返回 t1 与 t2 之间（含两端）的每年 1 月 1 日。
// 注意：若 t1 > t2，结果按时间降序排列；t1 与 t2 同年时仅返回一个元素。
func ListYearsBetweenTime(t1 time.Time, t2 time.Time) []time.Time {
	s1 := GetStartOfYear(t1)
	s2 := GetStartOfYear(t2)
	ret := make([]time.Time, 0)
	if s1.Before(s2) {
		for !s1.Equal(s2) {
			ret = append(ret, s1)
			s1 = s1.AddDate(1, 0, 0)
		}
		ret = append(ret, s2)
	} else if s1.After(s2) {
		for !s1.Equal(s2) {
			ret = append(ret, s1)
			s1 = s1.AddDate(-1, 0, 0)
		}
		ret = append(ret, s2)
	} else {
		ret = append(ret, s1)
	}
	return ret
}

// FromMonth 返回 t 所在年份中指定 month 的 1 日 00:00，时区与 t 一致。
func FromMonth(t time.Time, month time.Month) time.Time {
	return time.Date(t.Year(), month, 1, 0, 0, 0, 0, t.Location())
}
