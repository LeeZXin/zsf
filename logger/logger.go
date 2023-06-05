package logger

import (
	"bytes"
	"fmt"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
	"path"
)

// 日志logrus格式封装
// grpc logger封装

type vLogger struct {
	*logrus.Logger
}

func (*vLogger) V(l int) bool {
	return true
}

var (
	Logger *vLogger
)

type logFormatter struct {
}

// Format 格式化
func (l *logFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	buffer := entry.Buffer
	if buffer == nil {
		buffer = &bytes.Buffer{}
	}
	traceId := "-"
	if entry.Context != nil {
		mdc := GetMDC(entry.Context)
		t := mdc.Get(TraceId)
		if t != "" {
			traceId = t
		}
	}
	ts := entry.Time.Format("2006-01-02 15:04:05.000")
	logStr := fmt.Sprintf("%s [%s] [%s:%d] [%s] %s\n", ts, entry.Level, path.Base(entry.Caller.File), entry.Caller.Line, traceId, entry.Message)
	buffer.WriteString(logStr)
	return buffer.Bytes(), nil
}

func init() {
	Logger = &vLogger{Logger: logrus.New()}
	Logger.SetReportCaller(true)
	Logger.SetFormatter(&logFormatter{})
	Logger.SetLevel(logrus.InfoLevel)
	Logger.SetOutput(io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename:   "./logs/application.log", //日志文件位置
		MaxSize:    100,                      // 单文件最大容量,单位是MB
		MaxBackups: 0,                        // 最大保留过期文件个数
		MaxAge:     1,                        // 保留过期文件的最大时间间隔,单位是天
		Compress:   true,                     // 是否需要压缩滚动日志, 使用的 gzip 压缩
	}))
}
