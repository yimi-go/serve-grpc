serve-grpc
===
提供gPRC服务

* gRPC server runner
* gPRC server interceptors
    * [x] Recover
    * [x] Logging
    * [X] Authentication by JWT
      > Note: 只是认证，没有鉴权。即使 JWT 合法，可能请求也没有权限操作。
      > 鉴权逻辑与业务相关应由业务方实现。
    * [x] Metrics
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
  > 建议注册顺序（执行顺序）：
  > * Tracing: Logging 和 Metrics 要用到
  > * Logging：必须在 Recover 外层，防止 panic 漏打日志
  > * Metrics：必须在 Recover 外层，防止 panic 漏报数据
  > * Recover：其他的不依赖 recover 了，要在这个位置兜底
  > * Metadata propagation
  > * JWT：Rate Limit 和 Validate 可能用到 Token 里的数据
  > * 业务鉴权：必须先认证；Validate 可能用到这里产生的数据
  > * Rate Limit：如果在这里拦截，后续不需要费力计算。
  > * Validate