package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeInterfaces(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "interfaces")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp interfaces: %v", err)
	}
	return path
}

func readInterfaces(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read interfaces: %v", err)
	}
	return string(data)
}

// 修复验证：块替换必须保留 source 指令与其它未管理网卡，且清理旧版本写入的头部标记
func TestRewriteInterfacesPreservesOtherStanzas(t *testing.T) {
	orig := `# Auto generated config - 2026-01-01_00:00:00
auto lo
iface lo inet loopback

source /etc/network/interfaces.d/*

auto eth1
iface eth1 inet static
    address 10.0.0.2
    netmask 255.255.255.0

auto eth0
iface eth0 inet static
    address 192.168.1.10
    netmask 255.255.255.0
    gateway 192.168.1.1
`
	path := writeInterfaces(t, orig)
	err := rewriteInterfaces(path, []ifaceBlock{{
		iface: "eth0",
		lines: []string{
			"auto eth0",
			"iface eth0 inet static",
			"    address 192.168.1.20",
			"    netmask 255.255.255.0",
			"    gateway 192.168.1.1",
		},
	}})
	if err != nil {
		t.Fatalf("rewriteInterfaces: %v", err)
	}

	got := readInterfaces(t, path)
	for _, want := range []string{
		"source /etc/network/interfaces.d/*",
		"iface eth1 inet static",
		"address 10.0.0.2",
		"address 192.168.1.20",
		"iface lo inet loopback",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q to be preserved, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "192.168.1.10") {
		t.Errorf("old address should be replaced, got:\n%s", got)
	}
	if strings.Contains(got, "# Auto generated") {
		t.Errorf("legacy auto-generated header should be removed, got:\n%s", got)
	}
	if n := strings.Count(got, "iface eth0 inet static"); n != 1 {
		t.Errorf("expected exactly 1 eth0 stanza, got %d:\n%s", n, got)
	}
	if n := strings.Count(got, "auto eth0"); n != 1 {
		t.Errorf("expected exactly 1 auto eth0 line, got %d:\n%s", n, got)
	}
}

// auto 行含多个网卡时，只移除被管理的那个
func TestRewriteInterfacesKeepsUnmanagedAutoNames(t *testing.T) {
	path := writeInterfaces(t, "auto eth0 eth1\n")
	err := rewriteInterfaces(path, []ifaceBlock{{
		iface: "eth0",
		lines: []string{"auto eth0", "iface eth0 inet dhcp"},
	}})
	if err != nil {
		t.Fatalf("rewriteInterfaces: %v", err)
	}
	got := readInterfaces(t, path)
	if !strings.Contains(got, "auto eth1") {
		t.Errorf("unmanaged auto name dropped, got:\n%s", got)
	}
	if strings.Contains(got, "auto eth0 eth1") {
		t.Errorf("managed name should be removed from auto line, got:\n%s", got)
	}
}

// 文件缺少 lo stanza 时应自动补齐
func TestRewriteInterfacesAddsLoopback(t *testing.T) {
	path := writeInterfaces(t, "auto eth0\niface eth0 inet dhcp\n")
	if err := rewriteInterfaces(path, []ifaceBlock{{
		iface: "eth0",
		lines: []string{"auto eth0", "iface eth0 inet static", "    address 10.0.0.5", "    netmask 255.255.255.0", "    gateway 10.0.0.1"},
	}}); err != nil {
		t.Fatalf("rewriteInterfaces: %v", err)
	}
	got := readInterfaces(t, path)
	if !strings.Contains(got, "iface lo inet loopback") {
		t.Errorf("loopback stanza missing, got:\n%s", got)
	}
}

