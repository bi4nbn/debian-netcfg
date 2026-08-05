package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
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

func ReadInput(prompt string, defaultValue string) string {
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	return input
}

func ReadConfirm(prompt string, defaultYes bool) bool {
	defStr := "y"
	if !defaultYes {
		defStr = "n"
	}
	input := ReadInput(prompt, defStr)
	return strings.ToLower(input) == "y"
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
		if inputCIDR == "" {
			inputCIDR = defaultCIDR
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
		if ValidateIPv6CIDR(addr) {
			break
		}
		Error(T("invalid_ipv6"))
	}
	for {
		gw = ReadInput(T("input_ipv6_gw"), "")
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

func CheckAptNetwork() {
	err1 := RunCmdSilent("ping", "-c", "1", "-W", "2", "deb.debian.org")
	err2 := RunCmdSilent("ping", "-c", "1", "-W", "2", "mirrors.aliyun.com")
	if err1 != nil && err2 != nil {
		Fatal(T("no_network_apt"))
	}
}

func InstallBaseDeps() {
	var needInstall []string
	if !CommandExists("ip") {
		needInstall = append(needInstall, "iproute2")
	}
	if !CommandExists("ifup") || !CommandExists("ifdown") {
		needInstall = append(needInstall, "ifupdown")
	}
	if len(needInstall) > 0 {
		err := RunCmdSilent("apt", "update", "-qq")
		if err != nil {
			Fatal(T("apt_update_fail"))
		}
		args := append([]string{"install", "-y", "-qq"}, needInstall...)
		err = RunCmdSilent("apt", args...)
		if err != nil {
			Fatal(T("apt_install_fail"))
		}
	}
	if !CommandExists("ip") {
		Fatal(T("ip_unavailable"))
	}
	if !CommandExists("ifup") || !CommandExists("ifdown") {
		Fatal(T("ifupdown_unavailable"))
	}
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

func GetCurrentSSHLocalIP() string {
	conn := os.Getenv("SSH_CONNECTION")
	if conn == "" {
		return ""
	}
	parts := strings.Fields(conn)
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
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

func ValidateIPv4(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() != nil
}

func ValidateNetmask(mask string) bool {
	if !ValidateIPv4(mask) {
		return false
	}
	ipMask := net.IPMask(net.ParseIP(mask).To4())
	ones, bits := ipMask.Size()
	return bits == 32 && ones >= 0 && ones <= 32
}

func ParseNetmask(input string) (string, error) {
	if strings.Contains(input, ".") {
		if !ValidateIPv4(input) {
			return "", fmt.Errorf("invalid netmask format")
		}
		mask := net.ParseIP(input).To4()
		if mask == nil {
			return "", fmt.Errorf("invalid netmask")
		}
		maskInt := uint32(mask[0])<<24 | uint32(mask[1])<<16 | uint32(mask[2])<<8 | uint32(mask[3])
		if maskInt != 0 {
			inv := ^maskInt + 1
			if inv&(inv-1) != 0 {
				return "", fmt.Errorf("non-contiguous netmask")
			}
		}
		return input, nil
	}
	var prefix int
	_, err := fmt.Sscanf(input, "%d", &prefix)
	if err != nil {
		return "", fmt.Errorf("invalid format")
	}
	if prefix < 0 || prefix > 32 {
		return "", fmt.Errorf("CIDR prefix must be 0-32")
	}
	mask := net.CIDRMask(prefix, 32)
	return net.IP(mask).String(), nil
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

// ================== 初始化系统（Go 实现） ==================

// getDebianCodename 获取 Debian 版本代号
func getDebianCodename() (string, error) {
	// 先尝试 /etc/os-release
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "VERSION_CODENAME=") {
				val := strings.TrimPrefix(line, "VERSION_CODENAME=")
				val = strings.Trim(val, `"`)
				if val != "" {
					return val, nil
				}
			}
		}
	}
	// 尝试 lsb_release 命令
	if out, err := RunCmd("lsb_release", "-cs"); err == nil {
		ver := strings.TrimSpace(out)
		if ver != "" {
			return ver, nil
		}
	}
	// 回退到 /etc/debian_version
	if data, err := os.ReadFile("/etc/debian_version"); err == nil {
		ver := strings.TrimSpace(string(data))
		parts := strings.Split(ver, ".")
		major := parts[0]
		switch major {
		case "11":
			return "bullseye", nil
		case "12":
			return "bookworm", nil
		case "13":
			return "trixie", nil
		default:
			return "", fmt.Errorf("unsupported Debian version: %s", major)
		}
	}
	return "", fmt.Errorf("cannot determine Debian codename")
}

// writeSourcesList 写入华为云源
func writeSourcesList(codename string) error {
	content := fmt.Sprintf(`deb https://mirrors.huaweicloud.com/debian/ %s main contrib non-free non-free-firmware
deb https://mirrors.huaweicloud.com/debian/ %s-updates main contrib non-free non-free-firmware
deb https://mirrors.huaweicloud.com/debian/ %s-backports main contrib non-free non-free-firmware
deb https://mirrors.huaweicloud.com/debian-security/ %s-security main contrib non-free non-free-firmware
`, codename, codename, codename, codename)
	if err := os.WriteFile("/etc/apt/sources.list", []byte(content), 0644); err != nil {
		return err
	}
	// 处理 DEB822 格式源（如果存在）
	if _, err := os.Stat("/etc/apt/sources.list.d/debian.sources"); err == nil {
		data, err := os.ReadFile("/etc/apt/sources.list.d/debian.sources")
		if err == nil {
			newData := strings.ReplaceAll(string(data), "deb.debian.org", "mirrors.huaweicloud.com")
			_ = os.WriteFile("/etc/apt/sources.list.d/debian.sources", []byte(newData), 0644)
		}
	}
	return nil
}

// runAptUpdateInstall 执行 apt update 并安装指定包
func runAptUpdateInstall(packages ...string) error {
	Info("Running apt update...")
	if out, err := RunCmd("apt", "update", "-y"); err != nil {
		return fmt.Errorf("apt update failed: %v, output: %s", err, out)
	}
	if len(packages) > 0 {
		Info("Installing packages: " + strings.Join(packages, " "))
		args := append([]string{"install", "-y"}, packages...)
		if out, err := RunCmd("apt", args...); err != nil {
			return fmt.Errorf("apt install failed: %v, output: %s", err, out)
		}
	}
	return nil
}

// setupSSH 配置 SSH（公钥、sshd_config）
func setupSSH() error {
	const (
		authorizedKey = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDC8s1el1MUWsPgmSmJ1npXoiEkIdBlrBk5QbVm5/3USPUGt1GQ9XAvyufuDklLjK1Gz7IGSS0wu3iZH9u2baGvaHUxQZaOYgFf24nIUe4kv/Rba+4zWI3gajZk2WKJV1dr3diGHs9JLjeoX4ZiszRSAZi+zxs8BWj/7V2X5RoeaUwGvCdvpCAwET7N7Jdu9/WBG5ZoK7ypp1+B5EEc8TlLse5PcRdYnLh3arLSt/FDL8NpcjUgRgPTGUmT53cGvo8RXuVfE0W9+9JAO1b6GQFR8rBN3gkhHNSx5hGQeLHYN4WNuUo8/eTJ6hRYFJNG1kFEtaB8IX9WEATwFiso800TsthTa0EYVdHbatkGkDjBJBWeF8yc4Tg4af+FEigH7hYfEsLxBejcFBmFmaeBAx4RGwzGlX4J8xVvPoW7Yul0Ln2hTUwRwG3pZ0xcqX/CMj8BfvUbYNSLOqwInUspmwRfn6dxayMpcg9GEkLyM+VwseVmV+YQ0gKrTYwd2rCzKN2PinJVSkP8i2mA7+bnESELjoz9VLHucXT+TOVbLJsxRUnoIYQe6mw/bjAYM79E/8IOqafSaxuxMQ6NubL12K3CY2lC3H0VTi2+KoHCUO0ZEvrez0X5KjwGPreaa9CCygqF5497iGA88sVgTuD8KCPZEJmJEulYIeZ2QIAlnOBnaw== bi4nbn@qq.com`
		sshdConfigPath = "/etc/ssh/sshd_config"
		sshDir         = "/root/.ssh"
		authKeysPath   = sshDir + "/authorized_keys"
	)

	// 备份原始配置
	BackupFile(sshdConfigPath)
	if _, err := os.Stat(authKeysPath); err == nil {
		BackupFile(authKeysPath)
	}

	// 写入公钥
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create .ssh: %v", err)
	}
	if err := os.WriteFile(authKeysPath, []byte(authorizedKey), 0600); err != nil {
		return fmt.Errorf("failed to write authorized_keys: %v", err)
	}

	// 写入 sshd_config
	sshdConfigContent := `# ==================SSCLOUD SSHD CONFIGURATION==================
AllowUsers root
PermitRootLogin prohibit-password
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
ChallengeResponseAuthentication no
LoginGraceTime 10s
MaxAuthTries 3
MaxSessions 5
MaxStartups 10:30:50
Protocol 2
UsePAM yes
StrictModes yes
LogLevel VERBOSE
AllowAgentForwarding no
AllowTcpForwarding no
X11Forwarding no
PermitTunnel no
GatewayPorts no
PermitUserEnvironment no
PrintMotd no
AcceptEnv LANG LC_*
Subsystem sftp /usr/lib/openssh/sftp-server
`
	if err := os.WriteFile(sshdConfigPath, []byte(sshdConfigContent), 0644); err != nil {
		return fmt.Errorf("failed to write sshd_config: %v", err)
	}

	// 校验配置
	if out, err := RunCmd("sshd", "-t"); err != nil {
		// 回滚
		backupPath := sshdConfigPath + ".bak_" + time.Now().Format("2006-01-02-15:04:05")
		if _, err := os.Stat(backupPath); err == nil {
			_ = os.Rename(backupPath, sshdConfigPath)
		}
		return fmt.Errorf("sshd config test failed: %v, output: %s", err, out)
	}

	// 重启 SSH 服务
	restartCmds := [][]string{
		{"systemctl", "restart", "sshd"},
		{"systemctl", "restart", "ssh"},
		{"service", "ssh", "restart"},
		{"/etc/init.d/ssh", "restart"},
	}
	restarted := false
	for _, cmd := range restartCmds {
		if err := RunCmdSilent(cmd[0], cmd[1:]...); err == nil {
			restarted = true
			break
		}
	}
	if !restarted {
		Warn("SSH service restart may have failed; please check manually")
	}

	Success("SSH configuration updated successfully")
	return nil
}

// RunInitScript 执行系统初始化（Go 实现）
func RunInitScript() {
	Info("Starting system initialization...")

	// 1. 获取 Debian 代号
	codename, err := getDebianCodename()
	if err != nil {
		Warn("Failed to get Debian codename: " + err.Error())
		return
	}
	Info("Detected Debian codename: " + codename)

	// 2. 写入 APT 源
	if err := writeSourcesList(codename); err != nil {
		Warn("Failed to write sources.list: " + err.Error())
		return
	}
	Success("APT sources updated to Huawei Cloud mirror")

	// 3. 更新并安装基础包
	if err := runAptUpdateInstall("wget", "curl", "sudo", "ifenslave"); err != nil {
		Warn("apt operation failed: " + err.Error())
		return
	}
	Success("Required packages installed")

	// 4. 配置 SSH
	if err := setupSSH(); err != nil {
		Warn("SSH setup failed: " + err.Error())
		return
	}

	Success("System initialization completed successfully")
}

// ================== 更新自身（国际化版） ==================

// updateSelf 从远程下载最新版本并替换自身
func updateSelf() {
	const (
		remoteURL = "https://bash.niteng.net/netcfg"
		localPath = "/usr/local/bin/netcfg"
	)

	Info(fmt.Sprintf(T("update_self_start"), remoteURL))

	// 优先使用 wget，失败则尝试 curl
	var err error
	var out string

	// 先尝试 wget
	if CommandExists("wget") {
		out, err = RunCmd("wget", "-q", "-O", localPath+".tmp", remoteURL)
		if err == nil {
			goto install
		}
		Warn(T("update_self_wget_fail"))
	}

	// 再尝试 curl
	if CommandExists("curl") {
		out, err = RunCmd("curl", "-s", "-o", localPath+".tmp", remoteURL)
		if err == nil {
			goto install
		}
		Warn(fmt.Sprintf(T("update_self_curl_fail"), err.Error()))
		if out != "" {
			fmt.Println(out)
		}
		return
	}

	Error(T("update_self_no_tool"))
	return

install:
	// 赋予执行权限
	if err := os.Chmod(localPath+".tmp", 0755); err != nil {
		Error(fmt.Sprintf(T("update_self_chmod_fail"), err.Error()))
		return
	}

	// 覆盖原文件
	if err := os.Rename(localPath+".tmp", localPath); err != nil {
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