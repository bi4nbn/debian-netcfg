package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// 颜色定义
const (
	RED    = "\033[0;31m"
	GREEN  = "\033[0;32m"
	YELLOW = "\033[1;33m"
	BLUE   = "\033[0;34m"
	NC     = "\033[0m"
)

// 全局 DNS 配置（阿里云）
var (
	AliDNS4 = []string{"223.5.5.5", "223.6.6.6"}
	AliDNS6 = []string{"2400:3200::1", "2400:3200:baba::1"}
)

// 持久化标记文件路径（用于记录初始化状态）
const initFlagPath = "/etc/netcfg.initialized"

// ------------------------------
// 日志函数
// ------------------------------
func Error(msg string)    { fmt.Fprintf(os.Stderr, "%s[Error]%s %s\n", RED, NC, msg) }
func Fatal(msg string)    { fmt.Fprintf(os.Stderr, "%s[Fatal]%s %s\n", RED, NC, msg); os.Exit(1) }
func Info(msg string)     { fmt.Printf("%s[Info]%s %s\n", BLUE, NC, msg) }
func Success(msg string)  { fmt.Printf("%s[Success]%s %s\n", GREEN, NC, msg) }
func Warn(msg string)     { fmt.Printf("%s[Warning]%s %s\n", YELLOW, NC, msg) }
func Interact(msg string) { fmt.Printf("%s[Prompt]%s %s", YELLOW, NC, msg) }

// ------------------------------
// 交互输入工具
// ------------------------------
var reader = bufio.NewReader(os.Stdin)

// inputEOF 标记 stdin 是否已关闭（管道/重定向结束后用于安全退出，避免死循环）
var inputEOF bool

// InputClosed 返回标准输入是否已到达 EOF
func InputClosed() bool { return inputEOF }

