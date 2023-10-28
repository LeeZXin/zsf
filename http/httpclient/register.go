package httpclient

import "sync"

var (
	interceptors   = make([]Interceptor, 0)
	interceptorsMu = sync.Mutex{}
)

func RegisterInterceptors(f ...Interceptor) {
	if len(f) == 0 {
		return
	}
	interceptorsMu.Lock()
	defer interceptorsMu.Unlock()
	interceptors = append(interceptors, f...)
}

func getInterceptors() []Interceptor {
	interceptorsMu.Lock()
	defer interceptorsMu.Unlock()
	return interceptors[:]
}
