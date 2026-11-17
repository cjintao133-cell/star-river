/* JNI 薄桥：Kotlin/Java native 方法 → Go c-shared 导出（SrVersion/SrEcho）。
 * 内存契约：Go 返回 malloc 字符串，本层 free；jstring UTF 缓冲用后 Release。
 * REVIEW-NEEDED：手写 cgo/JNI 指针边界（红线 7）。
 */
#include <jni.h>
#include <stdlib.h>

extern char* SrVersion(void);
extern char* SrEcho(char* s);

JNIEXPORT jstring JNICALL
Java_com_starriver_desktop_KernelBridge_version(JNIEnv* env, jclass cls)
{
    (void)cls;
    char* v = SrVersion();
    if (v == NULL) return NULL;
    jstring out = (*env)->NewStringUTF(env, v);
    free(v);
    return out;
}

JNIEXPORT jstring JNICALL
Java_com_starriver_desktop_KernelBridge_echo(JNIEnv* env, jclass cls, jstring s)
{
    (void)cls;
    if (s == NULL) return NULL;
    const char* in = (*env)->GetStringUTFChars(env, s, NULL);
    if (in == NULL) return NULL; /* OOM，JVM 已挂异常 */
    char* out = SrEcho((char*)in);
    (*env)->ReleaseStringUTFChars(env, s, in);
    if (out == NULL) return NULL;
    jstring result = (*env)->NewStringUTF(env, out);
    free(out);
    return result;
}