// IPv6 追加/替换必须保留 IPv4 stanza，且不产生重复 inet6 块
func TestUpsertIPv6Block(t *testing.T) {
	orig := `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    address 192.168.1.10
    netmask 255.255.255.0
    gateway 192.168.1.1

iface eth0 inet6 static
    address 2409::1/64
    gateway 2409::1
`
	path := writeInterfaces(t, orig)
	if err := upsertIPv6Block(path, "eth0", "2409::2/64", "2409::1"); err != nil {
		t.Fatalf("upsertIPv6Block: %v", err)
	}

	got := readInterfaces(t, path)
	for _, want := range []string{"address 192.168.1.10", "gateway 192.168.1.1", "address 2409::2/64"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "2409::1/64") {
		t.Errorf("old IPv6 address should be replaced, got:\n%s", got)
	}
	if n := strings.Count(got, "iface eth0 inet6 static"); n != 1 {
		t.Errorf("expected exactly 1 inet6 stanza, got %d:\n%s", n, got)
	}
	if n := strings.Count(got, "iface eth0 inet static"); n != 1 {
		t.Errorf("expected exactly 1 IPv4 stanza, got %d:\n%s", n, got)
	}
}

// 首次追加 IPv6 时应插在该网卡的 IPv4 块之后
func TestUpsertIPv6BlockAppendsWhenMissing(t *testing.T) {
	orig := `auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp
`
	path := writeInterfaces(t, orig)
	if err := upsertIPv6Block(path, "eth0", "2409::2/64", "2409::1"); err != nil {
		t.Fatalf("upsertIPv6Block: %v", err)
	}

	got := readInterfaces(t, path)
	if !strings.Contains(got, "iface eth0 inet6 static") || !strings.Contains(got, "address 2409::2/64") {
		t.Errorf("IPv6 stanza not appended, got:\n%s", got)
	}
	if strings.Index(got, "iface eth0 inet dhcp") > strings.Index(got, "iface eth0 inet6 static") {
		t.Errorf("inet6 stanza should follow the IPv4 stanza, got:\n%s", got)
	}
}

func TestGetConfiguredIPv6Address(t *testing.T) {
	path := writeInterfaces(t, `auto eth0
iface eth0 inet static
    address 192.168.1.10
    netmask 255.255.255.0

iface eth0 inet6 static
    address 2409::5/64
    gateway 2409::1
`)
	if got := getConfiguredIPv6Address(path, "eth0"); got != "2409::5/64" {
		t.Errorf("getConfiguredIPv6Address = %q, want %q", got, "2409::5/64")
	}
	if got := getConfiguredIPv6Address(path, "eth1"); got != "" {
		t.Errorf("unexpected address for eth1: %q", got)
	}
}

func TestNormalizeSHA256(t *testing.T) {
	const sum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cases := map[string]string{
		sum:                sum,
		sum + "  netcfg\n": sum,
		"SHA256:" + sum:    "",
		"sha256:" + sum:    sum,
		"deadbeef":         "",
		"":                 "",
	}
	for input, want := range cases {
		if got := normalizeSHA256(input); got != want {
			t.Errorf("normalizeSHA256(%q) = %q, want %q", input, got, want)
		}
	}
}

// 对完全没有 stanza 的网卡下发 IPv6 时，必须同时生成 auto <iface>，
// 否则重启后 ifup -a 不会拉起该配置（只有 iface 段是不够的）。
func TestUpsertIPv6BlockAddsAutoLine(t *testing.T) {
	path := writeInterfaces(t, "auto lo\niface lo inet loopback\n")
	if err := upsertIPv6Block(path, "eth7", "2409::7/64", "2409::1"); err != nil {
		t.Fatalf("upsertIPv6Block: %v", err)
	}
	got := readInterfaces(t, path)
	if !strings.Contains(got, "auto eth7") {
		t.Errorf("auto eth7 missing, config would not come up after reboot:\n%s", got)
	}
	if strings.Index(got, "auto eth7") > strings.Index(got, "iface eth7 inet6 static") {
		t.Errorf("auto must precede the stanza:\n%s", got)
	}
}

