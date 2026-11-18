package bridge

import (
"runtime"
"testing"
)

func TestCPUFeaturesArchMatrix(t *testing.T) {
neon, avx2 := CPUFeatures()
if runtime.GOARCH != "arm64" && neon {
t.Errorf("neon=true on non-arm64 %s", runtime.GOARCH)
}
if runtime.GOARCH != "amd64" && runtime.GOARCH != "386" && avx2 {
t.Errorf("avx2=true on non-x86 %s", runtime.GOARCH)
}
t.Logf("neon=%v avx2=%v (goarch=%s)", neon, avx2, runtime.GOARCH)
}

func TestAvailMemBytes(t *testing.T) {
if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
t.Skip("unsupported platform")
}
m := AvailMemBytes()
if m == 0 {
t.Fatal("avail mem = 0: sysinfo failed or machine truly exhausted")
}
if m > uint64(1)<<42 {
t.Fatalf("avail mem implausible: %d", m)
}
t.Logf("avail_bytes=%d", m)
}
