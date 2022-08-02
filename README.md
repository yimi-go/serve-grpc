serve-grpc
===
提供gPRC服务

* gRPC server runner
* gPRC server interceptors
  * [x] Recover
  * [x] Logging
  * [ ] Authentication by JWT
  * [ ] Metrics
  * [ ] Rate Limit
    * [ ] BBR
      * 被动限流
      * 用于服务端自保
    * [ ] APF
      * 参考 K8s APF。进行主动限流
      * 用于阻止"坏用户"
  * [ ] Tracing
  * [ ] Validate
    * [ ] Basic 基于数据规则的校验
    * [ ] Contextual or Business 基于请求上下文业务数据要求的校验
  * [ ] Metadata propagation
    请求元数据传递，从收到的请求中提取，在之后处理中携带，并在需要请求下一依赖服务时附加。
