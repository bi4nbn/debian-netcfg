package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testKeySelf = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAISELFTESTKEYAAAAAAAAAAAAAAAAAAAAAAAAAAA netcfg-test"
	testKeyUser = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQUSERKEYAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA user@host"
)

func authKeysPathIn(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".ssh", "authorized_keys")
}

func TestEnsureAuthorizedKeyFromEmpty(t *testing.T) {
	path := authKeysPathIn(t)
	others, added, err := ensureAuthorizedKey(path, testKeySelf)
	if err != nil {
		t.Fatalf("ensureAuthorizedKey: %v", err)
	}
	if others != 0 {
		t.Errorf("others = %d, want 0", others)
	}
	if !added {
		t.Errorf("added = false, want true")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "AAAAC3NzaC1lZDI1NTE5AAAAISELFTESTKEY") {
		t.Errorf("key not written: %s", data)
	}
	// 权限必须为 0600
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("authorized_keys perm = %o, want 600", perm)
	}
}

// 重复执行初始化时：本工具自己的公钥必须去重，且不能计入“其它公钥”数量
// （该计数用于提示服务器上还有多少把用户自有密钥）。
func TestEnsureAuthorizedKeyRerunDedupes(t *testing.T) {
	path := authKeysPathIn(t)

	if _, _, err := ensureAuthorizedKey(path, testKeySelf); err != nil {
		t.Fatalf("first run: %v", err)
	}
	others, added, err := ensureAuthorizedKey(path, testKeySelf)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if others != 0 {
		t.Errorf("others after rerun = %d, want 0 (own key is not a foreign key)", others)
	}
	if added {
		t.Errorf("added after rerun = true, want false (dedupe)")
	}
	data, _ := os.ReadFile(path)
	if n := strings.Count(string(data), "AAAAC3NzaC1lZDI1NTE5AAAAISELFTESTKEY"); n != 1 {
		t.Errorf("key duplicated %d times: %s", n, data)
	}
}

// 关闭密码登录前的兜底校验：必须能确认公钥真的落在文件中
func TestFileHasAuthorizedKey(t *testing.T) {
	path := authKeysPathIn(t)
	if fileHasAuthorizedKey(path, testKeySelf) {
		t.Errorf("missing file should not report key present")
	}

	if _, _, err := ensureAuthorizedKey(path, testKeySelf); err != nil {
		t.Fatalf("ensureAuthorizedKey: %v", err)
	}
	if !fileHasAuthorizedKey(path, testKeySelf) {
		t.Errorf("key should be reported present")
	}
	if fileHasAuthorizedKey(path, testKeyUser) {
		t.Errorf("unrelated key must not match")
	}
	// 同一密钥体、不同注释（即不同行）也应视为已存在
	if !fileHasAuthorizedKey(path, strings.Fields(testKeySelf)[0]+" "+strings.Fields(testKeySelf)[1]+" another@comment") {
		t.Errorf("same key blob with different comment should match")
	}
}

func TestEnsureAuthorizedKeyPreservesUserKeys(t *testing.T) {
	path := authKeysPathIn(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	original := "# existing keys\n" + testKeyUser + "\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	others, added, err := ensureAuthorizedKey(path, testKeySelf)
	if err != nil {
		t.Fatalf("ensureAuthorizedKey: %v", err)
	}
	if others != 1 {
		t.Errorf("others = %d, want 1", others)
	}
	if !added {
		t.Errorf("added = false, want true")
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	if !strings.Contains(got, "USERKEY") {
		t.Errorf("existing user key was dropped:\n%s", got)
	}
	if !strings.Contains(got, "SELFTESTKEY") {
		t.Errorf("tool key missing:\n%s", got)
	}
}

func TestEnsureAuthorizedKeyCountsOnlyForeignKeys(t *testing.T) {
	path := authKeysPathIn(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	// 已有本工具公钥 + 一把用户公钥
	seed := testKeySelf + "\n" + testKeyUser + "\n"
	if err := os.WriteFile(path, []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}

	others, added, err := ensureAuthorizedKey(path, testKeySelf)
	if err != nil {
		t.Fatalf("ensureAuthorizedKey: %v", err)
	}
	if others != 1 {
		t.Errorf("others = %d, want 1", others)
	}
	if added {
		t.Errorf("added = true, want false")
	}
}

func TestIsValidSSHPublicKey(t *testing.T) {
	valid := []string{
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAB user@host",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI comment",
		"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTY= x",
	}
	for _, k := range valid {
		if !isValidSSHPublicKey(k) {
			t.Errorf("expected valid: %q", k)
		}
	}
	invalid := []string{"", "just-one-field", "ssh-rsa", "notakey AAAA", "<html>error</html>"}
	for _, k := range invalid {
		if isValidSSHPublicKey(k) {
			t.Errorf("expected invalid: %q", k)
		}
	}
}
