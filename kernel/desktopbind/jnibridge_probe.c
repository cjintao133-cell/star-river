/* JNI 薄桥（T-M0-07）：deviceProfile native 方法 → Go 导出 SrDeviceProfile。
 * 内存契约：Go 返回 malloc 字符串，本层 free；无入参，无 GetStringUTFChars 路径。
 * DeviceProfile JSON 全 ASCII（版本号/布尔/整数），NewStringUTF 无 Modified UTF-8 风险。
 * REVIEW-NEEDED：手写 cgo/JNI 指针边界（红线 7）。jnibridge.c 挂起评审中，本文件独立新增。
 */
#include <jni.h>
#include <stdlib.h>

extern char* SrDeviceProfile(void);

JNIEXPORT jstring JNICALL
Java_com_starriver_desktop_KernelBridge_deviceProfile(JNIEnv* env, jclass cls)
{
    (void)cls;
    char* p = SrDeviceProfile();
    if (p == NULL) return NULL;
    jstring out = (*env)->NewStringUTF(env, p);
    free(p);
    return out;
}
