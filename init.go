package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ================== 初始化系统（Go 实现） ==================

const (
	sshdConfigPath = "/etc/ssh/sshd_config"
	sshDir         = "/root/.ssh"
	authKeysPath   = sshDir + "/authorized_keys"
	// 可通过该文件或 NETCFG_SSH_PUBKEY 环境变量覆盖内置公钥
	sshPubkeyFile = "/etc/netcfg/ssh_pubkey"
)

// defaultAuthorizedKey 内置默认公钥（仅为兼容原行为，建议用上面两种方式显式指定）
var defaultAuthorizedKey = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDC8s1el1MUWsPgmSmJ1npXoiEkIdBlrBk5QbVm5/3USPUGt1GQ9XAvyufuDklLjK1Gz7IGSS0wu3iZH9u2baGvaHUxQZaOYgFf24nIUe4kv/Rba+4zWI3gajZk2WKJV1dr3diGHs9JLjeoX4ZiszRSAZi+zxs8BWj/7V2X5RoeaUwGvCdvpCAwET7N7Jdu9/WBG5ZoK7ypp1+B5EEc8TlLse5PcRdYnLh3arLSt/FDL8NpcjUgRgPTGUmT53cGvo8RXuVfE0W9+9JAO1b6GQFR8rBN3gkhHNSx5hGQeLHYN4WNuUo8/eTJ6hRYFJNG1kFEtaB8IX9WEATwFiso800TsthTa0EYVdHbatkGkDjBJBWeF8yc4Tg4af+FEigH7hYfEsLxBejcFBmFmaeBAx4RGwzGlX4J8xVvPoW7Yul0Ln2hTUwRwG3pZ0xcqX/CMj8BfvUbYNSLOqwInUspmwRfn6dxayMpcg9GEkLyM+VwseVmV+YQ0gKrTYwd2rCzKN2PinJVSkP8i2mA7+bnESELjoz9VLHucXT+TOVbLJsxRUnoIYQe6mw/bjAYM79E/8IOqafSaxuxMQ6NubL12K3CY2lC3H0VTi2+KoHCUO0ZEvrez0X5KjwGPreaa9CCygqF5497iGA88sVgTuD8KCPZEJmJEulYIeZ2QIAlnOBnaw== bi4nbn@qq.com`

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
	BackupFile("/etc/apt/sources.list")
	if err := os.WriteFile("/etc/apt/sources.list", []byte(content), 0644); err != nil {
		return err
	}
	// 处理 DEB822 格式源（如果存在）
	if _, err := os.Stat("/etc/apt/sources.list.d/debian.sources"); err == nil {
		data, err := os.ReadFile("/etc/apt/sources.list.d/debian.sources")
		if err == nil {
			BackupFile("/etc/apt/sources.list.d/debian.sources")
			newData := strings.ReplaceAll(string(data), "deb.debian.org", "mirrors.huaweicloud.com")
			_ = os.WriteFile("/etc/apt/sources.list.d/debian.sources", []byte(newData), 0644)
		}
	}
	return nil
}

// runAptUpdateInstall 执行 apt update 并安装指定包
func runAptUpdateInstall(packages ...string) error {
	Info("Running apt update...")
	if out, err := RunCmd("apt", "update"); err != nil {
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

// resolveSSHPublicKey 按优先级解析要写入 authorized_keys 的公钥
func resolveSSHPublicKey() (key, source string) {
	if v := strings.TrimSpace(os.Getenv("NETCFG_SSH_PUBKEY")); v != "" {
		return v, "NETCFG_SSH_PUBKEY"
	}
	if data, err := os.ReadFile(sshPubkeyFile); err == nil {
		if v := strings.TrimSpace(string(data)); v != "" {
			return v, sshPubkeyFile
		}
	}
	return defaultAuthorizedKey, "built-in default"
}

// isValidSSHPublicKey 粗略校验公钥格式，避免把无效内容写进 authorized_keys
func isValidSSHPublicKey(key string) bool {
	fields := strings.Fields(key)
	if len(fields) < 2 {
		return false
	}
	for _, p := range []string{"ssh-rsa", "ssh-ed25519", "ssh-dss", "ecdsa-sha2-", "sk-ssh-ed25519@", "sk-ecdsa-sha2-"} {
		if strings.HasPrefix(fields[0], p) {
			return true
		}
	}
	return false
}

// ensureAuthorizedKey 以“追加去重”方式写入公钥，保留服务器上已有的全部密钥。
// 返回：除本工具公钥外的其它密钥数量、是否新增、错误。
// 注意：这里刻意不把“上一次由本工具下发的公钥”计入其它密钥。
// 文件内容按原样保留（含注释与空行），只在末尾追加缺失的公钥。
func ensureAuthorizedKey(path, key string) (int, bool, error) {
	var raw string
	if data, err := os.ReadFile(path); err == nil {
		raw = string(data)
	} else if !os.IsNotExist(err) {
		return 0, false, err
	}

	keyFields := strings.Fields(key)
	base := keyFields[0] + " " + keyFields[1]

	others := 0
	found := false
	for _, line := range strings.Split(raw, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Fields(l)
		if len(f) >= 2 && f[0]+" "+f[1] == base {
			found = true
			continue
		}
		others++
	}
	if found {
		return others, false, nil
	}

	content := raw
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += key + "\n"
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return 0, false, err
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return 0, false, err
	}
	_ = os.Chmod(path, 0600)
	return others, true, nil
}

// fileHasAuthorizedKey 确认指定公钥（按“类型 + 密钥体”比对）确实存在于文件中，
// 用于关闭密码登录前的兜底校验。
func fileHasAuthorizedKey(path, key string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	kf := strings.Fields(key)
	if len(kf) < 2 {
		return false
	}
	base := kf[0] + " " + kf[1]
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) >= 2 && f[0]+" "+f[1] == base {
			return true
		}
	}
	return false
}

// setupSSH 配置 SSH（公钥追加 + sshd_config），失败时使用真实备份路径回滚
func setupSSH() error {
	if strings.TrimSpace(os.Getenv("NETCFG_SKIP_SSH_HARDENING")) != "" {
		Info(T("ssh_hardening_skipped"))
		return nil
	}

	pubKey, source := resolveSSHPublicKey()
	if !isValidSSHPublicKey(pubKey) {
		return fmt.Errorf("invalid SSH public key from %s", source)
	}
	Info(fmt.Sprintf(T("ssh_pubkey_source"), source))

	// 备份原始配置（记录真实备份路径，供校验失败时回滚）
	backupPath := BackupFile(sshdConfigPath)
	if _, err := os.Stat(authKeysPath); err == nil {
		BackupFile(authKeysPath)
	}

	otherKeys, added, err := ensureAuthorizedKey(authKeysPath, pubKey)
	if err != nil {
		return fmt.Errorf("failed to update authorized_keys: %v", err)
	}
	if !added {
		Success(T("ssh_key_already"))
	} else {
		Success(T("ssh_key_injected"))
	}
	if otherKeys > 0 {
		Info(fmt.Sprintf(T("ssh_other_keys_found"), otherKeys))
	}

	// 按设计关闭密码登录，仅允许公钥认证。
	// 兜底保护：若公钥最终并未落入 authorized_keys（例如写入异常），
	// 则强制保留密码登录，避免把自己彻底锁在服务器外面。
	passwordLine := "PasswordAuthentication no"
	switch {
	case !fileHasAuthorizedKey(authKeysPath, pubKey):
		passwordLine = "PasswordAuthentication yes"
		Warn(T("ssh_pw_keep_warn"))
	case strings.TrimSpace(os.Getenv("NETCFG_KEEP_PASSWORD_AUTH")) != "":
		passwordLine = "PasswordAuthentication yes"
		Warn(T("ssh_pw_keep_env"))
	default:
		Info(T("ssh_pw_disabled"))
	}

	sshdConfigContent := fmt.Sprintf(`# ==================SSCLOUD SSHD CONFIGURATION==================
