// *******************************************************
// YASMU - Yet Another System Monitoring Utility
// *******************************************************
// Author: Sergey Kalinin
// Home page: https://nuk-svk.ru
// License: GPL v3
// *******************************************************
// systemd unit installation/removal.
// *******************************************************

package config

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

// SystemdUnitTemplate — systemd unit file template.

//  %DESCRIPTION% — short service description
//  %BIN%         — full path to the executable
//  %CONFIG%      — full path to the configuration file
//  %USER%        — user the service runs as
//  %GROUP%       — group
//  %WORKDIR%     — working directory
var SystemdUnitTemplate = `[Unit]
Description=%DESCRIPTION%
After=network-online.target
Wants=network-online.target

[Service]
Type=forking
PIDFile=%PIDFILE%
User=%USER%
Group=%GROUP%
WorkingDirectory=%WORKDIR%
ExecStart=%BIN% -config %CONFIG% -daemon-start
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=10
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`

// SystemdInstallOptions holds parameters for systemd unit installation/removal.
type SystemdInstallOptions struct {
    // Full path to the executable (required for installation).
    BinaryPath string
    // Full path to the configuration file (required for installation).
    ConfigPath string
    // Directory for unit files. Defaults to /etc/systemd/system.
    UnitDir string
    // Unit file name without extension. Defaults to "yasmu".
    UnitName string
    // User to run the service as. Defaults to "root".
    User string
    // Group to run the service as. Defaults to "root".
    Group string
    // Working directory. Defaults to "/var/lib/yasmu".
    WorkDir string
    // Daemon PID file (for Type=forking). Defaults to "/run/yasmu.pid".
    // Must match daemon.pid_file in the config.
    PIDFile string
    // Description. Defaults to "YASMU system monitoring utility".
    Description string
    // Overwrite an existing unit file.
    Overwrite bool
    // Run `systemctl daemon-reload` after install/remove.
    DaemonReload bool
    // Run `systemctl enable` on install / `disable` on remove.
    Enable bool
}

func (options *SystemdInstallOptions) applyDefaults() {
    if options.UnitDir == "" {
        options.UnitDir = "/etc/systemd/system"
    }
    if options.UnitName == "" {
        options.UnitName = "yasmu"
    }
    if options.User == "" {
        options.User = "root"
    }
    if options.Group == "" {
        options.Group = "root"
    }
    if options.WorkDir == "" {
        options.WorkDir = "/var/lib/yasmu"
    }
    if options.Description == "" {
        options.Description = "YASMU - yet another system monitoring utility"
    }
    if options.PIDFile == "" {
        options.PIDFile = "/var/run/yasmu.pid"
    }
}

func (options *SystemdInstallOptions) unitPath() string {
    return filepath.Join(options.UnitDir, options.UnitName+".service")
}

func (options *SystemdInstallOptions) validate() error {
    if options.BinaryPath == "" {
        return fmt.Errorf("binary path is empty")
    }
    if options.ConfigPath == "" {
        return fmt.Errorf("config path is empty")
    }
    if _, err := os.Stat(options.BinaryPath); err != nil {
        return fmt.Errorf("binary %q: %w", options.BinaryPath, err)
    }
    if _, err := os.Stat(options.ConfigPath); err != nil {
        return fmt.Errorf("config %q: %w", options.ConfigPath, err)
    }

    if err := os.MkdirAll(options.UnitDir, 0755); err != nil {
        return fmt.Errorf("prepare unit dir %q: %w (run as root?)", options.UnitDir, err)
    }
    probe := filepath.Join(options.UnitDir, ".yasmu-write-test")
    f, err := os.Create(probe)
    if err != nil {
        return fmt.Errorf("unit dir %q is not writable: %w (run as root?)", options.UnitDir, err)
    }
    f.Close()
    _ = os.Remove(probe)

    if err := os.MkdirAll(options.WorkDir, 0755); err != nil {
        return fmt.Errorf("prepare workdir %q: %w", options.WorkDir, err)
    }
    return nil
}

