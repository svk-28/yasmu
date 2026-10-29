// *******************************************************
// YASMU - Yet Another System Monitoring Utility
// The utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
// Home page: https://nuk-svk.ru
// License: GPL v3
// *******************************************************
//  A module for sending data to OpenSearch.
// *******************************************************

package logslibs

import (
    "context"
    "crypto/tls"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net"
    "net/http"
    // "os"
    "strings"
    "time"
    
    opensearch "github.com/opensearch-project/opensearch-go/v2"
    opensearchapi "github.com/opensearch-project/opensearch-go/v2/opensearchapi"
    
    "yasmu/config"
)

var Debug bool
var opensearchUrl string

// Количество повторных попыток при ошибках отправки.
const osMaxRetries = 2

// Пауза между повторными попытками.
const osRetryDelay = 2 * time.Second

var IndexTemplate = `{
    "settings" : { },
    "mappings" : {
      "properties" : {
        "@timestamp" : {
          "type" : "date"
        },
        "Server" : {
          "type" : "keyword",
          "ignore_above": 256
        },
        "CheckName" : {
          "type" : "keyword"
        },
        "CheckStatus" : {
          "type" : "keyword"
        },
        "CheckData" : {
          "type" : "text"
        }
      }
    }
}`

func OsConnect(cfg config.OpenSearchConfig) (*opensearch.Client, error) {
	opensearchUrl = fmt.Sprintf("%s://%s:%d", cfg.Protocol, cfg.Server, cfg.Port)

	client, err := opensearch.NewClient(opensearch.Config{
		Transport: &http.Transport{
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   10,
			ResponseHeaderTimeout: 300 * time.Second,
			IdleConnTimeout:       1 * time.Millisecond,
			DisableKeepAlives:     true,
			MaxConnsPerHost:       20,
			DialContext: (&net.Dialer{
				Timeout: 300 * time.Second,
			}).DialContext,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.CertVerify},
		},
		Addresses: []string{opensearchUrl},
		Username:  cfg.User,
		Password:  cfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot initialize opensearch client: %w", err)
	}

	if cfg.DiscoverNode {
		if err := client.DiscoverNodes(); err != nil {
			log.Println("OpenSearch node discovery error:", err)
		}
	}

	log.Printf("OpenSearch client initialized for %s", opensearchUrl)
	return client, nil
}

