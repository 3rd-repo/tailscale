// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func init() {
	installSystemDaemon = installSystemDaemonLinux
	uninstallSystemDaemon = uninstallSystemDaemonLinux
}

const (
	systemdServicePath = "/etc/systemd/system/tailscaled.service"
	defaultTargetBin   = "/usr/local/bin/tailscaled"
	serviceName        = "tailscaled.service"
)

func buildExecStart(targetBin string, args []string) string {
	if len(args) == 0 {
		return fmt.Sprintf("%s --state=/var/lib/tailscale/tailscaled.state --socket=/run/tailscale/tailscaled.sock --port=${PORT} $FLAGS", targetBin)
	}
	parts := []string{targetBin}
	hasSocket := false
	hasState := false
	for _, a := range args {
		if strings.HasPrefix(a, "-socket") || strings.HasPrefix(a, "--socket") {
			hasSocket = true
		}
		if strings.HasPrefix(a, "-state") || strings.HasPrefix(a, "--state") {
			hasState = true
		}
		if strings.ContainsAny(a, " \t\n\"'") {
			parts = append(parts, fmt.Sprintf("%q", a))
		} else {
			parts = append(parts, a)
		}
	}
	if !hasSocket {
		parts = append(parts, "--socket=/run/tailscale/tailscaled.sock")
	}
	if !hasState {
		parts = append(parts, "--state=/var/lib/tailscale/tailscaled.state")
	}
	return strings.Join(parts, " ")
}

func buildSystemdUnit(targetBin string, args []string) string {
	execStart := buildExecStart(targetBin, args)
	return fmt.Sprintf(`[Unit]
Description=Tailscale node agent
Documentation=https://tailscale.com/docs/
Wants=network-pre.target
After=network-pre.target NetworkManager.service systemd-resolved.service

[Service]
EnvironmentFile=-/etc/default/tailscaled
EnvironmentFile=-/etc/sysconfig/tailscaled
ExecStart=%s
ExecStopPost=%s --cleanup

Restart=on-failure

RuntimeDirectory=tailscale
RuntimeDirectoryMode=0755
StateDirectory=tailscale
StateDirectoryMode=0700
CacheDirectory=tailscale
CacheDirectoryMode=0750
Type=notify

[Install]
WantedBy=multi-user.target
`, execStart, targetBin)
}

func installSystemDaemonLinux(args []string) (err error) {
	if os.Geteuid() != 0 {
		return errors.New("install-system-daemon must be run as root (or with sudo)")
	}

	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl not found in PATH: %w", err)
	}

	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return errors.New("systemd does not appear to be running on this system (/run/systemd/system not found)")
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	targetBin := defaultTargetBin
	if exe == "/usr/sbin/tailscaled" || exe == "/usr/bin/tailscaled" || exe == defaultTargetBin {
		targetBin = exe
	} else {
		same, _ := sameFile(exe, defaultTargetBin)
		if !same {
			if err := copyBinary(exe, defaultTargetBin); err != nil {
				return fmt.Errorf("failed to copy %s to %s: %w", exe, defaultTargetBin, err)
			}
		}
		targetBin = defaultTargetBin

		// If the companion "tailscale" CLI binary exists in the same directory, also copy it.
		cliSrc := filepath.Join(filepath.Dir(exe), "tailscale")
		if fi, err := os.Stat(cliSrc); err == nil && !fi.IsDir() {
			cliDst := "/usr/local/bin/tailscale"
			sameCLI, _ := sameFile(cliSrc, cliDst)
			if !sameCLI {
				_ = copyBinary(cliSrc, cliDst)
			}
		}
	}

	unitContent := buildSystemdUnit(targetBin, args)

	if err := os.WriteFile(systemdServicePath, []byte(unitContent), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", systemdServicePath, err)
	}

	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %v\n%s", err, out)
	}

	if out, err := exec.Command("systemctl", "enable", "--now", serviceName).CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable --now %s failed: %v\n%s", serviceName, err, out)
	}

	fmt.Printf("Tailscale systemd service successfully installed and started (%s).\n", serviceName)
	return nil
}

func uninstallSystemDaemonLinux(args []string) (ret error) {
	if os.Geteuid() != 0 {
		return errors.New("uninstall-system-daemon must be run as root (or with sudo)")
	}

	if _, err := exec.LookPath("systemctl"); err == nil {
		out, err := exec.Command("systemctl", "stop", serviceName).CombinedOutput()
		if err != nil && !strings.Contains(string(out), "not loaded") {
			fmt.Printf("systemctl stop %s: %v, %s\n", serviceName, err, out)
			ret = err
		}
		out, err = exec.Command("systemctl", "disable", serviceName).CombinedOutput()
		if err != nil && !strings.Contains(string(out), "not loaded") {
			fmt.Printf("systemctl disable %s: %v, %s\n", serviceName, err, out)
			if ret == nil {
				ret = err
			}
		}
	}

	if err := os.Remove(systemdServicePath); err != nil {
		if !os.IsNotExist(err) {
			if ret == nil {
				ret = err
			}
		}
	}

	if _, err := exec.LookPath("systemctl"); err == nil {
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}

	// Remove default target binary if it was copied there and is not a symlink.
	if !isSymlink(defaultTargetBin) {
		_ = os.Remove(defaultTargetBin)
	}

	fmt.Println("Tailscale systemd service successfully uninstalled and stopped.")
	return ret
}

func copyBinary(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	tmpBin := dst + ".tmp"
	f, err := os.Create(tmpBin)
	if err != nil {
		return err
	}
	srcf, err := os.Open(src)
	if err != nil {
		f.Close()
		return err
	}
	_, err = io.Copy(f, srcf)
	srcf.Close()
	if err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpBin, 0755); err != nil {
		return err
	}
	if err := os.Rename(tmpBin, dst); err != nil {
		return err
	}
	return nil
}

func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && (fi.Mode()&os.ModeSymlink == os.ModeSymlink)
}

func sameFile(path1, path2 string) (bool, error) {
	dst1, err := filepath.EvalSymlinks(path1)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("EvalSymlinks(%s): %w", path1, err)
	}
	dst2, err := filepath.EvalSymlinks(path2)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("EvalSymlinks(%s): %w", path2, err)
	}
	return dst1 == dst2, nil
}
