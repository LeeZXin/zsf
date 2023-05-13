package skywalking

const (
	ComponentIDGOHttpServer       = 60001
	ComponentIDGOHttpClient       = 60002
	ComponentIDGOGrpcUnaryServer  = 60003
	ComponentIDGOGrpcUnaryClient  = 60004
	ComponentIDGOGrpcStreamServer = 60005
	ComponentIDGOGrpcStreamClient = 60006

	TagGrpcMethod = "grpc.method"
	TagRpcScheme  = "rpc.scheme"
	TagGrpcScheme = "grpc"
	TagHttpScheme = "http"
)
