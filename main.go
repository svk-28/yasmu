package main

import (
    "fmt"
    "log"
    "time"
    "flag"
    "os"
    "os/signal"
    "context"
    "syscall"
    "strings"
    // "net"

    "watchdog/checklibs"
    "watchdog/notifylibs"
    "watchdog/mainlibs"

    "github.com/sevlyar/go-daemon"
)

const (
    Version = "0.0.1"
)

var (
    ConfigFile string
    RunAsDaemon bool
    MakeConfig  bool
    RunOnce     bool
)

var previousFileList checklibs.FileMap
var previousUrlList checklibs.UrlMap

// Runing checks
func runChecks(cfg *Config) {
    var resultOfFilesCheck string
    var summaryReport strings.Builder
    // var resultOfFilesCheck, resultOfUsersCheck, resultOfProcessCheck string
    // var resultOfPortsCheck, resultOfNetworkCheck, resultOfCronCheck, resultOfWebCheck string

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
            log.Println(v)
            status, response := checklibs.WebCheck(v)
            if status {
                log.Printf("URL %s check status is %t\n", v, status)
                // log.Println(response)
            } else {
                log.Printf("URL %s check status error\n", v)
                log.Println(response)
                // summaryReport.WriteString(fmt.Fprintf("===\nURL %s check status error\n", v"))
                // summaryReport.WriteString(fmt.Fprintf("%s\n\n", response))
                fmt.Fprintf(&summaryReport, "===\nURL %s check status error\n", v)
                fmt.Fprintf(&summaryReport, "%s\n", response)
            }
        }
    }

    if cfg.Checks.Files {
        previousFileList, resultOfFilesCheck = checklibs.FilesCheck(cfg.Checks.FilesPath.Path, previousFileList)
        if resultOfFilesCheck != "" {
            log.Println("Changes to the file system have been recorded:")
            log.Println(resultOfFilesCheck)
            summaryReport.WriteString(resultOfFilesCheck)
        } else {
            log.Println("No files changes have been recorded.")
        }
    }
    
    if len(summaryReport.String()) != 0 {
        log.Println("Result of all checks:", summaryReport.String())
        SendNotifycation(cfg, summaryReport.String())
    }
}

func RunService(cfg *Config) {
    for {
        runChecks(cfg)
        time.Sleep(time.Duration(cfg.Daemon.CheckTimeout) * time.Second)
    }
}


func DaemonRun(cfg *Config) {
    cntxt := &daemon.Context{
        PidFileName: cfg.Daemon.PidFile,
        PidFilePerm: 0600,
        LogFileName: cfg.Daemon.LogFile,
        LogFilePerm: 0600,
        WorkDir:     cfg.Daemon.WorkDir,
        Umask:       027,
    }

    d, err := cntxt.Reborn()
    if err != nil {
        log.Fatal("Failed to create a demon: ", err)
    }
    if d != nil {
        return
    }
    defer cntxt.Release()

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    log.Println("Daemon started")

    timeout := time.Duration(cfg.Daemon.CheckTimeout) * time.Second
    if timeout <= 0 {
        timeout = 30 * time.Second
    }

    for {
        runChecks(cfg)

        select {
        case <-ctx.Done():
            log.Println("Daemon stopped")
            return
        case <-time.After(timeout):
        }
    }
}

func SendNotifycation(cfg *Config, msg string) {
    // Sending notification

    hostname, err := mainlibs.GetFQDN()
    if err != nil {
	    log.Fatal(err)
    }
    log.Println("Running host FQDN:", hostname)


    if cfg.Notify.Email {
        var (
            message     notifylibs.MailMessage
            smtpSession notifylibs.SmtpSession
        )
        
        log.Println("Running notifylibs/EmailNotify")


        message.MailFrom = cfg.Notify.EmailCfg.MailFrom
        message.MailTo = cfg.Notify.EmailCfg.MailTo
        // %HOSTNAME% template in Subject will replace
        message.MailSubject = strings.ReplaceAll(cfg.Notify.EmailCfg.MailSubject, "%HOSTNAME%", hostname)
        message.MailMessage = msg
    
        smtpSession.SmtpServer = cfg.Notify.EmailCfg.Server
        smtpSession.SmtpPort = cfg.Notify.EmailCfg.Port
        smtpSession.SmtpUser = cfg.Notify.EmailCfg.User
        smtpSession.SmtpPassword = cfg.Notify.EmailCfg.Password
    
        log.Println("Sending email message:", message.MailFrom, message.MailTo)
        msgSendResult := notifylibs.EmailNotify(&smtpSession, &message)

        if msgSendResult {
            log.Println("Message send successfuly")
        } else {
            log.Println("Message send error")
        }
       
    }
    if cfg.Notify.Matrix {
        notifylibs.MatrixNotify()
    }
    if cfg.Notify.Webhook {
        notifylibs.WebHookNotify()
    }
}

func main() {
    
    flag.BoolVar(&RunAsDaemon, "daemon", false, "Run the programm as ad daemon")
    flag.BoolVar(&RunOnce, "run-once", false, "Run the programm one time")
    flag.StringVar(&ConfigFile, "config", "config.ini", "Config file with full path")
    flag.BoolVar(&MakeConfig, "make-config", false, "Generate config file")
    flag.BoolFunc("version", "Version", func(s string) error {
        log.Println("Version:", Version)
        os.Exit(0)
        return nil
    })
    flag.Parse()

    // Generate config
    if MakeConfig {
        ConfigMake(ConfigTemplate, ConfigFile)
        os.Exit(0)
    }
    // Loading config
	cfg, err := ConfigLoad(ConfigFile)
	if err != nil {
		log.Fatal(err)
	}

    // ConfigShow(cfg)

    if RunAsDaemon {
        DaemonRun(cfg)
    } else if RunOnce {
        runChecks(cfg)
    } else {
        RunService(cfg)
    }
}
