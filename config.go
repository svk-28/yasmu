// *******************************************************
// YASMU - Yet Another System Monitoring Utility
// The utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
// Home page: https://nuk-svk.ru
// License: GPL v3
// *******************************************************
// Configuration management module.
// Creating/reading a configuration file.
// Displaying settings on the screen.
// *******************************************************

package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ConfigTemplate — Config file template
var ConfigTemplate = `# =============================================================================
# YASMU configuration file.
# =============================================================================

# -----------------------------------------------------------------------------
# daemon — daemon mode parameters (launched with the -daemon flag).
# -----------------------------------------------------------------------------
daemon:
  # Pause between check iterations, in seconds.
  check_timeout: 60
  # PID file for the daemon.
  pid_file: yasmu.pid
  # Log file. Relative to work_dir unless absolute.
  log_file: yasmu.log
  # Working directory of the daemon. After daemonization the process chdirs here.
  # Relative paths in the config are resolved against it.
  work_dir: ./

# -----------------------------------------------------------------------------
# checks — which checks to run on each iteration.
# Every section has its own "enable" flag; when false the check is skipped.
# -----------------------------------------------------------------------------
checks:
  # --- connections -----------------------------------------------------------
  # Rule-based network connections monitoring.
  # Rule fields:
  #   name            — rule name shown in the report (required)
  #   proto           — "tcp" / "udp" (empty = any)
  #   port            — local port (0 = any)
  #   state           — TCP state: ESTABLISHED, TIME_WAIT, ... (empty = any)
  #   processes       — list of exact process basenames (empty = any process)
  #                     Compared against "basename /proc/<pid>/exe".
  #                     Case-insensitive. NOT substrings — exact match.
  #   allowed_remote  — list of IPs / CIDRs that are allowed for this rule.
  #                     Any connection from a remote NOT in the list → violation.
  #                     Ignored when alert_on_match: true.
  #   alert_on_match  — if true, the rule describes FORBIDDEN traffic:
  #                     any match → violation, allowed_remote is ignored.
  #
  # Two kinds of rules:
  #   * allowed_remote (whitelist) — "traffic must come only from these nets"
  #   * alert_on_match: true       — "this must never happen"
  connections:
    enable: false
    rules:
      # 1) SSH must be reachable only from the LAN / office VPN.
      # Any ESTABLISHED connection on port 22 from a remote outside the
      # allowed list is a violation.
      - name: "SSH from allowed networks"
        proto: tcp
        port: 22
        state: ESTABLISHED
        allowed_remote:
          - 192.168.0.0/16
          - 10.0.0.0/8
          - 172.16.0.0/12
          - 127.0.0.0/8

      # 2) PostgreSQL must accept connections only from the LAN.
      - name: "PostgreSQL from LAN only"
        proto: tcp
        port: 5432
        state: ESTABLISHED
        allowed_remote:
          - 10.0.0.0/8
          - 127.0.0.1

      # 3) Interpreters must not open external connections.
      # Any ESTABLISHED connection owned by python/python3/php/... to a remote
      # outside the allowed list is a violation.
      #
      # IMPORTANT: "processes" uses EXACT basename match, not substring.
      #   - "/usr/bin/python3"  matches only "python3", not "python"
      #   - "/usr/bin/python3.12" has basename "python3.12" — add it explicitly
      # Always check the real basename with:
      #   readlink /proc/<pid>/exe
      - name: "Interpreters must not connect externally"
        proto: tcp
        state: ESTABLISHED
        processes:
          - python
          - python3
          - python3.11
          - python3.12
          - php
          - tclsh
          - perl
          - ruby
          - node
          - nodejs
        allowed_remote:
          - 127.0.0.1
          - 10.0.0.0/8

      # 4) Shells must never have network connections.
      # Any ESTABLISHED TCP connection owned by a shell is a violation,
      # regardless of the remote address.
      - name: "shell must not have network connections"
        proto: tcp
        state: ESTABLISHED
        processes:
          - bash
          - zsh
          - ksh
          - sh
          - dash
          - fish
        alert_on_match: true

      # 5) (Optional) Deny outbound DNS-over-TCP from anything except the
      #     system resolver. Useful on locked-down servers.
      # - name: "DNS TCP only from resolver"
      #   proto: tcp
      #   port: 53
      #   state: ESTABLISHED
      #   allowed_remote:
      #     - 127.0.0.1
      #     - 10.0.0.53
  # --- files -----------------------------------------------------------------
  # File system monitoring: file creation, modification, deletion.
  files:
    enable: true
    # List of roots to traverse. Each root has its own set of watched
    # events and exclusions.
    paths:
      - path: /etc
        # If "hash: blake3" is specified, checksums are calculated for every
        # regular file. This detects content changes even when mtime is restored.
        # If omitted or empty — no hashing (faster, but weaker).
        hash: blake3
        # Which events to watch for this root:
        #   create — new files/directories appear
        #   modify — content, permissions, owner or mtime changes
        #   delete — files/directories are removed
        # If watch is omitted or empty, all events are tracked.
        watch: [create, modify, delete]

      - path: /dev
        watch: [create, modify, delete]
        # Subpaths to skip together with their entire contents.
        # Directories are not traversed into (SkipDir).
        # Matching is by exact path or by prefix with a separator:
        #   /dev/pts    skips both /dev/pts and /dev/pts/0
        #   /dev/ptsX   does NOT match the /dev/pts exclusion
        exclude:
          - /dev/pts
          - /dev/tty
        - path: /etc
          watch: [create, modify, delete]
          hash: blake3
        - path: /dev
          watch: [create, delete]
          exclude:
            - /dev/pts
            - /dev/tty
        # users crontab
        - path: /var/spool/cron
          watch: [create, modify, delete]
          hash: blake3
        # Other directories
        - path: /usr/local
          watch: [create, modify, delete]
          hash: blake3
        - path: /usr/bin
          watch: [create, modify, delete]
        - path: /usr/sbin
          watch: [create, modify, delete]
        - path: /root
          watch: [create, modify, delete]
          exclude:
            - /root/.cache
            - /root/.bash_history
        - path: /home
          watch: [create, modify, delete]
          # hash: blake3
          exclude:
            - /home/*/.cache
            - /home/*/.local/share/Trash

  # --- ports -----------------------------------------------------------------
  # Open port checks: what is listening and on which interfaces.
  ports:
    enable: false
    # If true, ESTABLISHED TCP connections are included (very noisy).
    # Leave false unless you really need it.
    established: false

  # --- processes -------------------------------------------------------------
  # Running process monitoring (PID, PPID, name, owner, executable path).
  processes:
    enable: false

    # List of substrings: if a process name contains any of them,
    # the process is not included in the snapshot. Case-insensitive.
    # Example: kworker/13:0-events, kworker/u64:3-... — all excluded.
    exclude:
      - kworker

  # --- services --------------------------------------------------------------
  # systemd service monitoring via D-Bus (LoadState, ActiveState, SubState, MainPID).
  services:
    enable: false

    # List of units to monitor.
    # If the list is empty (or commented out), ALL .service units are collected.
    # If specified, only the listed ones are used.
    # Short form without the suffix is allowed: "sshd" → "sshd.service".
    # units:
    #   - sshd.service
    #   - nginx.service
    #   - postgresql.service

  # --- users -----------------------------------------------------------------
  # User monitoring: UID, home directory, groups, SSH keys.
  users:
    enable: false

  # --- web -------------------------------------------------------------------
  # URL availability checks (HTTP GET, expecting 2xx).
  web:
    enable: false

    # List of URLs to check. Order is preserved.
    urls:
      - https://server.com
      - https://git.server.com


# -----------------------------------------------------------------------------
# notify — send alerts when changes are detected.
# If all three are disabled, reports are only written to the log.
# -----------------------------------------------------------------------------
notify:

  # --- email -----------------------------------------------------------------
  # Send reports by email via SMTP.
  # Both port 465 (implicit TLS) and 587 (STARTTLS) are supported.
  email:
    enable: false
    # SMTP server.
    server: smtp.server.com
    # Port: 465 for implicit TLS, 587 for STARTTLS, 25 for plain.
    port: 465
    # SMTP login.
    user: user
    # Password. Quote it — YAML may parse a numeric password as int.
    password: "12345678"
    # Recipient.
    mail_to: to@domain.com
    # Sender. Usually the same as user.
    mail_from: from@domain.com
    # Email subject. The %HOSTNAME% macro is replaced with the FQDN.
    mail_subject: "Alert message from %HOSTNAME%"

  # --- matrix ----------------------------------------------------------------
  # Send reports to a Matrix room.
  matrix:
    enable: false
    # Homeserver URL.
    server: matrix.server.com
    # Matrix login (full user id, e.g. @user:server.com, or short allowed).
    user: user
    # Matrix password.
    password: "12345678"
    # Access token. If set, it is used instead of user/password.
    access_token: ""
    # Room ID. Starts with "!" for rooms.
    room_id: "!1qu2ueu4383jr4o83"

  # --- webhook ---------------------------------------------------------------
  # Send reports via a POST request to an arbitrary URL.
  webhook:
    enable: false
    # URL to POST to.
    url: https://web.server.com/hook
    # Basic auth: username.
    user: user
    # Basic auth: password.
    password: "12345678"
    # Request body settings.
    message:
        # Body format:
        #   json — Content-Type: application/json, template must be valid JSON
        #   text — Content-Type: text/plain, template is arbitrary text
        format: json
        # Request body template.
        # Supported macros:
        #   %HOSTNAME% — host FQDN
        #   %MESSAGE%  — report text (produced by all checks combined)
        template: '{"message":"Alert message from %HOSTNAME% %MESSAGE%"}'
`

