// *******************************************************
// Watchdog - the utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
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

// ConfigTemplate — шаблон конфига, который создаётся при первом запуске.
var ConfigTemplate = `daemon:
  check_timeout: 60
  log_file: watchdog.log
  work_dir: ./

checks:
  cron:
    enable: false
  processes:
    enable: false
    exclude:
      - kworker
  users:
    enable: false
  network:
    enable: false
  ports:
    enable: false
  web:
    enable: false
    urls:
      - https://server.com
      - https://git.server.com
  files:
    enable: true
    paths:
      - path: /etc
        watch: [create, modify, delete]
      - path: /dev
        watch: [create, modify, delete]
        exclude:
          - /dev/pts
          - /dev/tty
    services:
        enable: true
        units:
          - sshd.service
          - nginx.service
          - postgresql.service
notify:
  email:
    enable: false
    server: smtp.server.com
    port: 465
    user: user
    password: "12345678"
    mail_to: to@domain.com
    mail_from: from@domain.com
    mail_subject: "Alert message from %HOSTNAME%"

  matrix:
    enable: false
    server: matrix.server.com
    user: user
    password: "12345678"
    chat_id: 1qu2ueu4383jr4o83

  webhook:
    enable: false
    url: https://web.server.com/hook
    user: user
    password: "12345678"
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

type BaseCheck struct {
	Enable bool `yaml:"enable"`
}

type ChecksConfig struct {
    Cron      CronCheck    `yaml:"cron"`
    Processes ProcessCheck `yaml:"processes"`
    Users     UsersCheck   `yaml:"users"`
    Network   NetworkCheck `yaml:"network"`
    Ports     PortsCheck   `yaml:"ports"`
    Web       WebCheck     `yaml:"web"`
    Files     FilesCheck   `yaml:"files"`
    Services  ServicesCheck `yaml:"services"`
}

type CronCheck struct {
	Enable bool `yaml:"enable"`
}

type ProcessCheck struct {
	Enable bool `yaml:"enable"`
    Exclude []string `yaml:"exclude"`
}

type UsersCheck struct {
	Enable bool `yaml:"enable"`
}

type NetworkCheck struct {
	Enable bool `yaml:"enable"`
	// BaseCheck `yaml:",inline"`
}

type PortsCheck struct {
	Enable bool `yaml:"enable"`
	// BaseCheck `yaml:",inline"`
}

type WebCheck struct {
	Enable bool `yaml:"enable"`
	// BaseCheck `yaml:",inline"`
	URLs      []string `yaml:"urls"`
}

type FilesCheck struct {
	Enable bool `yaml:"enable"`
	// BaseCheck `yaml:",inline"`
	Paths     []PathWatch `yaml:"paths"`
}

type ServicesCheck struct {
    Enable bool     `yaml:"enable"`
    Units  []string `yaml:"units"`
}

type PathWatch struct {
	Path    string   `yaml:"path"`
	Watch   []string `yaml:"watch"`   // "create", "modify", "delete"
	Exclude []string `yaml:"exclude"` // подпути, которые не отслеживаются
}

type NotifyConfig struct {
	Email   EmailNotify   `yaml:"email"`
	Matrix  MatrixNotify  `yaml:"matrix"`
	Webhook WebhookNotify `yaml:"webhook"`
}

type EmailNotify struct {
	BaseCheck   `yaml:",inline"`
	Server      string `yaml:"server"`
	Port        int    `yaml:"port"`
	User        string `yaml:"user"`
	Password    string `yaml:"password"`
	MailTo      string `yaml:"mail_to"`
	MailFrom    string `yaml:"mail_from"`
	MailSubject string `yaml:"mail_subject"`
}

type MatrixNotify struct {
	BaseCheck `yaml:",inline"`
	Server    string `yaml:"server"`
	User      string `yaml:"user"`
	Password  string `yaml:"password"`
	ChatID    string `yaml:"chat_id"`
}

type WebhookNotify struct {
	BaseCheck `yaml:",inline"`
	URL       string `yaml:"url"`
	User      string `yaml:"user"`
	Password  string `yaml:"password"`
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

// validate проверяет обязательные поля для включённых секций.
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
