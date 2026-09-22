package checklibs

import (
	"crypto/tls"
	"log"
	"io"
	"net/http"
	// "os"
	"time"
    "fmt"
    "strings"
)

type UrlMap map[string]UrlCheckInfo

type UrlCheckInfo struct {
    CheckStatus  bool
    ResponseCode int
    ResponseText string
}

func WebCheck(url string) (bool, string) {
    var (
        result strings.Builder
        status bool
    )

    log.Println("Running checklibs/WebCheck")
    transport := &http.Transport{
        TLSClientConfig: &tls.Config{
        MinVersion: tls.VersionTLS12,
            // InsecureSkipVerify: true, // НЕ используйте в продакшене!
        },
    }

        client := &http.Client{
        Timeout:   15 * time.Second,
        Transport: transport,
    }

    req, err := http.NewRequest(http.MethodGet, url, nil)
    if err != nil {
        log.Printf("Making the request error: %v\n", err)
    }

    req.Header.Set("User-Agent", "Go-HTTPS-Client/1.0")

    resp, err := client.Do(req)
    if err != nil {
        log.Printf("Request error: %v\n", err)
        fmt.Fprintf(&result, "URL: %s\n", url)
        fmt.Fprintf(&result, "Status: %s\n", err)
        return false, result.String()
    }
    defer resp.Body.Close()

    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        log.Printf("HTTP error: %s\n", resp.Status)
        fmt.Fprintf(&result, "Web check alarm on host %s:\n", url)
        status = false
    } else {
        status = true
    }

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        log.Printf("Request response reading error: %v\n", err)
    }

    if status {
        fmt.Fprintf(&result, "URL: %s\n", url)
        fmt.Fprintf(&result, "Status: %s\n", resp.Status)
        // log.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
        // log.Printf("Size: %d байт\n", len(body))
        // log.Println("---")
        // log.Println(string(body))
    } else {
        fmt.Fprintf(&result, "URL: %s\n", url)
        fmt.Fprintf(&result, "Status: %s\n", resp.Status)
        fmt.Fprintf(&result, "Content-Type: %s\n", resp.Header.Get("Content-Type"))
        fmt.Fprintf(&result, "Size: %d байт\n", len(body))
        fmt.Fprintf(&result, "---")
        fmt.Fprintf(&result, "%s\n", string(body))
    }

    return status, result.String()
}