type Config struct {
	Daemon DaemonConfig `yaml:"daemon"`
	Checks ChecksConfig `yaml:"checks"`
	Notify NotifyConfig `yaml:"notify"`
}

type DaemonConfig struct {
	CheckTimeout int    `yaml:"check_timeout"`
	LogFile      string `yaml:"log_file"`
	WorkDir      string `yaml:"work_dir"`
	PidFile      string `yaml:"pid_file"`
}

// type BaseCheck struct {
	// Enable bool `yaml:"enable"`
// }

type ChecksConfig struct {
    Processes     ProcessCheck      `yaml:"processes"`
    Users         UsersCheck        `yaml:"users"`
    Network       NetworkCheck      `yaml:"network"`
    Ports         PortsCheck        `yaml:"ports"`
    Web           WebCheck          `yaml:"web"`
    Files         FilesCheck        `yaml:"files"`
    Services      ServicesCheck     `yaml:"services"`
    Connections   ConnectionsCheck  `yaml:"connections"`
}

type ProcessCheck struct {
	Enable bool `yaml:"enable"`
    Exclude []string `yaml:"exclude"`
}

type UsersCheck struct {
	Enable bool `yaml:"enable"`
}

// убрать
type NetworkCheck struct {
	Enable bool `yaml:"enable"`
}

