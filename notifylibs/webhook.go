// ********************************************************************
// Watchdog - the utility for monitoring system integrity.
// And for web monitoring of websites availability.
// ********************************************************************
// Author: Sergey Kalinin
// License: GPL v3
// ********************************************************************
// Module for sending messages to a webhook.
// Content-Type: "application/json"
// {"message": "Message text"}
// ********************************************************************

package notifylibs

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"
    "log"
)

type WebhookClient struct {
    URL           string
    User          string
    Password      string
    MessageFormat string
    HTTPClient    *http.Client
}

func WebhookNewClient(cfg WebhookClient) *WebhookClient {
	c := cfg

	c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	if c.URL != "" &&
		!strings.HasPrefix(c.URL, "http://") &&
		!strings.HasPrefix(c.URL, "https://") {
		c.URL = "https://" + c.URL
	}

	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}

	return &c
}

func (c *WebhookClient) WebhookSend(ctx context.Context, message string) (string, error) {
	if c.URL == "" {
		return "", fmt.Errorf("webhook: empty URL")
	}

	var contentType string
	switch c.MessageFormat {
	case "text":
		contentType = "text/plain; charset=utf-8"
	default: // "json" and more
		contentType = "application/json"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader([]byte(message)))
	if err != nil {
		return "", fmt.Errorf("webhook: new request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	if c.User != "" {
		req.SetBasicAuth(c.User, c.Password)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("webhook: do: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("webhook: bad status %d: %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

func WebhookNotify(cfg WebhookClient, message string) (bool, string) {
	log.Println("\nRunning notifylibs/WebhookNotify")

	c := WebhookNewClient(cfg)

	respBody, err := c.WebhookSend(context.Background(), message)
	if err != nil {
		log.Println("Error sending webhook: ", err)
		return false, ""
	}
	return true, respBody
}
