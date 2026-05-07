# 短篇中文架构知识 Fixture Sources

供 I1-T7 公开复杂文档语料回归测试使用的短篇单话题架构知识文档（中文）。

## Upstream

- 仓库：[apache/dubbo-website](https://github.com/apache/dubbo-website)
- 路径：`content/zh-cn/docs/{concepts,advanced,references}/...`
- 抓取 commit：`19e75d7c79c2a7fcf13477d47aac8a0867ea704c`
- Commit 日期：2026-04-22
- Fetch 日期:2026-05-07
- 上游许可证：Apache-2.0（见 `LICENSE`）

抓取方式：`git clone --depth=1`，挑选 2-12KB 的单话题中文文档（核心概念、高级特性、参考手册条目），重命名为 `<分类>-<slug>.md` 以体现归属。

## Files

| 文件 | 上游 URL |
| --- | --- |
| `concepts-traffic-management.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/concepts/traffic-management.md |
| `concepts-extensibility.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/concepts/extensibility.md |
| `concepts-configuration.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/concepts/configuration.md |
| `concepts-service-discovery.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/concepts/service-discovery.md |
| `concepts-rpc-protocol.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/concepts/rpc-protocol.md |
| `concepts-registry-configcenter-metadata.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/concepts/registry-configcenter-metadata.md |
| `advanced-callback-parameter.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/advanced/callback-parameter.md |
| `advanced-local-mock.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/advanced/local-mock.md |
| `advanced-events-notify.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/advanced/events-notify.md |
| `advanced-async-call.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/advanced/async-call.md |
| `advanced-auth.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/advanced/auth.md |
| `advanced-concurrency-control.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/advanced/concurrency-control.md |
| `references-qos.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/references/qos.md |
| `references-telnet.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/references/telnet.md |
| `references-spis-filter.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/docs/references/spis/filter.md |
