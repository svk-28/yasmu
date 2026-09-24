// *******************************************************
// Watchdog - the utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
// License: GPL v3
// *******************************************************

package main

import (
    "context"
    "flag"
    "fmt"
    "log"
    "os"
    // "io"
    "os/signal"
    "strings"
    "syscall"
    "time"

    "watchdog/checklibs"
    "watchdog/mainlibs"
    "watchdog/notifylibs"

    "github.com/sevlyar/go-daemon"
)

const (
	Version = "0.0.2"
)

var (
	ConfigFile  string
	RunAsDaemon bool
	MakeConfig  bool
	RunOnce     bool
)

var (
    previousFileList    checklibs.FileList
    previousUrlList     checklibs.UrlList
    previousUsersList   checklibs.UserList
    previousServiceList checklibs.ServiceList
    previousProcessList checklibs.ProcessList
)

func runChecks(cfg *Config) {
    var (
        resultOfFilesCheck     string
        resultOfUsersCheck     string
        resultOfServicesCheck  string
        resultOfProcessesCheck string
        summaryReport          strings.Builder
    )
    // *********** Users Checks ***************
    if cfg.Checks.Users.Enable {
        previousUsersList, resultOfUsersCheck = checklibs.UsersCheck(previousUsersList)
        if resultOfUsersCheck != "" {
            log.Println("Changes in users have been recorded:")
            log.Println(resultOfUsersCheck)
            fmt.Fprintf(&summaryReport, "\n========= Changes to the users have been recorded =========\n")
            summaryReport.WriteString(resultOfUsersCheck)
        } else {
            log.Println("No user changes have been recorded.")
        }
    }

    // *********** Processes Checks ***************
    if cfg.Checks.Processes.Enable {
        previousProcessList, resultOfProcessesCheck = checklibs.ProcessesCheck(
            cfg.Checks.Processes.Exclude,
            previousProcessList,
        )
        if resultOfProcessesCheck != "" {
            log.Println("Changes in processes have been recorded:")
            log.Println(resultOfProcessesCheck)
            fmt.Fprintf(&summaryReport, "\n========= Changes in processes have been recorded =========\n")
            summaryReport.WriteString(resultOfProcessesCheck)
        } else {
            log.Println("No process changes have been recorded.")
        }
    }

    // *********** Network Connections Checks ***************
	if cfg.Checks.Network.Enable {
		checklibs.NetworkCheck()
	}

    // *********** Network Ports Checks ***************
	if cfg.Checks.Ports.Enable {
		checklibs.PortsCheck()
	}

    // *********** Web Links Checks ***************
	if cfg.Checks.Web.Enable {
		for _, v := range cfg.Checks.Web.URLs {
			log.Println(v)
			status, response := checklibs.WebCheck(v)
			if status {
				log.Printf("URL %s check status is %t\n", v, status)
			} else {
				log.Printf("URL %s check status error\n", v)
				log.Println(response)
				fmt.Fprintf(&summaryReport, "===\nURL %s check status error\n", v)
				fmt.Fprintf(&summaryReport, "%s\n", response)
			}
		}
	}

    // *********** Files And Directories Checks ***************
    if cfg.Checks.Files.Enable {
	    paths := make([]checklibs.PathSpec, 0, len(cfg.Checks.Files.Paths))
	    for _, pw := range cfg.Checks.Files.Paths {
		    paths = append(paths, checklibs.PathSpec{
			    Path:    pw.Path,
			    Exclude: pw.Exclude,
        			    Watch:   pw.Watch,
		    })
	    }
    
        previousFileList, resultOfFilesCheck = checklibs.FilesCheck(paths, previousFileList)

	    if resultOfFilesCheck != "" {
		    log.Println("Changes to the file system have been recorded:")
		    log.Println(resultOfFilesCheck)
            fmt.Fprintf(&summaryReport, "\n========= Changes to the file system have been recorded =========\n")
		    summaryReport.WriteString(resultOfFilesCheck)
	    } else {
		    log.Println("No files changes have been recorded.")
	    }
    }

    // *********** System Services Checks ***************
    if cfg.Checks.Services.Enable {
        previousServiceList, resultOfServicesCheck = checklibs.ServicesCheck(cfg.Checks.Services.Units, previousServiceList)
        if resultOfServicesCheck != "" {
            log.Println("Changes in services have been recorded:")
            log.Println(resultOfServicesCheck)
            fmt.Fprintf(&summaryReport, "\n========= Changes to the services have been recorded =========\n")
            summaryReport.WriteString(resultOfServicesCheck)
        } else {
            log.Println("No service changes have been recorded.")
        }
    }

    // *********** Sending Summary Report ***************
	if len(summaryReport.String()) != 0 {
		log.Println("Result of all checks:", summaryReport.String())
		SendNotifycation(cfg, summaryReport.String())
	}
}

