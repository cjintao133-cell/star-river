// Package proactive 是主动能力调度器（能力插件包，T-M5-01）。
//
// 依赖方向（红线 2）：底座不得 import 本包；本包可 import 底座开源包（core/clock）。
// 本包**不 import 其他插件包**（hippocampus/endocrine）：记忆到期条目与情绪状态由
// 应用层以只读输入（ReviewItem / EmotionState）喂入，调度判定住在本包内，接线在组合根。
//
// 三类触发源（数据驱动、可插拔，见 Source 接口）：
//   - memory_review：记忆复习到期（NextReviewAt<=now），提醒复习/回访。
//   - schedule：人设日程（每天 HH:MM 起算的窗口 + 周几过滤），如"催吃饭/晚安"。
//   - emotion：情绪驱动（对 endocrine 维度值做阈值规则），如"疲劳高→劝休息"。
//
// 三重闸门（主动能力的可撤回权限：默认关闭、静默时段、频率上限、停止开关）：
//  1. 总开关 Enabled + 按源开关（SetSourceEnabled），出厂默认关闭（New(Config{}) 即关闭）。
//  2. 静默时段 QuietHours（支持跨零点）：非紧急候选延后，不占用去重/限频额度，静默结束后可再评估。
//  3. 频率限制（滚动窗口 MaxPerWindow + 每日 DailyMax）+ 每候选冷却去重（按 Key 维度）。
//
// 时钟经 core/clock 注入（T-M3-06），全逻辑在假时钟上确定性可测；Snapshot/Restore 支持重启恢复。
package proactive
