zsf
---
> zsf是一套http、grpc的golang后端服务框架  
> 意在提供服务治理和标准化代码模版  
> 在gin、grpc、consul、skywalking、prometheus、pprof、xorm等基础上扩展了新功能  
> 基础组件有协程池、广播组件、通用负载均衡组件等   
> 以consul作为服务发现、注册以及配置中心使用  
> logrus异步化写日志支持和kafka、nsq hook  
> kafka、nsq consumer封装  
> 实现api网关组件，可利用组件快速实现自定义api网关，支持mock请求返回结果，支持多种匹配策略，网关层支持http代理转发、负载均衡等   
> rpc、监控、限流、请求header传递、prometheus监控、go2sky接入等  
> xorm慢sql日志告警  
> 支持自定义流程编排规则zengine  
> 支持简单http和grpc反向代理，反向代理上可新增鉴权、限流等功能
>
1、通用负载均衡路由实现

```
轮询路由
加权平滑轮询路由
哈希路由 默认带crc32和murur3哈希
```

2、协程池

```
与java线程池思想类似，但不完全相同。主要用于控制协程资源的使用
参数包括：
1、poolSize 最大协程数量
2、ququeSize 但协程数量到达最大时，新增的任务会加入队列排队，允许排队的数量
3、expire 当协程空闲到达一定时间时，便回收协程
4、rejectHandler 拒绝操作 当协程池无法接收新任务时的拒绝策略，默认实现了两种策略。丢弃策略（AbortPolicy）和 调用者执行策略(CallerRunsPolicy)

可执行两种任务
1、runnable 纯任务 
2、PromiseFuture 是future + promise形式，可随意控制获取任务超时时间和返回结果，更精细把控任务的执行
```

3、本地事件广播

```
类似消息队列，可实现对一个topic进行广播
通常用于不同模块之间的通信
降低各模块的耦合

被用来配置key变更触发回调
grpc client服务发现ip列表变更触发回调
```

4、rpc服务

```
gin的http server端实现
gin + nhooyr扩展对websocket server的支持

grpc server的实现

实现prometheus对http、grpc请求的耗时频率监控
http和grpc之间的请求头传递
```

5、服务注册

```
实现了http和grpc server的服务注册并定时上报consul心跳
```

6、服务发现

```
实现http client和grpc client的服务发现  
配合通用负载均衡路由，实现了http client和grpc client的负载均衡  
轮询路由和加权轮询路由  
其中默认实现了按版本号路由，可用于灰度发布，优先发送请求给相同版本的服务，若没有，发送给其他版本服务
有实现以consul服务发现以及文件服务发现
```

7、prometheus server

```
专门为prometheus的抓取启动新的http server，端口默认是16005
```

8、viper多环境配置和consul配置中心

```
配置文件默认路径是./resources/application.yaml
可根据环境配置多个文件 例如./resources/application-sit.yaml

优先加载application.yaml, 其他application-sit.yaml会覆盖application.yaml

实现监听consul配置中心变化来更新本地配置

实现监听某个key变化触发回调功能
```

9、pprof server

```
必要时可以打开pprof server可分析程序 只能本地访问
```

10、go2sky接入

```
skywalking grpc上报
接入skywalking, 默认0.6采样率，可根据配置中心变更来动态调整
实现http->grpc、grpc->http、等链路skywalking的打通
```

11、限流熔断

```
sentinel做限流熔断
```

12、字段校验validator

```
魔改了一个开源库，使其能返回自定义错误信息
```

13、业务网关组件

```
支持全匹配、前缀匹配、表达式匹配，path重写策略
优先级：全匹配 > 前缀匹配 > 表达式匹配（非文本表达式，json格式表达）
表达式匹配：支持处理header、cookie、path、host等数据，操作符支持等于、不等于、为空、不为空等
转发协议仅支持http请求，demo可看apigw/demo/demo.go
转发目标支持服务发现、域名ip，支持多种负载均衡策略
mock请求返回结果, 支持任意http的statusCode，返回结果支持json和string两种类型
```

14、反向代理proxy

```
支持grpc和http的反向代理，在反向代理上可做限流鉴权等操作
```

15、流程编排引擎zengine

```
可通过配置，对流程节点进行编排，不用发版便可修改函数调用顺序
支持自定义节点和脚本节点
脚本节点使用gopher-lua
```
