package mobilebind

import "testing"

func TestSetNetworkProxyBridge(t *testing.T) {
	if r := SetNetworkProxy(""); r != "OK:direct" {
		t.Fatalf("clear: %q", r)
	}
	if GetNetworkProxy() != "" {
		t.Fatalf("expected empty")
	}
	if r := SetNetworkProxy("http://127.0.0.1:7890"); r != "OK:proxy" {
		t.Fatalf("set: %q", r)
	}
	if GetNetworkProxy() != "http://127.0.0.1:7890" {
		t.Fatalf("get: %q", GetNetworkProxy())
	}
	if r := SetNetworkProxy("bad"); len(r) < 4 || r[:4] != "ERR:" {
		t.Fatalf("expected ERR:, got %q", r)
	}
	SetNetworkProxy("")
}
