package tree

import (
	"fmt"
	"strings"
	"time"
)

const (
	failTemplate = "%s的配置特征是:%s %s, 实际特征是%s;"
)

// AnalyseResult 特征树解析结果
type AnalyseResult interface {
	IsSuccess() bool
	GetMissResultDetailDesc() string
}

// SuccessMetricsResult 成功的结果
type SuccessMetricsResult struct {
	*AnalyseMetrics
}

func (s SuccessMetricsResult) IsSuccess() bool {
	return true
}

func (s SuccessMetricsResult) GetMissResultDetailDesc() string {
	return "success"
}

// FailMetricsResult 失败的结果
type FailMetricsResult struct {
	*AnalyseMetrics
	AnalyseDetails []*AnalyseDetail
}

func (s *FailMetricsResult) IsSuccess() bool {
	return false
}

func (s *FailMetricsResult) GetMissResultDetailDesc() string {
	if s.AnalyseDetails != nil && len(s.AnalyseDetails) > 0 {
		stringBuilder := strings.Builder{}
		for _, detail := range s.AnalyseDetails {
			stringBuilder.WriteString(
				fmt.Sprintf(
					failTemplate,
					detail.FeatureName,
					detail.Operator.Alias,
					detail.ExpectResult,
					fmt.Sprint(detail.Result),
				),
			)
		}
		return stringBuilder.String()
	}
	return "unknown reason"
}

// TimeoutMetricsResult 超时结果
type TimeoutMetricsResult struct {
	*AnalyseMetrics
	Timeout time.Duration
}

func (s *TimeoutMetricsResult) IsSuccess() bool {
	return false
}

func (s *TimeoutMetricsResult) GetMissResultDetailDesc() string {
	return fmt.Sprintf("execute timeout: %v", s.Timeout)
}

// ErrMetricsResult 异常结果
type ErrMetricsResult struct {
	*AnalyseMetrics
	Err error
}

func (s *ErrMetricsResult) IsSuccess() bool {
	return false
}

func (s *ErrMetricsResult) GetMissResultDetailDesc() string {
	return fmt.Sprintf("execute err: %s", fmt.Sprint(s.Err))
}

// CancelMetricsResult 被迫取消结果
type CancelMetricsResult struct {
	*AnalyseMetrics
}

func (s *CancelMetricsResult) IsSuccess() bool {
	return false
}

func (s *CancelMetricsResult) GetMissResultDetailDesc() string {
	return "execute canceled for some reason"
}