func ReadInput(prompt string, defaultValue string) string {
	fmt.Print(prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		// 记录 EOF，仍返回已读取到的内容（可能为空）
		inputEOF = true
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	return input
}

func ReadConfirm(prompt string, defaultYes bool) bool {
	defStr := "n"
	if defaultYes {
		defStr = "y"
	}
	for {
		input := strings.ToLower(ReadInput(prompt, defStr))
		if InputClosed() {
			return false
		}
		switch input {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
		Error(T("invalid_yes_no"))
	}
}

func PromptIPv4Config(defaultIP, defaultMask, defaultGW string) (ip, mask, gw string) {
	defaultCIDR := ""
	if defaultIP != "" && defaultMask != "" {
		cidrNum, err := NetmaskToCIDR(defaultMask)
		if err == nil {
			defaultCIDR = fmt.Sprintf("%s/%d", defaultIP, cidrNum)
		}
	}

	var inputCIDR string
	for {
		inputCIDR = ReadInput(T("input_ipv4"), defaultCIDR)
		if InputClosed() {
			return "", "", ""
		}
		if inputCIDR == "" {
			inputCIDR = defaultCIDR
		}
		if inputCIDR == "" {
			Error(T("invalid_ipv4"))
			continue
		}
		_, _, err := net.ParseCIDR(inputCIDR)
		if err != nil {
			Error(T("invalid_ipv4"))
			continue
		}
		break
	}

	parts := strings.Split(inputCIDR, "/")
	ip = parts[0]
	var prefix int
	fmt.Sscanf(parts[1], "%d", &prefix)
	mask = CIDRToNetmask(prefix)

	autoGW, err := GetAutoGatewayFromCIDR(inputCIDR)
	if err == nil && ValidateIPv4(autoGW) {
		gw = autoGW
	} else {
		gw = GetDefaultGateway()
	}
	if gw == "" {
		gw = "0.0.0.0"
	}

	if ReadConfirm(fmt.Sprintf(T("auto_gw_confirm"), gw), false) {
		for {
			inputGW := ReadInput(T("input_gw"), gw)
			if InputClosed() {
				return "", "", ""
			}
			if inputGW == "" {
				inputGW = gw
			}
			if ValidateIPv4(inputGW) {
				gw = inputGW
				break
			}
			Error(T("invalid_gw"))
		}
	}

	Info(fmt.Sprintf(T("ipv4_set"), ip, mask, gw))
	return
}

func PromptIPv6Config() (addr, gw string) {
	for {
		addr = ReadInput(T("input_ipv6_addr"), "")
		if InputClosed() {
			return "", ""
		}
		if ValidateIPv6CIDR(addr) {
			break
		}
		Error(T("invalid_ipv6"))
	}
	for {
		gw = ReadInput(T("input_ipv6_gw"), "")
		if InputClosed() {
			return "", ""
		}
		if ValidateIPv6(gw) {
			break
		}
		Error(T("invalid_ipv6"))
	}
	Info(fmt.Sprintf(T("ipv6_set"), addr, gw))
	return
}

// ------------------------------
// 系统工具
// ------------------------------
func CheckRoot() {
	if os.Getuid() != 0 {
		Fatal(T("err_run_root"))
	}
}

func CommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func RunCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func RunCmdSilent(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// CheckAptNetwork 检查是否可访问 apt 源
func CheckAptNetwork() error {
	err1 := RunCmdSilent("ping", "-c", "1", "-W", "2", "deb.debian.org")
	err2 := RunCmdSilent("ping", "-c", "1", "-W", "2", "mirrors.aliyun.com")
	if err1 != nil && err2 != nil {
		return fmt.Errorf(T("no_network_apt"))
	}
	return nil
}

// EnsureDependencies 检查 iproute2 / ifupdown 是否存在，缺失时经用户确认后自动安装。
// 不再使用 Fatal，由调用方决定如何优雅退出。
func EnsureDependencies() error {
	var needInstall []string
	if !CommandExists("ip") {
		needInstall = append(needInstall, "iproute2")
		Warn(T("ip_cmd_not_found"))
	}
	if !CommandExists("ifup") || !CommandExists("ifdown") {
		needInstall = append(needInstall, "ifupdown")
		Warn(T("ifup_not_found"))
	}
	if len(needInstall) == 0 {
		return nil
	}

	pkgList := strings.Join(needInstall, " ")
	if !ReadConfirm(fmt.Sprintf(T("dep_install_prompt"), pkgList), true) {
		return fmt.Errorf(T("dep_missing"), pkgList)
	}
	if err := CheckAptNetwork(); err != nil {
		return err
	}
	if err := runAptUpdateInstall(needInstall...); err != nil {
		return fmt.Errorf("%s: %v", T("apt_install_fail"), err)
	}
	if !CommandExists("ip") {
		return fmt.Errorf(T("ip_unavailable"))
	}
	if !CommandExists("ifup") || !CommandExists("ifdown") {
		return fmt.Errorf(T("ifupdown_unavailable"))
	}
	return nil
}

func DisableConflictServices() {
	if RunCmdSilent("systemctl", "is-active", "--quiet", "NetworkManager") == nil {
		_ = RunCmdSilent("systemctl", "stop", "NetworkManager")
		_ = RunCmdSilent("systemctl", "disable", "NetworkManager")
	}
	if RunCmdSilent("systemctl", "is-active", "--quiet", "systemd-networkd") == nil {
		_ = RunCmdSilent("systemctl", "stop", "systemd-networkd")
		_ = RunCmdSilent("systemctl", "disable", "systemd-networkd")
	}
}

func Sleep(seconds int) {
	time.Sleep(time.Duration(seconds) * time.Second)
}

func GetCurrentSSHPeerIP() string {
	conn := os.Getenv("SSH_CONNECTION")
	if conn == "" {
		return ""
	}
	parts := strings.Fields(conn)
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}

func GetRouteDevForIP(ip string) string {
	if ip == "" {
		return ""
	}
	out, err := RunCmd("ip", "route", "get", ip)
	if err != nil {
		return ""
	}
	fields := strings.Fields(out)
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// ------------------------------
// IP 工具函数
// ------------------------------
func CIDRToNetmask(cidr int) string {
	if cidr < 0 || cidr > 32 {
		return "255.255.0.0"
	}
	mask := net.CIDRMask(cidr, 32)
	return net.IP(mask).String()
}

func NetmaskToCIDR(maskStr string) (int, error) {
	ip := net.ParseIP(maskStr).To4()
	if ip == nil {
		return 0, fmt.Errorf("invalid netmask")
	}
	mask := net.IPMask(ip)
	ones, bits := mask.Size()
	if bits != 32 {
		return 0, fmt.Errorf("not ipv4 mask")
	}
	return ones, nil
}

func GetAutoGatewayFromCIDR(cidrStr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return "", err
	}
	ip4 := ipNet.IP.To4()
	if ip4 == nil {
		return "", fmt.Errorf("only support ipv4 cidr")
	}
	gatewayIP := make(net.IP, 4)
	copy(gatewayIP, ip4)
	gatewayIP[3] += 1
	return gatewayIP.String(), nil
}

// IsIPInCIDR 判断 ip 是否落在 cidrStr 网段内（用于网关可达性预检）
func IsIPInCIDR(ip, cidrStr string) bool {
	parsed := net.ParseIP(ip)
	_, ipNet, err := net.ParseCIDR(cidrStr)
	if err != nil || parsed == nil {
		return false
	}
	return ipNet.Contains(parsed)
}

func ValidateIPv4(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() != nil
}

func ValidateIPv6CIDR(addr string) bool {
	ip, _, err := net.ParseCIDR(addr)
	if err != nil {
		return false
	}
	return ip.To4() == nil
}

func ValidateIPv6(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() == nil
}

func GetDefaultGateway() string {
	out, err := RunCmd("ip", "route", "show", "default")
	if err != nil {
		return ""
	}
	fields := strings.Fields(out)
	for i, f := range fields {
		if f == "via" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

func IsDHCPClient(ip string) bool {
	if ip == "" {
		return false
	}
	leaseFile := "/var/lib/dhcp/dhclient.leases"
	if _, err := os.Stat(leaseFile); os.IsNotExist(err) {
		return false
	}
	data, err := os.ReadFile(leaseFile)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), ip)
}

// ------------------------------
// 文件工具
// ------------------------------
func BackupFile(path string) string {
	backupPath := fmt.Sprintf("%s.bak_%s", path, time.Now().Format("20060102_150405"))
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err == nil {
			_ = os.WriteFile(backupPath, data, 0644)
			Success(fmt.Sprintf(T("backup_config"), backupPath))
		}
	}
	return backupPath
}

// CopyFile 复制文件并保留权限
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// RestoreFile 用备份覆盖目标文件（备份不存在时返回 false）。
// 用于"已写盘但后续校验/应用失败"时回滚，避免留下不可用的配置。
func RestoreFile(backupPath, target string) bool {
	if backupPath == "" {
		return false
	}
	if _, err := os.Stat(backupPath); err != nil {
		return false
	}
	if err := CopyFile(backupPath, target); err != nil {
		Warn(fmt.Sprintf("failed to restore %s: %v", target, err))
		return false
	}
	Success(fmt.Sprintf(T("restore_backup_ok"), target, backupPath))
	return true
}

// ------------------------------
// 初始化状态持久化函数
// ------------------------------
// IsInitialized 检查系统是否已完成初始化（通过标记文件是否存在判断）
func IsInitialized() bool {
	_, err := os.Stat(initFlagPath)
	return err == nil
}

// markInitialized 创建初始化完成标记文件
func markInitialized() {
	_ = os.WriteFile(initFlagPath, []byte("1"), 0644)
}

// ================== 更新自身（国际化 + 完整性校验版） ==================

const (
	updateRemoteURL = "https://bash.niteng.net/netcfg"
	updateFallback  = "/usr/local/bin/netcfg"
)

// currentExecutablePath 返回当前运行程序自身的真实路径（解析软链接）
func currentExecutablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return updateFallback
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	if !filepath.IsAbs(resolved) {
		return updateFallback
	}
	return resolved
}

// downloadFile 使用 wget/curl 下载文件（curl 加 -f 以便对 HTTP 错误码失败）
func downloadFile(url, dest string, verbose bool) error {
	var lastErr error
	if CommandExists("wget") {
		if out, err := RunCmd("wget", "-q", "-O", dest, url); err == nil {
			return nil
		} else {
			lastErr = fmt.Errorf("wget: %v %s", err, strings.TrimSpace(out))
			if verbose {
				Warn(T("update_self_wget_fail"))
			}
		}
	}
	if CommandExists("curl") {
		if out, err := RunCmd("curl", "-fsSL", "-o", dest, url); err == nil {
			return nil
		} else {
			lastErr = fmt.Errorf("curl: %v %s", err, strings.TrimSpace(out))
		}
	}
	if lastErr == nil {
		return fmt.Errorf(T("update_self_no_tool"))
	}
	return lastErr
}

// fileSHA256 计算文件 SHA256
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// normalizeSHA256 从校验文件中提取 64 位十六进制摘要
func normalizeSHA256(raw string) string {
	for _, field := range strings.Fields(raw) {
		field = strings.TrimPrefix(field, "sha256:")
		field = strings.TrimPrefix(field, "*")
		field = strings.ToLower(strings.TrimSpace(field))
		if len(field) == 64 && strings.Trim(field, "0123456789abcdef") == "" {
			return field
		}
	}
	return ""
}

// isELF 校验下载内容是否为 ELF 可执行文件（防止把 HTML 错误页写成二进制）
func isELF(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil {
		return false
	}
	return magic[0] == 0x7f && magic[1] == 'E' && magic[2] == 'L' && magic[3] == 'F'
}

// updateSelf 从远程下载最新版本并替换自身。
// 校验顺序：NETCFG_UPDATE_SHA256 固定校验值 > 远程 .sha256 校验文件 > 无校验需用户确认。
func updateSelf() {
	localPath := currentExecutablePath()
	tmpPath := localPath + ".tmp"
	defer func() {
		_ = os.Remove(tmpPath)
		_ = os.Remove(tmpPath + ".sha256")
	}()

	Info(fmt.Sprintf(T("update_self_start"), updateRemoteURL))
	Warn(fmt.Sprintf("Target binary: %s", localPath))

	if err := downloadFile(updateRemoteURL, tmpPath, true); err != nil {
		Warn(fmt.Sprintf(T("update_self_curl_fail"), err.Error()))
		return
	}

	if !isELF(tmpPath) {
		Error(T("update_self_bad_binary"))
		return
	}

	// ---- 完整性校验 ----
	actual, err := fileSHA256(tmpPath)
	if err != nil {
		Error(fmt.Sprintf(T("update_self_checksum_fail"), "n/a", err.Error()))
		return
	}

	expected := strings.ToLower(strings.TrimSpace(os.Getenv("NETCFG_UPDATE_SHA256")))
	if expected == "" {
		// 尝试下载同名 .sha256 校验文件
		if err := downloadFile(updateRemoteURL+".sha256", tmpPath+".sha256", false); err == nil {
			if data, err := os.ReadFile(tmpPath + ".sha256"); err == nil {
				expected = normalizeSHA256(string(data))
			}
		}
	}

	switch {
	case expected != "" && expected != actual:
		Error(fmt.Sprintf(T("update_self_checksum_fail"), expected, actual))
		return
	case expected != "":
		Success(T("update_self_checksum_ok"))
	default:
		Warn(T("update_self_no_checksum"))
		if !ReadConfirm(T("update_self_confirm"), false) {
			Info(T("cancelled"))
			return
		}
	}

	// ---- 备份当前程序，失败则中止 ----
	backupPath := fmt.Sprintf("%s.bak_update_%s", localPath, time.Now().Format("20060102_150405"))
	if err := CopyFile(localPath, backupPath); err != nil {
		Error(fmt.Sprintf(T("update_self_backup_fail"), err.Error()))
		return
	}
	Success(fmt.Sprintf(T("update_self_backup"), backupPath))

	// ---- 覆盖原文件 ----
	if err := os.Chmod(tmpPath, 0755); err != nil {
		Error(fmt.Sprintf(T("update_self_chmod_fail"), err.Error()))
		return
	}
	if err := os.Rename(tmpPath, localPath); err != nil {
		Error(fmt.Sprintf(T("update_self_rename_fail"), err.Error()))
		return
	}

	Success(T("update_self_success"))
	Sleep(2)

	// 重新执行自身
	cmd := exec.Command(localPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		Error(fmt.Sprintf(T("update_self_restart_fail"), err.Error()))
		return
	}

	// 退出当前进程
	os.Exit(0)
}