type ConnectionsCheck struct {
    Enable bool       `yaml:"enable"`
    Rules  []ConnRule `yaml:"rules"`
}

type ConnRule struct {
    Name          string   `yaml:"name"`
    Proto         string   `yaml:"proto"`
    Port          uint32   `yaml:"port"`
    // Process       string   `yaml:"process"`    // оставлено для обратной совместимости
    Processes     []string `yaml:"processes"`  // новое поле
    State         string   `yaml:"state"`
    AllowedRemote []string `yaml:"allowed_remote"`
    AlertOnMatch  bool     `yaml:"alert_on_match"`
}

type PortsCheck struct {
	Enable      bool `yaml:"enable"`
	Established bool `yaml:"established"`
}

type WebCheck struct {
	Enable bool `yaml:"enable"`
	URLs      []string `yaml:"urls"`
}

type FilesCheck struct {
	Enable bool        `yaml:"enable"`
	Paths  []PathWatch `yaml:"paths"`
}

type ServicesCheck struct {
    Enable bool     `yaml:"enable"`
    Units  []string `yaml:"units"`
}

type PathWatch struct {
    Path    string   `yaml:"path"`
    Watch   []string `yaml:"watch"`   // "create", "modify", "delete"
    Exclude []string `yaml:"exclude"`
    Hash    string    `yaml:"hash"`
}

