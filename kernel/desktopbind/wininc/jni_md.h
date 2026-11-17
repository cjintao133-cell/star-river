/* 最小 Windows jni_md.h（mingw 交叉编译用）。
 * Linux JDK 只带 linux/jni_md.h；交叉 Windows 时以本文件替代。
 * 类型按 JNI 规范：Windows 上 jint=32 位 long，jlong=__int64，调用约定 __stdcall（x64 下无效化）。
 * M5 桌面工程落地时换正式 Windows JDK 头复核。
 */
#ifndef _JAVASOFT_JNI_MD_H_
#define _JAVASOFT_JNI_MD_H_
#define JNIEXPORT __declspec(dllexport)
#define JNIIMPORT __declspec(dllimport)
#define JNICALL __stdcall
typedef long jint;
typedef __int64 jlong;
typedef signed char jbyte;
#endif
