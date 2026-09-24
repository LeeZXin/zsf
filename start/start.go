/*
Package start 提供启动钩子注册，是 quit 的启动侧镜像。

与 quit 的四级关闭钩子不同，启动钩子只有一条队列，靠 order 排序：
值越小越先执行，同值按注册顺序稳定保序。order 可省略（默认 10000，排在有显式 order 的钩子之后）。

顺序约定：zsf 框架自身的钩子全部使用负数 order（越基础越靠前，
如 env -10、static -9、logger -8），消费方（如 corelet）使用正数，
保证框架初始化永远先于业务初始化。

组件应在 init() 或 main 早期（lifecycle.Run 之前）通过 AddInit 注册。
lifecycle.Run 会在创建 PID 文件之后、各对象 OnApplicationStart 之前，
一次性快照并按序串行执行全部钩子（均在 main goroutine 内）。

钩子失败应自行 Fatal（fail-fast），框架不捕获不重试；AddInit 忽略 nil。
与 quit 相同：注册与快照有锁保护，GetInit 只读快照、不修改共享状态。
*/
package start

import (
	"slices"
	"sort"
	"sync"

	"github.com/LeeZXin/zsf/utils/listutil"
)

var (
	mu   sync.Mutex
	list []task
)

type task struct {
	fn    func()
	order int
}

// AddInit 注册一个启动初始化钩子：order 越小越先执行（省略时默认 10000），
// 同 order 按注册顺序稳定保序；nil 被忽略。
func AddInit(fn func(), order ...int) {
	if fn == nil {
		return
	}
	finalOrder := 10000
	if len(order) > 0 {
		finalOrder = order[0]
	}
	mu.Lock()
	list = append(list, task{
		fn:    fn,
		order: finalOrder,
	})
	mu.Unlock()
}

// GetInit 返回按 order 升序（同值稳定保序）排列的钩子快照，供 lifecycle.Run 调用。
// 只读快照，不修改已注册队列，可重复调用。
func GetInit() []func() {
	mu.Lock()
	tasks := slices.Clone(list)
	mu.Unlock()
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].order < tasks[j].order
	})
	return listutil.MapNe(tasks, func(t task) func() {
		return t.fn
	})
}
