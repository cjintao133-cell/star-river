import org.jetbrains.compose.desktop.application.dsl.TargetFormat

// G0-PREP：最小 Compose Multiplatform 桌面 spike（Xvfb 无头验证 + JNI）。
// Kotlin 2.0.21 与 ui-android 同版；Compose 1.7.3 为官方配对版本。M5 正式工程沿用此结论。
plugins {
    kotlin("jvm") version "2.0.21"
    id("org.jetbrains.compose") version "1.7.3"
    id("org.jetbrains.kotlin.plugin.compose") version "2.0.21"
}

dependencies {
    implementation(compose.desktop.currentOs)
}

// 复用 ui-desktop 的 Java JNI 载体（包名/类名不变则 JNI 符号不变，零复制）
sourceSets["main"].java.srcDir("../ui-desktop/src/main/java")

compose.desktop {
    application {
        mainClass = "com.starriver.desktop.compose.MainKt"
        jvmArgs("-Xmx512m")
        nativeDistributions {
            targetFormats(TargetFormat.AppImage) // G0-PREP 不打包，仅占位；M5/T-M7 定 exe/msi
        }
    }
}
