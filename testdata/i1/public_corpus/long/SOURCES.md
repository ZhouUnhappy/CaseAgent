# 长篇中文设计文档 Fixture Sources

供 I1-T7 公开复杂文档语料回归测试使用的长篇设计/分析文档（中文）。

## Upstream

- 仓库：[apache/dubbo-website](https://github.com/apache/dubbo-website)
- 路径：`content/zh-cn/blog/...`（codeanalysis 系列、proposals、integration 等长文）
- 抓取 commit：`19e75d7c79c2a7fcf13477d47aac8a0867ea704c`
- Commit 日期：2026-04-22
- Fetch 日期：2026-05-07
- 上游许可证：Apache-2.0（见 `LICENSE`）

抓取方式：`git clone --depth=1`，挑选篇幅 27-50KB 的中文长文（源码分析 / proposal / 集成方案），重命名为 ASCII slug 以避免上传时编码问题。原文标题保留在 markdown 内。

## Files

| 文件 | 上游 URL |
| --- | --- |
| `dubbo-provider-dual-register.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/blog/java/codeanalysis/3.0.8/17-Dubbo%E6%9C%8D%E5%8A%A1%E6%8F%90%E4%BE%9B%E8%80%85%E7%9A%84%E5%8F%8C%E6%B3%A8%E5%86%8C%E5%8E%9F%E7%90%86.md |
| `dubbo-module-publisher.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/blog/java/codeanalysis/3.0.8/16-%E6%A8%A1%E5%9D%97%E5%8F%91%E5%B8%83%E5%99%A8%E5%8F%91%E5%B8%83%E6%9C%8D%E5%8A%A1%E5%85%A8%E8%BF%87%E7%A8%8B.md |
| `dubbo-domain-model-init.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/blog/java/codeanalysis/3.0.8/3-%E6%A1%86%E6%9E%B6%2C%E5%BA%94%E7%94%A8%E7%A8%8B%E5%BA%8F%2C%E6%A8%A1%E5%9D%97%E9%A2%86%E5%9F%9F%E6%A8%A1%E5%9E%8BModel%E5%AF%B9%E8%B1%A1%E7%9A%84%E5%88%9D%E5%A7%8B%E5%8C%96.md |
| `dubbo-service-weaver-paper.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/blog/proposals/google-service-weaver-paper-2023.md |
| `dubbo-metrics-proposal.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/blog/proposals/metrics.md |
| `dubbo-zipkin-integration.md` | https://github.com/apache/dubbo-website/blob/19e75d7c79c2a7fcf13477d47aac8a0867ea704c/content/zh-cn/blog/integration/use-zipkin-in-dubbo.md |