// 已有 stanza 但缺 auto 时也要补上，且 auto 必须在 stanza 之前
func TestUpsertIPv6BlockAddsMissingAutoToExistingStanza(t *testing.T) {
	path := writeInterfaces(t, "auto lo\niface lo inet loopback\n\niface eth8 inet static\n    address 10.1.1.1\n    netmask 255.255.255.0\n")
	if err := upsertIPv6Block(path, "eth8", "2409::8/64", "2409::1"); err != nil {
		t.Fatalf("upsertIPv6Block: %v", err)
	}
	got := readInterfaces(t, path)
	if n := strings.Count(got, "auto eth8"); n != 1 {
		t.Errorf("expected exactly 1 auto eth8, got %d:\n%s", n, got)
	}
	if strings.Index(got, "auto eth8") > strings.Index(got, "iface eth8 inet static") {
		t.Errorf("auto must precede the stanza:\n%s", got)
	}
	if !strings.Contains(got, "iface eth8 inet6 static") || !strings.Contains(got, "address 2409::8/64") {
		t.Errorf("ipv6 stanza missing:\n%s", got)
	}
}

// 已有 auto 时不应重复添加
func TestUpsertIPv6BlockKeepsSingleAuto(t *testing.T) {
	path := writeInterfaces(t, "auto lo\niface lo inet loopback\n\nauto eth0\niface eth0 inet dhcp\n")
	if err := upsertIPv6Block(path, "eth0", "2409::2/64", "2409::1"); err != nil {
		t.Fatalf("upsertIPv6Block: %v", err)
	}
	got := readInterfaces(t, path)
	if n := strings.Count(got, "auto eth0"); n != 1 {
		t.Errorf("expected exactly 1 auto eth0, got %d:\n%s", n, got)
	}
}

// 注释里的 `# iface eth0 inet dhcp` 不能被当作真实模式
func TestIPModeFromInterfacesFile(t *testing.T) {
	path := writeInterfaces(t, `# iface eth0 inet dhcp
auto eth0
iface eth0 inet static
    address 192.168.1.10
    netmask 255.255.255.0

auto eth1
iface eth1 inet dhcp
`)
	if m, st := ipModeFromInterfacesFile(path, "eth0"); m != "static" || !st {
		t.Errorf("eth0 = %q/%v, want static/true", m, st)
	}
	if m, st := ipModeFromInterfacesFile(path, "eth1"); m != "dhcp" || st {
		t.Errorf("eth1 = %q/%v, want dhcp/false", m, st)
	}
	if m, _ := ipModeFromInterfacesFile(path, "eth2"); m != "" {
		t.Errorf("eth2 = %q, want empty", m)
	}
	if m, _ := ipModeFromInterfacesFile(filepath.Join(t.TempDir(), "nope"), "eth0"); m != "" {
		t.Errorf("missing file = %q, want empty", m)
	}
}

// resolv.conf 的 search/options 等指令必须保留，否则内网短名解析会失效
func TestMergeResolvContentPreservesSearchAndOptions(t *testing.T) {
	existing := `# generated by netcfg
search corp.example.com example.com
options ndots:5 timeout:2
nameserver 10.0.0.53
nameserver 223.5.5.5
`
	content, preserved := mergeResolvContent(existing, false)

	if !strings.HasPrefix(content, "nameserver 223.5.5.5\n") {
		t.Errorf("AliDNS should be first:\n%s", content)
	}
	for _, want := range []string{
		"search corp.example.com example.com",
		"options ndots:5 timeout:2",
		"nameserver 10.0.0.53",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("lost %q:\n%s", want, content)
		}
	}
	if n := strings.Count(content, "nameserver 223.5.5.5"); n != 1 {
		t.Errorf("AliDNS duplicated %d times:\n%s", n, content)
	}
	if len(preserved) != 1 || preserved[0] != "10.0.0.53" {
		t.Errorf("preserved = %v, want [10.0.0.53]", preserved)
	}
}

func TestMergeResolvContentWithIPv6(t *testing.T) {
	content, _ := mergeResolvContent("search lan\n", true)
	if !strings.Contains(content, "nameserver "+AliDNS6[0]) {
		t.Errorf("IPv6 DNS missing when enabled:\n%s", content)
	}
	content4, _ := mergeResolvContent("search lan\n", false)
	if strings.Contains(content4, AliDNS6[0]) {
		t.Errorf("IPv6 DNS should not be added when disabled:\n%s", content4)
	}
}
