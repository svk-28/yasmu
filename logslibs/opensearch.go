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
    "io"
    "log"
    "fmt"
    "context"
    "os"
    "time"
    "net"
    "crypto/tls"
    "net/http"
    "strings"
    "encoding/json"

    opensearch "github.com/opensearch-project/opensearch-go/v2"
    opensearchapi "github.com/opensearch-project/opensearch-go/v2/opensearchapi"

    "yasmu/config"
)

var Debug bool
var opensearchUrl string

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

// CloseIdleConnections()
func OsConnect(cfg config.OpenSearchConfig) *opensearch.Client {
    // Initialize the Client with SSL/TLS enabled.
    opensearchUrl = fmt.Sprintf("%s://%s:%d", cfg.Protocol, cfg.Server, cfg.Port)

    OsClient, err := opensearch.NewClient(opensearch.Config{
        Transport: &http.Transport{
            MaxIdleConns:          10,
            MaxIdleConnsPerHost:   10,
            ResponseHeaderTimeout: 300 * time.Second,
            IdleConnTimeout:       1 * time.Millisecond,
            DisableKeepAlives: true,
            // ForceAttemptHTTP2:     true,
            MaxConnsPerHost:       20,
            DialContext: (&net.Dialer{
                Timeout: 300 * time.Second,
                // KeepAlive: 10 * time.Millisecond,
            }).DialContext,
            TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.CertVerify},
        },
        Addresses: []string{opensearchUrl},
        Username:  cfg.User,
        Password:  cfg.Password,
    })
    if err != nil {
        log.Println("cannot initialize", err)
        os.Exit(1)
    }
    if cfg.DiscoverNode {
        OsClient.DiscoverNodes()
    }
    
    if err != nil {
        log.Println("OpenSearch Node discovery error :", err)
        os.Exit(1)
    }
    log.Println("Connecting to OpenSearch:", OsClient)
    return OsClient
    
}

// Create index
func OsCreateIndex(indexName string, client *opensearch.Client) {
    mapping := strings.NewReader(IndexTemplate)
   // Create an index with non-default settings.
    createIndex := opensearchapi.IndicesCreateRequest{
        Index: indexName,
        Body:  mapping,
   }

   if Debug {
        log.Println("Create index:", createIndex)
    }
    createIndexResponse, err := createIndex.Do(context.Background(), client)
    if err != nil {
        log.Println("failed to create index ", err, createIndexResponse)
        os.Exit(1)
    }
    if Debug {
        log.Println("Create index:", createIndex)
    }
    fmt.Println(createIndexResponse.StatusCode)
    if strings.Contains(createIndexResponse.String(), "already exists") {
        log.Println(createIndexResponse.IsError())
        log.Println("Index", indexName, "already exists")
    } else if createIndexResponse.StatusCode == 200 {
        log.Println("Создан индекс:", indexName)    
    }
}


// Add record
func OsAddRecord(client *opensearch.Client, indexName string, line map[string]string) bool {
    if len(line) == 0 {
        return false
    }
    var operResult bool
    aJson, err := json.MarshalIndent(line, "", "  ")
    if err != nil {
        log.Printf("Error encoding: %s", line, "with error %s", err)
    }
    if Debug {
        log.Println(line)
    }
    document := strings.NewReader(string(aJson))
    if Debug {
        log.Println(document)
    }
    req := opensearchapi.IndexRequest{
        Index:      indexName,
        Body:       document,
    }

    if Debug {
        log.Println(req)
    }
    insertResponse, err := req.Do(context.Background(), client)
    // res, err := req.Do(context.Background(), client)
    if err != nil {
        log.Println("failed to insert document ", err, line)
        // os.Exit(1)
        operResult = false
    } else {
        operResult = true
    }

    // Check the document insertion result.
    // If the response status is not 200, retry several times.
    if insertResponse.StatusCode == 200 && operResult {
        log.Println("Пакет данных добавлен")
    } else {
        j := 1
        for j <= 5 {
            time.Sleep(2 * time.Second)
            insertResponse, err := req.Do(context.Background(), client)
            if err != nil {
                log.Println("Error adding record to the opensearch (esAddRecords):", err)
            }
            if insertResponse.StatusCode == 200 {
                operResult = true
                break
            }
            j++
        }
    }

    if Debug {
        log.Printf("Response from %s when adding the data packet: %s", opensearchUrl, insertResponse)
    }

    defer insertResponse.Body.Close()
    return operResult
}

func OsAddRecordsBulk(client *opensearch.Client, indexName string, arr []map[string]string) bool {
    if Debug {
        log.Println("================= Data package sending ===================")
        log.Println("Index:", indexName)
    }
    if len(arr) == 0 {
        return false
    }
    var (
        operResult bool
        resultJson string
    )
    resultJson = ""
    for _, line := range arr {
        if len(line) != 0 {
            resultJson += fmt.Sprintf("{\"create\": { \"_index\": \"%s\"}}\n", indexName)
            aJson, err := json.Marshal(line)
            if err != nil {
                log.Printf("Error encoding: %s", line, "with error %s", err)
            } else {
                 resultJson += fmt.Sprintf("%s\n", string(aJson))
            }
        }
    }
    // _ = strings.NewReader(string(aJson))
    if Debug {
        log.Println(string(resultJson))
    }

    r := strings.NewReader(resultJson)
    bulkResp, err := client.Bulk(r)
    if err != nil {
        log.Println("Error adding the data packet to OpenSearch (esAddRecordsBulk procedure):", err)
        operResult = false
    } else {
        operResult = true
    }

    // Check the insertion result; if the status code is not 200,
    // retry several times.
    if bulkResp.StatusCode == 200 && operResult {
        if Debug {
            log.Println("Пакет данных добавлен")
        }
    } else {
        j := 1
        for j <= 5 {
            time.Sleep(2 * time.Second)
            bulkResp, err = client.Bulk(r)
            if err != nil {
                log.Println("Error retrying to add the data packet to OpenSearch (esAddRecordsBulk procedure):", err)
            }
            if bulkResp.StatusCode == 200 {
                operResult = true
                break
            }
            j++
        }
    }
    if Debug {
        log.Printf("Response from %s when adding the data packet: %s", opensearchUrl, bulkResp)
    }

    defer bulkResp.Body.Close()
    return operResult
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
