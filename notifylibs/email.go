package notifylibs

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
   	"errors"
)

type SmtpSession struct {
	smtpUser     string
	smtpPassword string
	smtpServer   string
	smtpPort     string
}

type MailMessage struct {
	mailFrom    string
	mailTo      string
	mailSubject string
	mailMessage string
	messageUUID string
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


func SendEmail(smtpSession *SmtpSession, message *MailMessage) bool {

	// Receiver email address.
	to := []string{
		message.mailTo,
	}

	serverName := smtpSession.smtpServer + ":" + smtpSession.smtpPort
	// fmt.Println(serverName, smtpSession.smtpUser, smtpSession.smtpPassword)

	conn, err := net.Dial("tcp", serverName)
	if err != nil {
		log.Println(err)
	}

	c, err := smtp.NewClient(conn, smtpSession.smtpServer)
	if err != nil {
		log.Println(err)
	}

	tlsconfig := &tls.Config{
		ServerName: smtpSession.smtpServer,
	}

	if err = c.StartTLS(tlsconfig); err != nil {
		log.Println(err)
	}

	auth := LoginAuth(smtpSession.smtpUser, smtpSession.smtpPassword)

	if err = c.Auth(auth); err != nil {
		log.Println(err)
	}

	msg := []byte("From: " + message.mailFrom + "\r\n" +
		"To: " + message.mailTo + "\r\n" +
		"Subject: " + message.mailSubject + "\r\n\r\n" +
		message.mailMessage + "\r\n")

	// Sending email.
	err = smtp.SendMail(serverName, auth, message.mailFrom, to, msg)

	if err != nil {
		fmt.Println(err)
		return false
	}
	// fmt.Println("Email Sent!")

	if err != nil {
		log.Fatal(err)
		return false
	} else {
		log.Println("Email", message.messageUUID, "from", message.mailFrom, "sent successfully via", smtpSession.smtpServer)
		return true
	}
}

func EmailNotify() {
    fmt.Println("\nRunning notifylibs/EmailNotify")
}
