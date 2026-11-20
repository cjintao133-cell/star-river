// Package visual 是视觉管线（T-M6-04）：视觉配方 schema + 生成适配器（ComfyUI / OpenAI 兼容）
// + 异步任务队列 + 画廊 + **框架级不可关的 AI 标识水印**。
//
// 依赖方向：本包为内核私有区（未定，暂私有；非能力插件包），只 import 标准库与 core 底座；
// 不 import 任何能力插件包（红线 2）。
//
// AI 标识水印（不可关）：所有生成物在进入画廊/对外暴露前**必须**经 Watermarker.Mark 处理；
// Service 构造时强制注入 Watermarker，类型上不暴露"关闭"开关，并有测试卡死"无法绕过"。
// 水印实现（纯标准库，无第三方/字体依赖）：
//   - 可见层：把图像重绘到 RGBA，叠加稀疏半透明方块图样（肉眼可见的溯源标记）。
//   - 机器层：在 PNG 中注入 tEXt 块（keyword=Software, value 含 "AIGC"），可被 HasAIWatermark 检出。
//
// 更强的文字水印需字体依赖（红线 1 需单独报批）或 native 图像模块，留待后续，不在本任务偷偷引入。
package visual
