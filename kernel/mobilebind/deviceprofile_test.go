package mobilebind

import (
"encoding/json"
"testing"
)

func TestDeviceProfileJSONRoundTrip(t *testing.T) {
s := DeviceProfile()
var m map[string]any
if err := json.Unmarshal([]byte(s), &m); err != nil {
t.Fatalf("not valid JSON: %v\nraw=%s", err, s)
}
if m["version"] != Version() {
t.Errorf("version mismatch: %v", m["version"])
}
for _, k := range []string{"neon", "avx2"} {
if _, ok := m[k].(bool); !ok {
t.Errorf("%s not bool: %T", k, m[k])
}
}
mem, ok := m["avail_mem_bytes"].(float64)
if !ok || mem <= 0 {
t.Errorf("avail_mem_bytes invalid: %v", m["avail_mem_bytes"])
}
t.Logf("profile=%s", s)
}
