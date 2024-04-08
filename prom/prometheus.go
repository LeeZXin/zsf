package prom

import "github.com/prometheus/client_golang/prometheus"

// prometheus请求监控
// httpServer、httpClient

var (
	HttpClientRequestTotal = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "http_client_request_total",
		Help: "http client request summary",
	}, []string{"target", "request", "code"})

	HttpServerRequestTotal = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "http_server_request_total",
		Help: "http server request summary",
	}, []string{"request", "code"})
)

func init() {
	prometheus.MustRegister(HttpClientRequestTotal)
	prometheus.MustRegister(HttpServerRequestTotal)
}