AllowUsers root
PermitRootLogin prohibit-password
PubkeyAuthentication yes
%s
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
AllowTcpForwarding yes
X11Forwarding no
PermitTunnel no
GatewayPorts no
PermitUserEnvironment no
PrintMotd no
AcceptEnv LANG LC_*
Subsystem sftp /usr/lib/openssh/sftp-server

# 放在末尾：sshd 对同一关键字取"首次出现"的值，因此 drop-in 无法覆盖上面的策略，
# 但云镜像放在 sshd_config.d/ 里的 Port / ListenAddress 等仍能生效，
# 不会因为本模板没写 Port 而被改回 22。
Include /etc/ssh/sshd_config.d/*.conf
`, passwordLine)

	if err := os.WriteFile(sshdConfigPath, []byte(sshdConfigContent), 0644); err != nil {
		return fmt.Errorf("failed to write sshd_config: %v", err)
	}

	// 校验配置，失败则从真实备份路径恢复
	if out, err := RunCmd("sshd", "-t"); err != nil {
		if backupPath != "" {
			if _, statErr := os.Stat(backupPath); statErr == nil {
				if restoreErr := CopyFile(backupPath, sshdConfigPath); restoreErr == nil {
					Warn(fmt.Sprintf(T("ssh_restore_ok"), backupPath))
					return fmt.Errorf("sshd config test failed: %v, output: %s", err, out)
				}
			}
		}
		Warn(fmt.Sprintf(T("ssh_restore_fail"), sshdConfigPath))
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
	// 写入持久化标记文件
	markInitialized()
}
