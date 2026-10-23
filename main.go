// *******************************************************
// YASMU - Yet Another System Monitoring Utility
// The utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
// Home page: https://nuk-svk.ru
// License: GPL v3
// *******************************************************

package main

import (
    "context"
    "flag"
    "fmt"
    "log"
    "os"
    "errors"
    "os/signal"
    "strings"
    "syscall"
    "time"
    "strconv"
    "path/filepath"
    "encoding/gob"

    "yasmu/checklibs"
    "yasmu/mainlibs"
    "yasmu/notifylibs"
    "yasmu/config"
    "yasmu/logslibs"

    "github.com/sevlyar/go-daemon"
     opensearch "github.com/opensearch-project/opensearch-go/v2"
)

const (
	Version = "1.0.4"
)

var (
    ConfigFile  string
    RunAsDaemon bool
    MakeConfig  bool
    RunOnce     bool
    StopDaemon  bool
    HostName    string
    previousFileList    checklibs.FileList
    previousUrlList     checklibs.UrlList
    previousUsersList   checklibs.UserList
    previousServiceList checklibs.ServiceList
    previousProcessList checklibs.ProcessList
    previousPortList    checklibs.PortList
    opensearchClient    *opensearch.Client
)

func runChecks(cfg *config.Config) {
    var (
        resultOfFilesCheck     string
        resultOfUsersCheck     string
        resultOfServicesCheck  string
        resultOfProcessesCheck string
        resultOfWebCheck       string
        resultOfPortsCheck     string
        summaryReport          strings.Builder
        summaryJSONReport      []map[string]string
    )
    indexName := logslibs.ConvertIndexName(cfg.Log.OpenSearch.IndexName)

    log.Println("Host name is:", HostName)
    // *********** Users Checks ***************
    if cfg.Checks.Users.Enable {
        previousUsersList, resultOfUsersCheck = checklibs.UsersCheck(previousUsersList)
        if resultOfUsersCheck != "" {
            log.Println("Changes in users have been recorded:")
            log.Println(resultOfUsersCheck)
            fmt.Fprintf(&summaryReport, "\n========= Changes to the users have been recorded =========\n")
            summaryReport.WriteString(resultOfUsersCheck)
            doc := logslibs.MakeJsonDocument(HostName, "users", "changed", resultOfUsersCheck)
            summaryJSONReport = append(summaryJSONReport, doc)
            if cfg.Log.OpenSearch.Enable && opensearchClient != nil && !cfg.Log.OpenSearch.Bulk {
                // doc := logslibs.MakeJsonDocument(
                    // HostName,            // FQDN хоста
                    // "users",             // checkName
                    // "changed",           // checkStatus
                    // resultOfUsersCheck,  // текст отчёта
                // )
                if !logslibs.OsAddRecord(opensearchClient, indexName, doc) {
                    log.Println("warn: failed to send 'users' check to OpenSearch")
                }
            }
        } else {
            log.Println("No user changes have been recorded.")
        }
    }

    // *********** Processes Checks ***************
    if cfg.Checks.Processes.Enable {
        previousProcessList, resultOfProcessesCheck = checklibs.ProcessesCheck(
            cfg.Checks.Processes,
            previousProcessList,
        )
        if resultOfProcessesCheck != "" {
            log.Println("Changes in processes have been recorded:")
            log.Println(resultOfProcessesCheck)
            fmt.Fprintf(&summaryReport, "\n========= Changes in processes have been recorded =========\n")
            summaryReport.WriteString(resultOfProcessesCheck)
            doc := logslibs.MakeJsonDocument(HostName, "processes", "changed", resultOfProcessesCheck)
            summaryJSONReport = append(summaryJSONReport, doc)

            if cfg.Log.OpenSearch.Enable && opensearchClient != nil && !cfg.Log.OpenSearch.Bulk {
                // doc := logslibs.MakeJsonDocument(
                    // HostName,
                    // "processes",
                    // "changed",
                    // resultOfProcessesCheck,
                // )
                if !logslibs.OsAddRecord(opensearchClient, indexName, doc) {
                    log.Println("warn: failed to send 'processes' check to OpenSearch")
                }
            }
        } else {
            log.Println("No process changes have been recorded.")
        }
    }

    // *********** Network Connections Checks ***************
    if cfg.Checks.Connections.Enable {
        rules := make([]checklibs.ConnRule, 0, len(cfg.Checks.Connections.Rules))
        for _, r := range cfg.Checks.Connections.Rules {
            rules = append(rules, checklibs.ConnRule{
                Name:          r.Name,
                Proto:         r.Proto,
                Port:          r.Port,
                Processes:     r.Processes,
                State:         r.State,
                AllowedRemote: r.AllowedRemote,
                AlertOnMatch:  r.AlertOnMatch,
            })
        }
    
        if resultOfConnCheck := checklibs.ConnectionsCheck(rules); resultOfConnCheck != "" {
            log.Println("Connection rule violations have been recorded:")
            log.Println(resultOfConnCheck)
            fmt.Fprintf(&summaryReport, "\n========= Connection rule violations have been recorded =========\n")
            summaryReport.WriteString(resultOfConnCheck)
            doc := logslibs.MakeJsonDocument(HostName, "connections", "changed", resultOfConnCheck)
            summaryJSONReport = append(summaryJSONReport, doc)
            if cfg.Log.OpenSearch.Enable && opensearchClient != nil && !cfg.Log.OpenSearch.Bulk {
                // doc := logslibs.MakeJsonDocument(
                    // HostName,
                    // "connections",
                    // "changed",
                    // resultOfConnCheck,
                // )
                if !logslibs.OsAddRecord(opensearchClient, indexName, doc) {
                    log.Println("warn: failed to send 'connections' check to OpenSearch")
                }
            }
        } else {
            log.Println("No connection rule violations.")
        }
    }

    // *********** Network Ports Checks ***************
    if cfg.Checks.Ports.Enable {
        opts := checklibs.PortsOptions{
            ExcludePorts:     cfg.Checks.Ports.ExcludePorts,
            ExcludeProcesses: cfg.Checks.Ports.ExcludeProcesses,
            IgnorePIDChanges: cfg.Checks.Ports.IgnorePIDChanges,
        }
    
        previousPortList, resultOfPortsCheck = checklibs.PortsCheck(opts, previousPortList)
        if resultOfPortsCheck != "" {
            log.Println("Changes in ports have been recorded:")
            fmt.Fprintf(&summaryReport, "\n========= Changes in ports checks have been recorded =========\n")
            log.Println(resultOfPortsCheck)
            summaryReport.WriteString(resultOfPortsCheck)
            doc := logslibs.MakeJsonDocument(HostName, "ports", "changed", resultOfPortsCheck)
            summaryJSONReport = append(summaryJSONReport, doc)
            if cfg.Log.OpenSearch.Enable && opensearchClient != nil && !cfg.Log.OpenSearch.Bulk {
                // doc := logslibs.MakeJsonDocument(
                    // HostName,
                    // "ports",
                    // "changed",
                    // resultOfPortsCheck,
                // )
                if !logslibs.OsAddRecord(opensearchClient, indexName, doc) {
                    log.Println("warn: failed to send 'ports' check to OpenSearch")
                }
            }
        } else {
            log.Println("No port changes have been recorded.")
        }
    }

    // *********** Web Links Checks ***************
    if cfg.Checks.Web.Enable {
        previousUrlList, resultOfWebCheck = checklibs.WebCheck(cfg.Checks.Web.URLs, previousUrlList)
        if resultOfWebCheck != "" {
            fmt.Fprintf(&summaryReport, "\n========= Changes in web checks have been recorded =========\n")
            log.Println(resultOfWebCheck)
            summaryReport.WriteString(resultOfWebCheck)
            doc := logslibs.MakeJsonDocument(HostName, "web", "changed", resultOfWebCheck)
            summaryJSONReport = append(summaryJSONReport, doc)
            if cfg.Log.OpenSearch.Enable && opensearchClient != nil && !cfg.Log.OpenSearch.Bulk {
                // doc := logslibs.MakeJsonDocument(
                    // HostName,
                    // "web",
                    // "changed",
                    // resultOfWebCheck,
                // )
                if !logslibs.OsAddRecord(opensearchClient, indexName, doc) {
                    log.Println("warn: failed to send 'web' check to OpenSearch")
                }
            }
        } else {
            log.Println("No web changes have been recorded.")
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
                Hash:    pw.Hash,
		    })
	    }
    
        previousFileList, resultOfFilesCheck = checklibs.FilesCheck(paths, previousFileList)

	    if resultOfFilesCheck != "" {
		    log.Println("Changes to the file system have been recorded:")
		    log.Println(resultOfFilesCheck)
            fmt.Fprintf(&summaryReport, "\n========= Changes to the file system have been recorded =========\n")
		    summaryReport.WriteString(resultOfFilesCheck)
            doc := logslibs.MakeJsonDocument( HostName, "files", "changed", resultOfFilesCheck)
            summaryJSONReport = append(summaryJSONReport, doc)
            if cfg.Log.OpenSearch.Enable && opensearchClient != nil && !cfg.Log.OpenSearch.Bulk {
                // doc := logslibs.MakeJsonDocument(
                    // HostName,
                    // "files",
                    // "changed",
                    // resultOfFilesCheck,
                // )
                if !logslibs.OsAddRecord(opensearchClient, indexName, doc) {
                    log.Println("warn: failed to send 'files' check to OpenSearch")
                }
            }
	    } else {
		    log.Println("No files changes have been recorded.")
	    }
    }

    // *********** System Services Checks ***************
    if cfg.Checks.Services.Enable {
        previousServiceList, resultOfServicesCheck = checklibs.ServicesCheck(
            cfg.Checks.Services.Exclude,
            cfg.Checks.Services.Units,
            previousServiceList,
        )
        if resultOfServicesCheck != "" {
            log.Println("Changes in services have been recorded:")
            log.Println(resultOfServicesCheck)
            fmt.Fprintf(&summaryReport, "\n========= Changes to the services have been recorded =========\n")
            summaryReport.WriteString(resultOfServicesCheck)
            doc := logslibs.MakeJsonDocument(HostName, "services", "changed", resultOfServicesCheck)
            summaryJSONReport = append(summaryJSONReport, doc)
            if cfg.Log.OpenSearch.Enable && opensearchClient != nil && !cfg.Log.OpenSearch.Bulk {
                // doc := logslibs.MakeJsonDocument(
                    // HostName,
                    // "services",
                    // "changed",
                    // resultOfServicesCheck,
                // )
                if !logslibs.OsAddRecord(opensearchClient, indexName, doc) {
                    log.Println("warn: failed to send 'services' check to OpenSearch")
                }
            }
        } else {
            log.Println("No service changes have been recorded.")
        }
    }

    // *********** Sending Summary Report ***************
	if len(summaryReport.String()) != 0 {
		// log.Println("Result of all checks:", summaryReport.String())
		SendNotifycation(cfg, summaryReport.String())
	}

    // *********** Sending Summary Report to OpenSearch***************
    if cfg.Log.OpenSearch.Enable && opensearchClient != nil && cfg.Log.OpenSearch.Bulk {
        if !logslibs.OsAddRecordsBulk(opensearchClient, indexName, summaryJSONReport) {
            log.Println("warn: failed to send bulk report to OpenSearch")
        }
    }

}

