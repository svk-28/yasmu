# YASMU — Yet Another System Monitoring Utility

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

A utility for monitoring system integrity and website availability. It periodically collects system state snapshots, compares them with previous ones, and sends notifications when changes are detected.

---

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Command Line](#command-line)
- [Configuration](#configuration)
- [How It Works](#how-it-works)
- [Usage Examples](#usage-examples)
- [Diagnostics](#diagnostics)
- [Files and Directories](#files-and-directories)
- [Building Packages](#building-packages)
- [License](#license)
- [Links](#links)

---

## Features

### File System Monitoring

Allows tracking changes in the file system, such as creation, modification, deletion of files, and BLAKE3 content hash. Through the configuration file, you can specify which directories to monitor and which operations to track.

### User Monitoring

The program reads the (local) user list and monitors user creation and deletion, changes to UID, home directory, and group.

Additionally, for each user (both system and regular), it checks for the presence of the `.ssh` directory in the home directory and SSH keys in `authorized_keys`, and tracks key changes.

### Process Monitoring

After YASMU starts, all running processes are checked, and changes to the process ID (PID), parent process ID (PPID), owner, and binary path are tracked. The program also pays attention to deleted binaries of running processes. There is an option to filter both kernel processes and arbitrary processes.

### Open Port (Socket) Monitoring

Network ports are tracked: TCP 'LISTEN', UDP sockets bound to a local address.
That is, after startup, any changes in "listening" network processes (within the specified scope) will be tracked.

Through the configuration file, you can set filters by ports and processes.

### Connection Monitoring

Monitoring of established connections. Processes that have established connections with external (relative to the target) nodes are tracked.

A process is described by simple rules in the configuration file. There you can specify which processes to react to and which not, set filters by addresses, ports, connection state, etc.

### System Service (systemd unit) Monitoring

YASMU implements monitoring of system services (via D-Bus) — current state, state changes, i.e., enabling, disabling a service, abnormal termination of a running daemon, startup (LoadState, ActiveState, SubState, MainPID).

You can monitor all services or exclude some from monitoring, or monitor only the required ones. All of this is configured in the configuration file.

### URL Availability Check (web check)

The program polls the addresses specified in the list and, if the response is other than 2xx, generates an alarm (status change).

### Notification

At the end of the check cycle, the program can send the result to the specified destinations by sending a message.

- E-mail — sends a message to the specified address via a mail server (SMTP).
- Matrix — sends a message to the specified channel on the specified server via the Matrix messaging protocol.
- webhook — the message is sent to any HTTP resource. The message can be either plain text or in JSON format (the format is set in the config).

### Operation Mode

YASMU can be run both in the console and as a service (daemon).
When running as a service, both when the daemon is stopped by standard means (system manager or yasmu command-line options) and when run once, the current state of checks will be saved to disk and loaded into memory on the next start.

---

## Installation

### Debian / Ubuntu

1. Install the package:

    sudo apt install ./yasmu_1.0.0_amd64.deb

   If `apt install ./file.deb` is not supported:

    sudo dpkg -i yasmu_1.0.0_amd64.deb
    sudo apt install -f

2. Edit the config:

    sudo vi /etc/yasmu/config.yaml

3. Start the service:

    sudo systemctl enable --now yasmu

4. Check:

    sudo systemctl status yasmu
    sudo journalctl -u yasmu -f

### RHEL / CentOS / Oracle Linux

1. Install the package:

    sudo dnf install ./yasmu-1.0.0-1.el8.x86_64.rpm

   Or:

    sudo rpm -ivh yasmu-1.0.0-1.el8.x86_64.rpm

2. Edit the config:

    sudo vi /etc/yasmu/config.yaml

3. Start the service:

    sudo systemctl enable --now yasmu

4. Check:

    sudo systemctl status yasmu
    sudo journalctl -u yasmu -f

### From Source

    git clone git.nuk-svk.ru/svk/yasmu.git
    cd yasmu
    make build
    sudo make install

---

## Quick Start

    # Create the default config
    yasmu -make-config -config /etc/yasmu/config.yaml

    # Edit the config
    vi /etc/yasmu/yasmu.yaml

    # Run a single scan in the console
    yasmu -config /etc/yasmu/yasmu.yaml -run-once

    # Run in the console
    yasmu -config /etc/yasmu/yasmu.yaml

    # Run as a daemon
    yasmu -config /etc/yasmu/yasmu.yaml -daemon-start

    # Stop the daemon
    yasmu -config /etc/yasmu/yasmu.yaml -daemon-stop

---

## Command Line

| Flag            | Description
| ---             | ---
| `-config PATH`  | Path to the configuration file. Defaults to `config.yaml`.
| `-run-once`     | Run all checks once (the check state will be saved) and exit.
| `-daemon-start` | Start as a daemon.
| `-daemon-stop`  | Stop the daemon.
| `-make-config`  | Create a configuration file from the template.
| `-version`      | Show the version and exit.
| `-h`, `-help`   | Show help.

---

## Configuration

### `daemon` Section

| Field           | Type   | Description
| ---             | ---    | ---
| `check_timeout` | int    | Pause between iterations, in seconds. `<= 0` → 30 seconds.
| `pid_file`      | string | Path to the PID file.
| `log_file`      | string | Path to the log file.
| `work_dir`      | string | Daemon working directory.

    daemon:
      check_timeout: 60
      pid_file: /run/yasmu/yasmu.pid
      log_file: /var/log/yasmu/yasmu.log
      work_dir: /var/lib/yasmu

### `checks` Section

Each subsection is a separate check module. The flag `enable: false` skips the check.

#### `checks.files`

    checks:
      files:
        enable: true
        paths:
          - path: /etc
            watch: [create, modify, delete]
            hash: blake3
            exclude:
              - /etc/mtab

| Field     | Type     | Description
| ---       | ---      | ---
| `path`    | string   | Traversal root (recursive).
| `watch`   | []string | `create`, `modify`, `delete`. Empty — all.
| `hash`    | string   | `blake3`, or empty.
| `exclude` | []string | Subpaths to skip.

Compared: type, size, permissions, owner, group, modification time, hash.

#### `checks.users`

    checks:
      users:
        enable: true

Compared: UID, primary GID, home, groups, `~/.ssh/authorized_keys`.

#### `checks.processes`

    checks:
      processes:
        enable: true
        exclude:
          - some-process-1
          - some-process-2
        kernel_threads:
          check_ppid: false
          prefixes:
            - kworker
            - psimon

| Field                                                                   | Type     | Description
| ---                                                                     | ---      | ---
| `exclude`                                                               | []string | Skip processes if the Exe file contains the specified string.
| `check_ppid`                                                            | bool     | Enables the filter by parent process ID, 0 (swapper) or 2 (kthreadd),
| to filter kernel threads.                                               |          |
| `prefixes`                                                              | []string | List of name prefixes. Used as a fallback when PPID doesn't help
| (e.g., on non-Linux systems, or when /proc/<pid>/stat is unreadable).   |          | 
| Applied only when Exe is NOT available — a real process with a matching |          |
| name but a valid Exe will NOT be filtered.                              |          |

Compared: PID, PPID, Name, Owner, Exe, Deleted, Suspicious.

#### `checks.ports`

    checks:
      ports:
        enable: true
        established: false
        exclude_ports: 
          - 10024
        exclude_processes:
          - some-process
        ignore_pid_changes: false

| Field                | Type     | Description
| ---                  | ---      | ---
| `established`        | bool     | Include ESTABLISHED TCP.
| `exclude_ports`      | []uint32 | Ports to skip.
| `exclude_processes`  | []string | Skip processes if the Exe file contains the specified string.
| `ignore_pid_changes` | bool     | Ignore PID/Exe changes.

#### `checks.services`

    checks:
      services:
        enable: true
        units:
          - sshd.service
          - nginx.service
        exclude:
          - NetworkManager-dispatcher.service

| Name      | Type     | Description
| ---       | ---      | ---
| `units`   | []string | List of services to monitor. If empty, then all services.
| `exclude` | []string | Filter (list) for exclusion.

Compared: LoadState, ActiveState, SubState, MainPID.

#### `checks.connections`

    checks:
      connections:
        enable: true
        rules:
          - name: "SSH from allowed networks"
            proto: tcp
            port: 22
            state: ESTABLISHED
            allowed_remote:
              - 192.168.0.0/16
          - name: "shell must not have network"
            proto: tcp
            state: ESTABLISHED
            processes:
              - bash
              - sh
              - ksh
            alert_on_match: true

| Name             | Type     | Description
| ---              | ---      | ---
| `name`           | string   | Rule name.
| `proto`          | string   | Protocol tcp / udp.
| `port`           | uint32   | Local port.
| `state`          | string   | TCP state.
| `processes`      | []string | Exact basenames.
| `allowed_remote` | []string | List of allowed IPs/CIDRs.
| `alert_on_match` | bool     | Inverts the logic.

#### `checks.web`

    checks:
      web:
        enable: true
        urls:
          - https://server.com
          - https://git.server.com

### `notify` Section

#### `notify.email`

    notify:
      email:
        enable: true
        server: smtp.yandex.ru
        port: 465
        user: user@yandex.ru
        password: "app-password"
        mail_to: admin@domain.com
        mail_from: user@yandex.ru
        mail_subject: "YASMU alert from %HOSTNAME%"

Ports 465 (implicit TLS) and 587 (STARTTLS).

#### `notify.matrix`

    notify:
      matrix:
        enable: true
        server: https://matrix.example.com
        user: "@user:example.com"
        password: "password"
        access_token: ""
        room_id: "!abc:example.com"

#### `notify.webhook`

    notify:
      webhook:
        enable: true
        url: https://web.server.com/hook
        user: user
        password: "password"
        message:
          format: json
          template: '{"message":"Alert from %HOSTNAME%: %MESSAGE%"}'

Macros: `%HOSTNAME%`, `%MESSAGE%`.

---

## Usage Examples

### Minimal Config

    daemon:
      check_timeout: 300
      work_dir: /var/lib/yasmu

    checks:
      files:
        enable: true
        paths:
          - path: /etc
            watch: [modify, delete]
            hash: blake3

    notify:
      email:
        enable: true
        server: smtp.example.com
        port: 587
        user: alerts@example.com
        password: "secret"
        mail_to: admin@example.com
        mail_from: alerts@example.com
        mail_subject: "YASMU: %HOSTNAME%"

---

## Diagnostics

### Logs

- Systemd: journalctl -u yasmu -f
- File: /var/log/yasmu.log

The log file path is set in the config in the 'daemon' section.

### Common Issues

### Debugging

---

## Files and Directories

- /usr/bin/yasmu — the executable file
- /etc/yasmu/config.yaml — configuration file
- /lib/systemd/system/yasmu.service — Systemd unit (Debian)
- /usr/lib/systemd/system/yasmu.service — Systemd unit (RPM)
- /var/lib/yasmu/ — State
- /var/log/yasmu.log — Log
- /run/yasmu.pid — PID file

---

## Building Packages

### Debian (.deb)

    make deb
    sudo dpkg -i build/yasmu_1.0.0_amd64.deb

### RHEL / CentOS / Oracle (.rpm)

    make rpm
    sudo rpm -ivh ~/rpmbuild/RPMS/x86_64/yasmu-1.0.0-1.el8.x86_64.rpm

### Binary

    make build
    # or
	go build -ldflags "-s -w" -o bin/yasmu main.go config.go

### Static Binary

    make static
    # or
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -ldflags "-s -w" -o bin/yasmu main.go config.go

---

## License

GPL v3. See the LICENSE file in the repository.

---

## Links

- Home page: https://nuk-svk.ru
