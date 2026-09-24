// ********************************************************************
// Watchdog - the utility for monitoring system integrity.
// And for web monitoring of websites availability.
// ********************************************************************
// Author: Sergey Kalinin
// License: GPL v3
// ********************************************************************
// Module for sending messages to the Matrix
// ********************************************************************

package notifylibs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
    "github.com/google/uuid"
)

type MatrixClient struct {
	HomeServer   string
	UserName     string
	UserPassword string
	AccessToken  string
	RoomID       string
	HTTPClient   *http.Client
}

func MatrixNewClient(matrixSession MatrixClient) *MatrixClient {
	c := matrixSession

	// HomeServer: убираем trailing slash и добавляем схему, если её нет.
	c.HomeServer = strings.TrimRight(c.HomeServer, "/")
	if c.HomeServer != "" &&
		!strings.HasPrefix(c.HomeServer, "http://") &&
		!strings.HasPrefix(c.HomeServer, "https://") {
		c.HomeServer = "https://" + c.HomeServer
	}

	// RoomID: если не содержит ":", значит указан только localpart —
	// добавляем ! и имя сервера, извлечённое из HomeServer.
	if c.RoomID != "" && !strings.Contains(c.RoomID, ":") {
		c.RoomID = fmt.Sprintf("!%s:%s",
			strings.TrimPrefix(c.RoomID, "!"),
			extractServerName(c.HomeServer),
		)
	}

	// UserName: приводим к формату @user:server.
	if c.UserName != "" {
		c.UserName = normalizeMatrixID(c.UserName, "@", extractServerName(c.HomeServer))
	}

	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}

	return &c
}

// extractServerName вытаскивает host из URL HomeServer.
func extractServerName(homeserver string) string {
	s := strings.TrimPrefix(homeserver, "https://")
	s = strings.TrimPrefix(s, "http://")
	if idx := strings.Index(s, "/"); idx != -1 {
		s = s[:idx]
	}
	return s
}

// normalizeMatrixID приводит идентификатор к полному виду symbol+username:server.
// Если в строке уже есть ":" — считаем, что сервер указан, и только проверяем symbol.
// Если ":" нет — добавляем symbol и server.
func normalizeMatrixID(id, symbol, server string) string {
	id = strings.TrimSpace(id)

	// Если сервер уже указан — проверяем только symbol.
	if strings.Contains(id, ":") {
		if !strings.HasPrefix(id, symbol) {
			id = symbol + id
		}
		return id
	}

	// Сервер не указан — добавляем symbol и server.
	id = strings.TrimPrefix(id, symbol)
	return fmt.Sprintf("%s%s:%s", symbol, id, server)
}

func (c *MatrixClient) MatrixSendText(ctx context.Context, text string) (string, error) {
	if c.AccessToken == "" {
		return "", fmt.Errorf("matrix: empty access token")
	}
	if c.RoomID == "" {
		return "", fmt.Errorf("matrix: empty room ID")
	}

    msgID := uuid.New().String()
    
	endpoint := fmt.Sprintf(
		"%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		c.HomeServer, url.PathEscape(c.RoomID), msgID,
	)

	payload := map[string]string{"msgtype": "m.text", "body": text}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("matrix error: %d %s", resp.StatusCode, respBody)
	}

	var out struct {
		EventID string `json:"event_id"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("unmarshal: %w", err)
	}
	return out.EventID, nil
}

// MatrixNotify отправляет сообщение, используя данные из matrixSession.
// Возвращает (успех, event_id).
func MatrixNotify(matrixSession MatrixClient, message string) (bool, string) {
	fmt.Println("\nRunning notifylibs/MatrixNotify")

    c := MatrixNewClient(matrixSession)

	eventID, err := c.MatrixSendText(context.Background(), message)
	if err != nil {
		log.Println("Error sending message to matrix: ", err)
		return false, ""
	}
	return true, eventID
}