func RunService(ctx context.Context, cfg *config.Config) {
	timeout := time.Duration(cfg.Daemon.CheckTimeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

    LoadState(cfg)

	for {
		runChecks(cfg)

		select {
		case <-ctx.Done():
			return
		case <-time.After(timeout):
		}
	}
    SaveState(cfg)
}

func DaemonRun(cfg *config.Config) {
        cntxt := &daemon.Context{
        PidFileName: cfg.Daemon.PidFile,
        PidFilePerm: 0600,
        LogFileName: cfg.Log.LogFile.Path,
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

    LoadState(cfg)
    RunService(ctx, cfg)
    SaveState(cfg)

    log.Println("Daemon stopped")
}

func DaemonStop(pidFile string) error {
    data, err := os.ReadFile(pidFile)
    if err != nil {
        if os.IsNotExist(err) {
            return fmt.Errorf("daemon is not running: pid file %s not found", pidFile)
        }
        return fmt.Errorf("read pid file %s: %w", pidFile, err)
    }

    pidStr := strings.TrimSpace(string(data))
    if pidStr == "" {
        return fmt.Errorf("pid file %s is empty", pidFile)
    }

    pid, err := strconv.Atoi(pidStr)
    if err != nil {
        return fmt.Errorf("invalid pid %q in %s: %w", pidStr, pidFile, err)
    }

    proc, err := os.FindProcess(pid)
    if err != nil {
        return fmt.Errorf("find process %d: %w", pid, err)
    }

    if err := proc.Signal(syscall.SIGTERM); err != nil {
        if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
            _ = os.Remove(pidFile)
            return nil
        }
        return fmt.Errorf("signal process %d: %w", pid, err)
    }

    log.Printf("Daemon (pid %d) stopped.\n", pid)
    return nil
}

func SendNotifycation(cfg *config.Config, msg string) {
    // E-Mail notification
	if cfg.Notify.Email.Enable {
		var (
			message     notifylibs.MailMessage
			smtpSession notifylibs.SmtpSession
		)

		log.Println("Running notifylibs/EmailNotify on ", HostName)

		message.MailFrom = cfg.Notify.Email.MailFrom
		message.MailTo = cfg.Notify.Email.MailTo
		// %HOSTNAME% template in Subject will replace
		message.MailSubject = strings.ReplaceAll(cfg.Notify.Email.MailSubject, "%HOSTNAME%", HostName)
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

    // Matrix notification
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

    // Webhook notification
    if cfg.Notify.Webhook.Enable {
        var webhookSession notifylibs.WebhookClient
        webhookSession.URL = cfg.Notify.Webhook.URL
        webhookSession.User = cfg.Notify.Webhook.User
        webhookSession.Password = cfg.Notify.Webhook.Password
        webhookSession.MessageFormat = cfg.Notify.Webhook.Message.Format

        escMsg := msg
        if cfg.Notify.Webhook.Message.Format == "json" {
	        escMsg = mainlibs.EscapeJSONString(msg)
        }
        messageForSending := strings.ReplaceAll(cfg.Notify.Webhook.Message.Template, "%HOSTNAME%", HostName)
        messageForSending = strings.ReplaceAll(messageForSending, "%MESSAGE%", escMsg)

        ok, respBody := notifylibs.WebhookNotify(webhookSession, messageForSending)
        if ok {
            log.Println("Webhook sent successfully, response:", respBody)
        } else {
            log.Println("Webhook send error")
        }
    }
}


func SaveStateToFile(path string, mapState any) error {
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    defer f.Close()
    log.Println("Saving current check state", path)

    return gob.NewEncoder(f).Encode(mapState)
}

func LoadStateFromFile(path string, mapState any) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    defer f.Close()
    log.Println("Loading previous check state", path)
    return gob.NewDecoder(f).Decode(mapState)
}

// Saving the state of checks when exiting the program
func SaveState(cfg *config.Config) {
    save := func(name string, src any) {
        path := filepath.Join(cfg.Daemon.WorkDir, name)
        if err := SaveStateToFile(path, src); err != nil {
            log.Printf("warn: cannot save state to %s: %v", path, err)
        }
    }

    save("previousFileList", previousFileList)
    save("previousUrlList", previousUrlList)
    save("previousUsersList", previousUsersList)
    save("previousServiceList", previousServiceList)
    save("previousProcessList", previousProcessList)
    save("previousPortList", previousPortList)
}

// LoadState loads the state of previous checks from files.
func LoadState(cfg *config.Config) {
    load := func(name string, dst any) {
        path := filepath.Join(cfg.Daemon.WorkDir, name)
        if !mainlibs.IsFile(path) {
            return
        }
        if err := LoadStateFromFile(path, dst); err != nil {
            log.Printf("warn: cannot load state from %s: %v", path, err)
        } else {
            if err = os.Remove(filepath.Join(cfg.Daemon.WorkDir, name)); err != nil {
                log.Printf("Delete file %s error %v:", path, err)
            }
        }
    }

    load("previousFileList", &previousFileList)
    load("previousUrlList", &previousUrlList)
    load("previousUsersList", &previousUsersList)
    load("previousServiceList", &previousServiceList)
    load("previousProcessList", &previousProcessList)
    load("previousPortList", &previousPortList)
}


func main() {
	flag.BoolVar(&RunAsDaemon, "daemon-start", false, "Run the programm as a daemon")
	flag.BoolVar(&StopDaemon, "daemon-stop", false, "Stop the programm runing as a daemon")
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
		if err := config.ConfigMake(config.ConfigTemplate, ConfigFile); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	cfg, err := config.ConfigLoad(ConfigFile)
	if err != nil {
		log.Fatal(err)
	}

	err = os.MkdirAll(cfg.Daemon.WorkDir, 0700)
	if err != nil {
		log.Fatal(err)
	}

	HostName, err = mainlibs.GetFQDN()
	if err != nil {
		log.Fatal(err)
	} else {
        	log.Println("Host name is:", HostName)
    }

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

    if StopDaemon {
        err = DaemonStop(cfg.Daemon.PidFile)
        if err != nil {
            log.Println("Daemon stop error:", err)
        }
        os.Exit(0)
    }


    if cfg.Log.OpenSearch.Enable {
        opensearchClient = logslibs.OsConnect(cfg.Log.OpenSearch)
        if opensearchClient != nil && cfg.Log.OpenSearch.CreateTemplate {
             if err := logslibs.CreateIndexTemplate(
                    opensearchClient,
                    "monitoring-template",  // имя шаблона
                    "monitoring-*",         // маска индексов
            ); err != nil {
                log.Printf("warn: ensure index template: %v", err)
            }
            if cfg.Log.OpenSearch.CreateIndex {
                indexName := logslibs.ConvertIndexName(cfg.Log.OpenSearch.IndexName)
                logslibs.OsCreateIndex(indexName, opensearchClient)
            }
        }
    }

    if RunAsDaemon {
        DaemonRun(cfg)
    } else if RunOnce {
        LoadState(cfg)
		runChecks(cfg)
        SaveState(cfg)
    } else {
        RunService(ctx, cfg)
    }

}
