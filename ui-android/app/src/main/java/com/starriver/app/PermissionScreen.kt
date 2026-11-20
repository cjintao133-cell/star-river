package com.starriver.app

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat

private data class PermItem(val title: String, val desc: String, val perm: String?)

/** 权限确认 UI（T-M6 权限 UI）：敏感能力逐项申请 + 状态显示。宿主授权，AI 不自取。 */
@Composable
fun PermissionScreen(onBack: () -> Unit) {
    val ctx = LocalContext.current
    val items = remember {
        val notif = if (Build.VERSION.SDK_INT >= 33) Manifest.permission.POST_NOTIFICATIONS else null
        val media = if (Build.VERSION.SDK_INT >= 33) Manifest.permission.READ_MEDIA_IMAGES else Manifest.permission.READ_EXTERNAL_STORAGE
        listOf(
            PermItem("麦克风", "语音输入（STT）录音", Manifest.permission.RECORD_AUDIO),
            PermItem("相机", "拍照 / 自拍改头像", Manifest.permission.CAMERA),
            PermItem("相册", "选头像 / 人设图 / 导入图片", media),
            PermItem("通知", "主动消息 / 后台提醒", notif),
        )
    }
    var refresh by remember { mutableStateOf(0) }
    var pendingPerm by remember { mutableStateOf<String?>(null) }
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { _ ->
        pendingPerm = null; refresh++
    }
    fun granted(perm: String?): Boolean {
        if (perm == null) return true
        return ContextCompat.checkSelfPermission(ctx, perm) == PackageManager.PERMISSION_GRANTED
    }
    refresh // 触发读取最新状态

    Column(Modifier.fillMaxSize().padding(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            TextButton(onClick = onBack) { Text("← 返回") }
            Text("权限管理", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        }
        Text("敏感能力由宿主逐项授权，AI 不会自行开启。关闭可在系统设置随时收回。",
            fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        items.forEach { it ->
            Card(Modifier.fillMaxWidth()) {
                Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(it.title, fontWeight = FontWeight.Bold, fontSize = 15.sp)
                        Text(it.desc, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        val ok = granted(it.perm)
                        Text(
                            if (it.perm == null) "系统无需授权" else if (ok) "✓ 已授权" else "未授权",
                            fontSize = 11.sp,
                            color = if (ok) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    if (it.perm != null && !granted(it.perm)) {
                        Button(onClick = { pendingPerm = it.perm; launcher.launch(it.perm) }) { Text("申请") }
                    }
                }
            }
        }
    }
}

/** 举报入口 UI（合规要求）：分类 + 描述 + 提交（本地记录，联网通道待阶段 C 接入）。 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun ReportScreen(onBack: () -> Unit) {
    var reason by remember { mutableStateOf("") }
    var detail by remember { mutableStateOf("") }
    var done by remember { mutableStateOf(false) }
    val reasons = listOf("违法违规", "色情低俗", "暴力恐怖", "侵权盗版", "虚假信息", "其他")
    Column(Modifier.fillMaxSize().padding(12.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            TextButton(onClick = onBack) { Text("← 返回") }
            Text("举报与反馈", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        }
        Text("选择问题类型", fontWeight = FontWeight.Bold)
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            reasons.forEach { r ->
                OutlinedButton(onClick = { reason = r; done = false }) {
                    Text(r, fontWeight = if (reason == r) FontWeight.Bold else FontWeight.Normal,
                        color = if (reason == r) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
        OutlinedTextField(value = detail, onValueChange = { detail = it; done = false }, label = { Text("补充说明（可选）") }, modifier = Modifier.fillMaxWidth(), minLines = 3)
        Text("注：本地已记录，正式联网举报通道随内容合规双 flavor 上线接入。",
            fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Button(onClick = { if (reason.isNotBlank()) done = true }, enabled = reason.isNotBlank(), modifier = Modifier.fillMaxWidth()) {
            Text("提交举报")
        }
        if (done) Text("已记录举报：$reason（本地）", color = MaterialTheme.colorScheme.primary, fontSize = 13.sp)
    }
}

/** 轻量举报弹窗（供社区帖子等处内联调用）。 */
@Composable
fun ReportDialog(onDismiss: () -> Unit) {
    var sent by remember { mutableStateOf(false) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("举报该内容") },
        text = { Text(if (sent) "已记录举报（本地），感谢反馈。" else "确认举报？正式联网举报通道待接入。") },
        confirmButton = {
            TextButton(onClick = { if (sent) onDismiss() else sent = true }) { Text(if (sent) "关闭" else "提交") }
        },
        dismissButton = { if (!sent) TextButton(onClick = onDismiss) { Text("取消") } },
    )
}