type NotifyConfig struct {
	Email   EmailNotify   `yaml:"email"`
	Matrix  MatrixNotify  `yaml:"matrix"`
	Webhook WebhookNotify `yaml:"webhook"`
}

type EmailNotify struct {
	Enable      bool   `yaml:"enable"`
	Server      string `yaml:"server"`
	Port        int    `yaml:"port"`
	User        string `yaml:"user"`
	Password    string `yaml:"password"`
	MailTo      string `yaml:"mail_to"`
	MailFrom    string `yaml:"mail_from"`
	MailSubject string `yaml:"mail_subject"`
}

type MatrixNotify struct {
	Enable       bool   `yaml:"enable"`
	Server       string `yaml:"server"`
	UserName     string `yaml:"user"`
	Password     string `yaml:"password"`
	AccessToken  string `yaml:"access_token"`
	RoomID       string `yaml:"room_id"`
}

type WebhookNotify struct {
    Enable    bool   `yaml:"enable"`
    URL       string `yaml:"url"`
    User      string `yaml:"user"`
    Password  string `yaml:"password"`
    Message   WebhookMessage `yaml:"message"`
}

type WebhookMessage struct {
    Format    string `yaml:"format"`
    Template  string `yaml:"template"`
}

func ConfigLoad(configFileName string) (*Config, error) {
	f, err := os.Open(configFileName)
    if err != nil {
        return nil, fmt.Errorf("open config: %w\n\rTry running 'watchdog -make-config'", err)
    }
	defer f.Close()

	var c Config
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("decode yaml: %w", err)
	}

	if err := c.validate(); err != nil {
		return nil, err
	}

	return &c, nil
}

// validate - checks the required fields for the included sections
func (c *Config) validate() error {
	if c.Notify.Email.Enable {
		if c.Notify.Email.Server == "" {
			return fmt.Errorf("notify.email.server is required when email is enabled")
		}
		if c.Notify.Email.MailFrom == "" || c.Notify.Email.MailTo == "" {
			return fmt.Errorf("notify.email.mail_from and mail_to are required")
		}
	}
	if c.Notify.Matrix.Enable && c.Notify.Matrix.Server == "" {
		return fmt.Errorf("notify.matrix.server is required when matrix is enabled")
	}
	if c.Notify.Webhook.Enable && c.Notify.Webhook.URL == "" {
		return fmt.Errorf("notify.webhook.url is required when webhook is enabled")
	}
	if c.Checks.Web.Enable && len(c.Checks.Web.URLs) == 0 {
		return fmt.Errorf("checks.web.urls is empty but web check is enabled")
	}
	if c.Checks.Files.Enable && len(c.Checks.Files.Paths) == 0 {
		return fmt.Errorf("checks.files.paths is empty but files check is enabled")
	}
	return nil
}

// ConfigMake - Creates a configuration file from a template (ConfigTemplate)
func ConfigMake(cfgTmpl, cfgFileName string) error {
	if _, err := os.Stat(cfgFileName); err == nil {
		return fmt.Errorf("config file already exists: %s", cfgFileName)
	}

	file, err := os.Create(cfgFileName)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(cfgTmpl); err != nil {
		return fmt.Errorf("write template: %w", err)
	}

	fmt.Printf("Config file '%s' has been created.\n", cfgFileName)
	return nil
}
