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
