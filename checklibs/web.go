package checklibs

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func WebCheck(url string) {
    fmt.Println("\nRunning checklibs/WebCheck")
	// Настраиваем транспорт (можно отключить проверку сертификата при необходимости)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// InsecureSkipVerify: true, // НЕ используйте в продакшене!
		},
	}

	// Создаём клиент с таймаутом
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
	}

	// Формируем запрос
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка создания запроса: %v\n", err)
		os.Exit(1)
	}

	// Устанавливаем User-Agent
	req.Header.Set("User-Agent", "Go-HTTPS-Client/1.0")

	// Выполняем запрос
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка запроса: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Проверяем статус
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "HTTP ошибка: %s\n", resp.Status)
	}

	// Читаем тело
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка чтения: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("URL: %s\n", url)
	fmt.Printf("Статус: %s\n", resp.Status)
	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
	fmt.Printf("Размер: %d байт\n", len(body))
	fmt.Println("---")
	fmt.Println(string(body))
}
