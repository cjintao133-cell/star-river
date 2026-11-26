package mobilebind

import "testing"

func TestConnectorsAddListRemove(t *testing.T) {
	dir := t.TempDir()
	if r := InitKernel(dir+"/c.db", b64key()); r != "OK" {
		t.Fatalf("init: %s", r)
	}
	defer CloseKernel()
	def := `{"id":"demo","name":"演示","auth":{"type":"oauth2","auth_url":"https://example.com/auth","client_id":"cid","scopes":["read"]},"tools":[{"name":"ping","description":"ping"}]}`
	if r := ConnectorsAdd(def); r != "OK" {
		t.Fatalf("add: %s", r)
	}
	list := ConnectorsList()
	if list == "[]" || len(list) < 5 {
		t.Fatalf("list empty: %s", list)
	}
	if u := ConnectorsAuthURL("demo", "app://oauth", ""); len(u) < 4 && u[:4] == "ERR:" {
		t.Fatalf("authurl: %s", u)
	}
	if r := ConnectorsRemove("demo"); r != "OK" {
		t.Fatalf("remove: %s", r)
	}
	if r := ConnectorsRemove("demo"); r != "ERR:not found" {
		t.Fatalf("remove twice should be not found: %s", r)
	}
}
