package nacosutil

import (
	"fmt"

	"github.com/LeeZXin/zsf/logger"
)

type nlogger struct {
}

func (*nlogger) Info(args ...any) {
	logger.Logger.Info().Msg(fmt.Sprint(args...))
}

func (*nlogger) Warn(args ...any) {
	logger.Logger.Warn().Msg(fmt.Sprint(args...))
}

func (*nlogger) Error(args ...any) {
	logger.Logger.Error().Msg(fmt.Sprint(args...))
}

func (*nlogger) Debug(args ...any) {
	logger.Logger.Debug().Msg(fmt.Sprint(args...))
}

func (*nlogger) Infof(fmt string, args ...any) {
	logger.Logger.Info().Msgf(fmt, args...)
}

func (*nlogger) Warnf(fmt string, args ...any) {
	logger.Logger.Warn().Msgf(fmt, args...)
}

func (*nlogger) Errorf(fmt string, args ...any) {
	logger.Logger.Error().Msgf(fmt, args...)
}

func (*nlogger) Debugf(fmt string, args ...any) {
	logger.Logger.Debug().Msgf(fmt, args...)
}

func (*nlogger) Close() error {
	return nil
}
