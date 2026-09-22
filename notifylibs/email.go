package notifylibs

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
   	"errors"
    "time"
)

type SmtpSession struct {
	SmtpUser     string
	SmtpPassword string
	SmtpServer   string
	SmtpPort     string
}

type MailMessage struct {
	MailFrom    string
	MailTo      string
	MailSubject string
	MailMessage string
}

type loginAuth struct {
	username string
	password string
}

func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, errors.New("unknown server challenge: " + string(fromServer))
		}
	}
	return nil, nil
}


func EmailNotify_(smtpSession *SmtpSession, message *MailMessage) bool {

	// Receiver email address.
	to := []string{
		message.MailTo,
	}

	serverName := smtpSession.SmtpServer + ":" + smtpSession.SmtpPort
	log.Println("SMTP Server", serverName, smtpSession.SmtpUser, smtpSession.SmtpPassword)

	conn, err := net.Dial("tcp", serverName)
	if err != nil {
		log.Println(err)
        return false
	}

	c, err := smtp.NewClient(conn, smtpSession.SmtpServer)
	if err != nil {
		log.Println(err)
        return false
	}

	tlsconfig := &tls.Config{
		ServerName: smtpSession.SmtpServer,
	}

	if err = c.StartTLS(tlsconfig); err != nil {
		log.Println(err)
	}

	auth := LoginAuth(smtpSession.SmtpUser, smtpSession.SmtpPassword)

	if err = c.Auth(auth); err != nil {
		log.Println(err)
        return false
    }

	msg := []byte("From: " + message.MailFrom + "\r\n" +
		"To: " + message.MailTo + "\r\n" +
		"Subject: " + message.MailSubject + "\r\n\r\n" +
		message.MailMessage + "\r\n")


	// Sending email.
	err = smtp.SendMail(serverName, auth, message.MailFrom, to, msg)

    log.Println(err)

	if err != nil {
		fmt.Println(err)
		return false
	}
	// fmt.Println("Email Sent!")

	if err != nil {
		log.Fatal(err)
		return false
	} else {
        log.Println("Email", "from", message.MailFrom, "sent successfully via", smtpSession.SmtpServer)
		return true
	}
}


func EmailNotify(smtpSession *SmtpSession, message *MailMessage) bool {
	log.Println("\nRunning notifylibs/EmailNotify")

	serverName := net.JoinHostPort(smtpSession.SmtpServer, smtpSession.SmtpPort)
	log.Printf("SMTP Server %s, user %s", serverName, smtpSession.SmtpUser)

	dialer := &net.Dialer{Timeout: 10 * time.Second}

	tlsConfig := &tls.Config{
		ServerName: smtpSession.SmtpServer,
		MinVersion: tls.VersionTLS12,
	}

	var (
		conn net.Conn
		err  error
	)

	switch smtpSession.SmtpPort {
	    case "465":
		    conn, err = tls.DialWithDialer(dialer, "tcp", serverName, tlsConfig)
		    if err != nil {
			    log.Println("tls dial:", err)
			    return false
		    }
	    case "587", "25", "2525":
		    conn, err = dialer.Dial("tcp", serverName)
		    if err != nil {
			    log.Println("dial:", err)
			    return false
		    }
	    default:
		    log.Printf("unsupported SMTP port: %s", smtpSession.SmtpPort)
		    return false
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, smtpSession.SmtpServer)
	if err != nil {
		log.Println("new client:", err)
		return false
	}
	defer c.Close()

	if smtpSession.SmtpPort != "465" {
		if err = c.StartTLS(tlsConfig); err != nil {
			log.Println("starttls:", err)
			return false
		}
	}

	auth := LoginAuth(smtpSession.SmtpUser, smtpSession.SmtpPassword)
	if err = c.Auth(auth); err != nil {
		log.Println("auth:", err)
		return false
	}

	// Mail from 
	if err = c.Mail(message.MailFrom); err != nil {
		log.Println("mail from:", err)
		return false
	}
    // Mail to
	if err = c.Rcpt(message.MailTo); err != nil {
		log.Println("rcpt to:", err)
		return false
	}

	// Message body
	w, err := c.Data()
	if err != nil {
		log.Println("data:", err)
		return false
	}

	body := fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: text/plain; charset=UTF-8\r\n"+
			"\r\n"+
			"%s\r\n",
		message.MailFrom,
		message.MailTo,
		message.MailSubject,
		message.MailMessage,
	)

	if _, err = w.Write([]byte(body)); err != nil {
		log.Println("write body:", err)
		w.Close()
		return false
	}
	if err = w.Close(); err != nil {
		log.Println("close body:", err)
		return false
	}

	if err = c.Quit(); err != nil {
		log.Println("quit:", err)
	}

	log.Printf("Email from %s sent successfully via %s", message.MailFrom, smtpSession.SmtpServer)
	return true
}
