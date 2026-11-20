
## 内容合规差异（T-M7-09，双 flavor）

同一代码库，两个 SKU 靠构建参数区分（不 fork），由 `-ldflags "-X github.com/cjintao133-cell/star-river/kernel/compliance.currentFlavor=<flavor>"` 选择；默认 `international`。

| 维度 | international（海外，默认） | domestic（国内） |
|---|---|---|
| 敏感过滤 | standard | strict |
| 默认屏蔽分类 | nsfw | nsfw / violence / gambling |
| 默认界面语言 | en | zh |
| 举报入口 | 展示 | 展示（强制） |
| 视觉生成 | 开 | 开 |

- 内核侧 `kernel/compliance.ContentPolicy` 暴露默认策略与 `ReportEntryVisible` 位；举报入口 UI、双 SKU 真实构建产出属界面/构建层（押后）。平台能力位：`ReportEntryVisible` 两 flavor 均为 true（合规底线）。
