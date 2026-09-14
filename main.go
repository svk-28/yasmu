package main

import (
    "fmt"
    "log"
    "watchdog/checklibs"
    "watchdog/notifylibs"
)

func main() {
    // Loading config
	cfg, err := ConfigLoad("config.ini")
	if err != nil {
		log.Fatal(err)
	}

    ConfigShow(cfg)
    

    // Runing checks
    if cfg.Checks.Users {
        checklibs.UsersCheck()
        checklibs.UsersPermissionCheck()
        checklibs.UsersSudoersCheck()
    }
    if cfg.Checks.Process {
        checklibs.ProcessCheck()
    }
    if cfg.Checks.Network {
        checklibs.NetworkCheck()
    }
    if cfg.Checks.Ports {
        checklibs.PortsCheck()
    }
    if cfg.Checks.Cron {
        checklibs.CronCheck()
        checklibs.CrontabCheck()
    }
    if cfg.Checks.Web {
        for _, v := range cfg.Checks.WebCheck.URLs {
            fmt.Println(v)
            checklibs.WebCheck(v)
        }
    }

    // Sending notification
    if cfg.Notify.Email {
        notifylibs.EmailNotify()
    }
    if cfg.Notify.Matrix {
        notifylibs.MatrixNotify()
    }
    if cfg.Notify.Webhook {
        notifylibs.WebHookNotify()
    }
}
