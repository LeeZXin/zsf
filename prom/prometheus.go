package prom

import "github.com/prometheus/client_golang/prometheus"

// prometheus请求监控
// httpServer、httpClient

var (
	HttpClientRequestTotal = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "http_client_request_total",
		Help: "http client request summary",
	}, []string{"request"})

	HttpServerRequestTotal = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "http_server_request_total",
		Help: "http server request summary",
	}, []string{"request"})
)

func init() {
	prometheus.MustRegister(HttpClientRequestTotal)
	prometheus.MustRegister(HttpServerRequestTotal)
}
