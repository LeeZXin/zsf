package prom

import "github.com/prometheus/client_golang/prometheus"

// prometheus请求监控
// grpcServer、grpcClient、httpServer、httpClient

var (
	GrpcClientUnaryRequestTotal = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "grpc_client_unary_request_total",
		Help: "grpc client unary request summary",
	}, []string{"request"})

	GrpcServerUnaryRequestTotal = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "grpc_server_unary_request_total",
		Help: "grpc server unary request summary",
	}, []string{"request"})

	GrpcClientStreamRequestTotal = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "grpc_client_stream_request_total",
		Help: "grpc client unary request summary",
	}, []string{"request"})

	GrpcServerStreamRequestTotal = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "grpc_server_stream_request_total",
		Help: "grpc server unary request summary",
	}, []string{"request"})

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
	prometheus.MustRegister(GrpcClientUnaryRequestTotal)
	prometheus.MustRegister(GrpcClientStreamRequestTotal)
	prometheus.MustRegister(HttpClientRequestTotal)
	prometheus.MustRegister(GrpcServerUnaryRequestTotal)
	prometheus.MustRegister(GrpcServerStreamRequestTotal)
	prometheus.MustRegister(HttpServerRequestTotal)
}
