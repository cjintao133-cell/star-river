// Package backup 是数据导出/导入（T-M7-07）：
// 把本地 SQLite 的逻辑内容（会话/消息/模型统计/应用配置）打包成可携带的 zip，
// 带格式版本 + 逐文件 SHA-256 完整性校验，可导入到空库实现"导出→清库→导入"往返一致。
//
// 依赖方向（红线 2）：本包为内核底座，只 import 标准库与 kernel/store（同为开源底座）。
// 不 import 任何能力插件包。key 的加密备份见 kernel/secret（EncryptBackup/DecryptBackup），
// 本包只负责把（已加密的）key blob 作为额外文件原样纳入归档 / 取出，不接触明文密钥。
//
// 包内不含加密逻辑（那是 secret 的 REVIEW-NEEDED 区）；本包仅做搬运与完整性校验。
package backup
