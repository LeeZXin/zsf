package httpclient

import "sync"

var (
	interceptors = make([]Interceptor, 0)
	imu          = sync.Mutex{}
)

func RegisterInterceptors(is ...Interceptor) {
	if len(is) == 0 {
		return
	}
	imu.Lock()
	defer imu.Unlock()
	interceptors = append(interceptors, is...)
}

func getInterceptors() []Interceptor {
	imu.Lock()
	defer imu.Unlock()
	return interceptors[:]
}
