// *******************************************************
// Watchdog - the utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
// License: GPL v3
// *******************************************************
// Configuration management module.
// Creating/reading a configuration file.
// Displaying settings on the screen.package main
// *******************************************************

package main

import (
    "fmt"
    "os"
    "gopkg.in/ini.v1"
)

var ConfigTemplate = `
[Daemon]
checktimeout = 60
logfile = watchdog.log
workdir = ./

[Checks]
process = yes
users = yes
network = yes
ports = yes
web = no
cron = yes
files = no

[Notify]
email = no
matrix = no
webhook = no

[Notify.email]
server = smtp.server.com 
port = 465
user = user
password = 12345678
mailto = to@domain.com
mailfrom = from@domain.com
mailsubject = Alert message from %HOSTNAME$

[Notify.matrix]
server = matrix.server.com
user = user
password = 12345678
chatid = 1qu2ueu4383jr4o83

[Notify.webhook]
url = https://web.server.com/hook
user = user
password = 12345678


[Checks.web]
url = https://server.com
url = https://git.server.com
urs = ...

[Check.files]
path = /etc
path = ...
`
type Daemon struct {
    	CheckTimeout int    `ini:"checktimeout"`
    	LogFile      string `ini:"logfile"`
    	WorkDir      string `ini:"workdir"`
     PidFile      string `ini:"pidfile"`
}

type Config struct {
    Daemon  Daemon  `ini:"Daemon"`
	Checks  Checks  `ini:"Checks"`
	Notify  Notify  `ini:"Notify"`
}

type Checks struct {
	Process bool `ini:"process"`
	Users   bool `ini:"users"`
	Network bool `ini:"network"`
	Ports   bool `ini:"ports"`
	Web     bool `ini:"web"`
	Cron    bool `ini:"cron"`
	Files   bool `ini:"files"`

	WebCheck WebCheckConfig `ini:"web"`
    FilesPath FilesConfig `ini:"files"`
}

type WebCheckConfig struct {
	URLs []string `ini:"-"`
}

type Notify struct {
	Email   bool `ini:"email"`
	Matrix  bool `ini:"matrix"`
	Webhook bool `ini:"webhook"`

	EmailCfg  EmailConfig  `ini:"email"`
	MatrixCfg MatrixConfig `ini:"matrix"`
    WebhookCfg WebhookConfig `ini:"webhook"`
}

type EmailConfig struct {
	Server   string `ini:"server"`
	Port     string `ini:"port"`
	User     string `ini:"user"`
	Password string `ini:"password"`
    MailFrom string `ini:"mailfrom"`
    MailTo   string `ini:"mailto"`
    MailSubject string `ini:"mailsubject"`
}

type MatrixConfig struct {
	Server   string `ini:"server"`
	User     string `ini:"user"`
	Password string `ini:"password"`
	ChatID   string `ini:"chatID"`
}

type WebhookConfig struct {
	URL   string `ini:"url"`
	User     string `ini:"user"`
	Password string `ini:"password"`
}

type FilesConfig struct {
    Path []string `ini:"path"`
}

func ConfigLoad(configFileName string) (*Config, error) {
	cfg, err := ini.LoadSources(ini.LoadOptions{
		AllowShadows:    true,
		AllowBooleanKeys: true,
		Insensitive:     true,
	}, configFileName)
	if err != nil {
		return nil, fmt.Errorf("load ini: %w", err)
	}

	var c Config

    // 1. Маппим корневые секции
    if err := cfg.Section("Daemon").MapTo(&c.Daemon); err != nil {
        return nil, fmt.Errorf("map Daemon: %w", err)
    }
    if err := cfg.Section("Checks").MapTo(&c.Checks); err != nil {
        return nil, fmt.Errorf("map Checks: %w", err)
    }
    if err := cfg.Section("Notify").MapTo(&c.Notify); err != nil {
        return nil, fmt.Errorf("map Notify: %w", err)
    }

    // 2. Маппим вложенные секции по полному пути
    if err := cfg.Section("Notify.email").MapTo(&c.Notify.EmailCfg); err != nil {
        return nil, fmt.Errorf("map Notify.email: %w", err)
    }
    if err := cfg.Section("Notify.matrix").MapTo(&c.Notify.MatrixCfg); err != nil {
        return nil, fmt.Errorf("map Notify.matrix: %w", err)
    }
    if err := cfg.Section("Notify.webhook").MapTo(&c.Notify.WebhookCfg); err != nil {
        return nil, fmt.Errorf("map Notify.webhook: %w", err)
    }

	// Отдельно вычитываем повторяющиеся url из [Checks.web]
	urls := cfg.Section("Checks.web").Key("url").ValueWithShadows()
	c.Checks.WebCheck.URLs = urls

	// Отдельно вычитываем повторяющиеся path из [Checks.files]
	path := cfg.Section("Checks.files").Key("path").ValueWithShadows()
	c.Checks.FilesPath.Path = path

	return &c, nil
}

func ConfigShow(cfg *Config) {
   	fmt.Printf("Пауза между проверками при запуске как службы (демон) в сек.: %v\n", cfg.Daemon.CheckTimeout)
   	fmt.Printf("Проверять процессы (processes): %v\n", cfg.Checks.Process)
	fmt.Printf("Проверять пользователей (users): %v\n", cfg.Checks.Users)
	fmt.Printf("Проверять соединения (network): %v\n", cfg.Checks.Network)
	fmt.Printf("Проверять открытые порты (ports): %v\n", cfg.Checks.Ports)
	fmt.Printf("Проверять cron (cron): %v\n", cfg.Checks.Cron)
	fmt.Printf("Проверять web: %v\n", cfg.Checks.Web)
	fmt.Printf("\tURL для проверки: %v\n", cfg.Checks.WebCheck.URLs)
    
    fmt.Printf("\nОповещение:\n")
    fmt.Printf("\tE-mail: %v\n", cfg.Notify.Email)
	fmt.Printf("\t\tSMTP: %s:%d, user=%s\n",
		cfg.Notify.EmailCfg.Server,
		cfg.Notify.EmailCfg.Port,
		cfg.Notify.EmailCfg.User,
	)
    fmt.Printf("\tMatrix: %v\n", cfg.Notify.Matrix)
	fmt.Printf("\t\tServer: %s, user=%s, chat id:%s \n",
		cfg.Notify.MatrixCfg.Server,
		cfg.Notify.MatrixCfg.User,
		cfg.Notify.MatrixCfg.ChatID,
	)
    fmt.Printf("\tWeb hook: %v\n", cfg.Notify.Webhook)
	fmt.Printf("\t\tURL: %s, user=%s, password=%s\n",
		cfg.Notify.WebhookCfg.URL,
		cfg.Notify.WebhookCfg.User,
		cfg.Notify.WebhookCfg.Password,
	)
}

func ConfigMake(cfgTmpl string, cfgFileName string) {
    file, err := os.Create(cfgFileName)
     
    if err != nil{
        fmt.Println("Unable to create file:", err) 
        os.Exit(1) 
    }
    defer file.Close() 
    file.WriteString(cfgTmpl)
     
    fmt.Printf("Config file '%s' has been created.\n", cfgFileName)
}
