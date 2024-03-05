package httpclient

var (
	interceptors = make([]Interceptor, 0)
)

func RegisterInterceptors(is ...Interceptor) {
	if len(is) == 0 {
		return
	}
	interceptors = append(interceptors, is...)
}

func getInterceptors() []Interceptor {
	return interceptors[:]
}