func renderUnit(options *SystemdInstallOptions) string {
    r := strings.NewReplacer(
        "%DESCRIPTION%", options.Description,
        "%BIN%", options.BinaryPath,
        "%CONFIG%", options.ConfigPath,
        "%USER%", options.User,
        "%GROUP%", options.Group,
        "%WORKDIR%", options.WorkDir,
        "%PIDFILE%", options.PIDFile,
    )
    return r.Replace(SystemdUnitTemplate)
}

func InstallSystemdUnit(opts SystemdInstallOptions) error {
    opts.applyDefaults()

    if err := opts.validate(); err != nil {
        return err
    }

    unitPath := opts.unitPath()

    if _, err := os.Stat(unitPath); err == nil && !opts.Overwrite {
        return fmt.Errorf("unit %q already exists (use overwrite to replace)", unitPath)
    }

    content := renderUnit(&opts)

    tmp, err := os.CreateTemp(opts.UnitDir, ".yasmu-unit-*.tmp")
    if err != nil {
        return fmt.Errorf("create temp unit: %w", err)
    }
    tmpName := tmp.Name()
    defer os.Remove(tmpName)

    if _, err := tmp.WriteString(content); err != nil {
        tmp.Close()
        return fmt.Errorf("write temp unit: %w", err)
    }
    if err := tmp.Chmod(0644); err != nil {
        tmp.Close()
        return fmt.Errorf("chmod temp unit: %w", err)
    }
    if err := tmp.Close(); err != nil {
        return fmt.Errorf("close temp unit: %w", err)
    }
    if err := os.Rename(tmpName, unitPath); err != nil {
        return fmt.Errorf("install unit %q: %w", unitPath, err)
    }

    fmt.Printf("Systemd unit installed: %s\n", unitPath)

    if opts.DaemonReload {
        if err := runSystemctl("daemon-reload"); err != nil {
            return fmt.Errorf("systemctl daemon-reload: %w", err)
        }
    }
    if opts.Enable {
        if err := runSystemctl("enable", opts.UnitName+".service"); err != nil {
            return fmt.Errorf("systemctl enable: %w", err)
        }
        fmt.Printf("Service enabled: %s.service\n", opts.UnitName)
    }
    fmt.Printf("Start it with: systemctl start %s.service\n", opts.UnitName)
    return nil
}

func RemoveSystemdUnit(opts SystemdInstallOptions) error {
    opts.applyDefaults()

    if err := os.MkdirAll(opts.UnitDir, 0755); err != nil {
        return fmt.Errorf("prepare unit dir %q: %w", opts.UnitDir, err)
    }

    unitPath := opts.unitPath()

    if opts.Enable {
        if err := runSystemctl("disable", opts.UnitName+".service"); err != nil {
            fmt.Printf("warn: systemctl disable: %v\n", err)
        }
    }

    if err := os.Remove(unitPath); err != nil {
        if !os.IsNotExist(err) {
            return fmt.Errorf("remove unit %q: %w", unitPath, err)
        }
        fmt.Printf("Systemd unit not present: %s\n", unitPath)
    } else {
        fmt.Printf("Systemd unit removed: %s\n", unitPath)
    }

    if opts.DaemonReload {
        if err := runSystemctl("daemon-reload"); err != nil {
            return fmt.Errorf("systemctl daemon-reload: %w", err)
        }
    }
    return nil
}

func RemoveFileIfExists(path string) (bool, error) {
    if path == "" {
        return false, nil
    }
    if _, err := os.Stat(path); err != nil {
        if os.IsNotExist(err) {
            return false, nil
        }
        return false, err
    }
    if err := os.Remove(path); err != nil {
        return false, err
    }
    return true, nil
}

func RemoveDirIfExists(path string) (bool, error) {
    if path == "" {
        return false, nil
    }
    if _, err := os.Stat(path); err != nil {
        if os.IsNotExist(err) {
            return false, nil
        }
        return false, err
    }
    if err := os.RemoveAll(path); err != nil {
        return false, err
    }
    return true, nil
}

func runSystemctl(args ...string) error {
    cmd := exec.Command("systemctl", args...)
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
    }
    return nil
}