func RunService(ctx context.Context, cfg *Config) {
	timeout := time.Duration(cfg.Daemon.CheckTimeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	for {
		runChecks(cfg)

		select {
		case <-ctx.Done():
			return
		case <-time.After(timeout):
		}
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
	RunService(ctx, cfg)
	log.Println("Daemon stopped")
}

func SendNotifycation(cfg *Config, msg string) {
	hostname, err := mainlibs.GetFQDN()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Running host FQDN:", hostname)

	if cfg.Notify.Email.Enable {
		var (
			message     notifylibs.MailMessage
			smtpSession notifylibs.SmtpSession
		)

		log.Println("Running notifylibs/EmailNotify")

		message.MailFrom = cfg.Notify.Email.MailFrom
		message.MailTo = cfg.Notify.Email.MailTo
		// %HOSTNAME% template in Subject will replace
		message.MailSubject = strings.ReplaceAll(cfg.Notify.Email.MailSubject, "%HOSTNAME%", hostname)
		message.MailMessage = msg

		smtpSession.SmtpServer = cfg.Notify.Email.Server
		smtpSession.SmtpPort = fmt.Sprint(cfg.Notify.Email.Port)
		smtpSession.SmtpUser = cfg.Notify.Email.User
		smtpSession.SmtpPassword = cfg.Notify.Email.Password

		log.Println("Sending email message:", message.MailFrom, message.MailTo)
		msgSendResult := notifylibs.EmailNotify(&smtpSession, &message)

		if msgSendResult {
			log.Println("E-Mail message send successfuly")
		} else {
			log.Println("E-Mail message send error")
		}
	}
    if cfg.Notify.Matrix.Enable {
        var matrixSession notifylibs.MatrixClient
        matrixSession.HomeServer = cfg.Notify.Matrix.Server
        matrixSession.UserName = cfg.Notify.Matrix.UserName
        matrixSession.UserPassword = cfg.Notify.Matrix.Password
        matrixSession.AccessToken = cfg.Notify.Matrix.AccessToken
        // matrixSession.RoomID = cfg.Notify.Matrix.RoomID + ':' + cfg.Notify.Matrix.Server
        matrixSession.RoomID = fmt.Sprintf("%s:%s", cfg.Notify.Matrix.RoomID, cfg.Notify.Matrix.Server)
        msgSendResult, msgEventID := notifylibs.MatrixNotify(matrixSession, msg)
       	if msgSendResult {
			log.Println("Matrix message send successfuly with event id ", msgEventID)
		} else {
			log.Println("Matrix message send error")
		}
    }
    if cfg.Notify.Webhook.Enable {
        notifylibs.WebHookNotify()
    }
}

func main() {
	flag.BoolVar(&RunAsDaemon, "daemon", false, "Run the programm as a daemon")
	flag.BoolVar(&RunOnce, "run-once", false, "Run the programm one time")
	flag.StringVar(&ConfigFile, "config", "config.yaml", "Config file with full path")
	flag.BoolVar(&MakeConfig, "make-config", false, "Generate config file")
	flag.BoolFunc("version", "Version", func(s string) error {
		log.Println("Version:", Version)
		os.Exit(0)
		return nil
	})
	flag.Parse()

    // Disable log output
    // log.SetOutput(io.Discard)
    // log.SetFlags(0)
    // log.SetPrefix("")
    
	if MakeConfig {
		if err := ConfigMake(ConfigTemplate, ConfigFile); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	cfg, err := ConfigLoad(ConfigFile)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if RunAsDaemon {
		DaemonRun(cfg)
	} else if RunOnce {
		runChecks(cfg)
	} else {
		RunService(ctx, cfg)
	}
}
