package starter

import (
	_ "github.com/LeeZXin/zsf/actuator"
	_ "github.com/LeeZXin/zsf/http/httpserver"
	_ "github.com/LeeZXin/zsf/pprof"
	_ "github.com/LeeZXin/zsf/prom"
	"github.com/LeeZXin/zsf/zsf"
)

func Run(options ...zsf.Option) {
	zsf.Run(options...)
}
