package main

import (
    "fmt"

    "gopkg.in/ini.v1"
)

// Config — корень конфигурации
type Config struct {
	Checks  Checks  `ini:"Checks"`
	Notify  Notify  `ini:"Notify"`
}

// Checks — секция [Checks]
type Checks struct {
	Process bool `ini:"process"`
	Users   bool `ini:"users"`
	Network bool `ini:"network"`
	Ports   bool `ini:"ports"`
	Web     bool `ini:"web"`
	Cron    bool `ini:"cron"`

	// Вложенная секция [Checks.web] — имя поля должно совпадать с "web"
	WebCheck WebCheckConfig `ini:"web"`
}

// WebConfig — секция [Checks.web]
type WebCheckConfig struct {
	// Массив URL из повторяющихся ключей "url"
	// ini.v1 не мапит дубликаты в []string автоматически,
	// поэтому поле читаем вручную через ValueWithShadows()
	URLs []string `ini:"-"`
}

// Notify — секция [Notify]
type Notify struct {
	Email   bool `ini:"email"`
	Matrix  bool `ini:"matrix"`
	Webhook bool `ini:"webhook"`

	EmailCfg  EmailConfig  `ini:"email"`
	MatrixCfg MatrixConfig `ini:"matrix"`
    WebhookCfg WebhookConfig `ini:"webhook"`
}

// EmailConfig — секция [Notify.email]
type EmailConfig struct {
	Server   string `ini:"server"`
	Port     int    `ini:"port"`
	User     string `ini:"user"`
	Password string `ini:"password"`
}

// MatrixConfig — секция [Notify.matrix]
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
	// if err := cfg.MapTo(&c); err != nil {
		// return nil, fmt.Errorf("map ini: %w", err)
	// }

    // 1. Маппим корневые секции
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

	return &c, nil
}

func ConfigShow(cfg *Config) {
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