func OsPing(client *opensearch.Client) error {
	if client == nil {
		return fmt.Errorf("opensearch client is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Ping(client.Ping.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("opensearch ping: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("opensearch ping: empty response")
	}
	defer resp.Body.Close()

	if resp.IsError() {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("opensearch ping: %s: %s", resp.Status(), strings.TrimSpace(string(raw)))
	}
	return nil
}

func OsCreateIndex(indexName string, client *opensearch.Client) error {
	if client == nil {
		return fmt.Errorf("opensearch client is nil")
	}
	if indexName == "" {
		return fmt.Errorf("index name is empty")
	}

	req := opensearchapi.IndicesCreateRequest{
		Index: indexName,
		Body:  strings.NewReader(IndexTemplate),
	}

	if Debug {
		log.Printf("Create index: %s", indexName)
	}

	resp, err := req.Do(context.Background(), client)
	if err != nil {
		return fmt.Errorf("create index %q: %w", indexName, err)
	}
	if resp == nil {
		return fmt.Errorf("create index %q: empty response", indexName)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	body := strings.TrimSpace(string(raw))

	switch {
	case strings.Contains(body, "already exists"):
		log.Printf("Index %q already exists", indexName)
		return nil
	case resp.IsError():
		return fmt.Errorf("create index %q: %s: %s", indexName, resp.Status(), body)
	case resp.StatusCode == http.StatusOK:
		log.Printf("Создан индекс: %s", indexName)
	default:
		log.Printf("create index %q: unexpected status %s: %s", indexName, resp.Status(), body)
	}
	return nil
}

func OsAddRecord(client *opensearch.Client, indexName string, line map[string]string) bool {
    if client == nil || len(line) == 0 {
        return false
    }
    
    aJson, err := json.Marshal(line)
    if err != nil {
        log.Printf("Error encoding record: %v", err)
        return false
    }
    if Debug {
        log.Printf("Index %q, doc: %s", indexName, aJson)
    }
    
    payload := string(aJson)
    
    for attempt := 0; attempt <= osMaxRetries; attempt++ {
        if attempt > 0 {
            time.Sleep(osRetryDelay)
        }
        
        req := opensearchapi.IndexRequest{
            Index: indexName,
            Body:  strings.NewReader(payload),
        }
        
        resp, err := req.Do(context.Background(), client)
        
        if err != nil {
            log.Printf("Insert attempt %d/%d failed: %v", attempt+1, osMaxRetries+1, err)
            continue
        }
        
        if resp == nil {
            log.Printf("Insert attempt %d/%d: empty response", attempt+1, osMaxRetries+1)
            continue
        }
        
        if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
            resp.Body.Close()
            log.Printf("Record added to index %s (status %s)", indexName, resp.Status())
            return true
        }
        
        log.Printf("Insert attempt %d/%d: status %s", attempt+1, osMaxRetries+1, resp.Status())
        resp.Body.Close()
    }
    
    return false
}

func OsAddRecordsBulk(client *opensearch.Client, indexName string, arr []map[string]string) bool {
	if client == nil || len(arr) == 0 {
		return false
	}
	if Debug {
		log.Println("================= Data package sending ===================")
		log.Printf("Index: %s, records: %d", indexName, len(arr))
	}

	var sb strings.Builder
	written := 0
	for _, line := range arr {
		if len(line) == 0 {
			continue
		}
		aJson, err := json.Marshal(line)
		if err != nil {
			log.Printf("Error encoding record: %v", err)
			continue
		}
		sb.WriteString(fmt.Sprintf("{\"create\": { \"_index\": \"%s\"}}\n", indexName))
		sb.Write(aJson)
		sb.WriteByte('\n')
		written++
	}
	if written == 0 {
		log.Println("bulk: no valid records to send")
		return false
	}

	payload := sb.String()
	if Debug {
		log.Printf("bulk payload (%d records):\n%s", written, payload)
	}

	for attempt := 0; attempt <= osMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(osRetryDelay)
		}

		resp, err := client.Bulk(strings.NewReader(payload))
		if err != nil {
			log.Printf("bulk attempt %d/%d failed: %v", attempt+1, osMaxRetries+1, err)
			continue
		}
		if resp == nil {
			log.Printf("bulk attempt %d/%d: empty response", attempt+1, osMaxRetries+1)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			if Debug {
				log.Println("Пакет данных добавлен")
			}
			return true
		}

		log.Printf("bulk attempt %d/%d: status %s", attempt+1, osMaxRetries+1, resp.Status())
		resp.Body.Close()
	}

	return false
}

func MakeJsonDocument(server, checkName, checkStatus, checkData string) map[string]string {
	return map[string]string{
		"@timestamp":      time.Now().UTC().Format(time.RFC3339),
		"Server":          server,
		"ApplicationName": "YASMU",
		"CheckName":       checkName,
		"CheckStatus":     checkStatus,
		"CheckData":       checkData,
	}
}

func ConvertIndexName(indexName string) string {
	now := time.Now()
	replacer := strings.NewReplacer(
		"%YYYY%", now.Format("2006"),
		"%YY%",   now.Format("06"),
		"%MM%",   now.Format("01"),
		"%DD%",   now.Format("02"),
		"%HH%",   now.Format("15"),
		"%mm%",   now.Format("04"),
		"%ss%",   now.Format("05"),
	)
	return replacer.Replace(indexName)
}

func CreateIndexTemplate(client *opensearch.Client, templateName string, indexPattern string) error {
	if client == nil {
		return fmt.Errorf("opensearch client is nil")
	}
	if templateName == "" {
		return fmt.Errorf("template name is empty")
	}
	if indexPattern == "" {
		return fmt.Errorf("index pattern is empty")
	}

	body := fmt.Sprintf(`{
        "index_patterns": ["%s"],
        "template": %s
    }`, indexPattern, IndexTemplate)

	req := opensearchapi.IndicesPutIndexTemplateRequest{
		Name: templateName,
		Body: strings.NewReader(body),
	}

	resp, err := req.Do(context.Background(), client)
	if err != nil {
		return fmt.Errorf("put index template %q: %w", templateName, err)
	}
	if resp == nil {
		return fmt.Errorf("put index template %q: empty response", templateName)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("put index template %q: %s: %s",
			templateName, resp.Status(), strings.TrimSpace(string(raw)))
	}

	log.Printf("opensearch: index template %q ensured for pattern %q",
		templateName, indexPattern)
	return nil
}
