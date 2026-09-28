// *******************************************************
// YASMU - Yet Another System Monitoring Utility
// The utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
// License: GPL v3
// *******************************************************
// WEB (URL) checking module
// *******************************************************

package checklibs

import (
    "crypto/tls"
    "fmt"
    "io"
    "log"
    // "maps"
    "net/http"
    "sort"
    "strings"
    "time"
)

type UrlCheckInfo struct {
    CheckStatus  bool   // true — 2xx, false — another
    ResponseCode int    // HTTP-response code (0, if response is not sucess)
    ResponseText string // тело ответа (для отчёта об ошибке)
    Error        string // текст ошибки транспорта (пусто, если запрос удался)
}

type UrlList map[string]UrlCheckInfo

type UrlDiff struct {
    URL     string
    Changes []FieldChange
}

type UrlDiffResult struct {
    OnlyInBase   []string
    OnlyInActual []string
    Changed      []UrlDiff
    Unchanged    []string
}

func CompareUrlLists(base, actual UrlList) UrlDiffResult {
    var res UrlDiffResult

    for url, b := range base {
        a, ok := actual[url]
        if !ok {
            res.OnlyInBase = append(res.OnlyInBase, url)
            continue
        }

        var changes []FieldChange

        if b.CheckStatus != a.CheckStatus {
            changes = append(changes, FieldChange{
                "status",
                fmt.Sprint(b.CheckStatus),
                fmt.Sprint(a.CheckStatus),
            })
        }
        if b.ResponseCode != a.ResponseCode {
            changes = append(changes, FieldChange{
                "code",
                fmt.Sprint(b.ResponseCode),
                fmt.Sprint(a.ResponseCode),
            })
        }
        if b.Error != a.Error {
            changes = append(changes, FieldChange{"error", b.Error, a.Error})
        }

        if len(changes) > 0 {
            res.Changed = append(res.Changed, UrlDiff{URL: url, Changes: changes})
        } else {
            res.Unchanged = append(res.Unchanged, url)
        }
    }

    for url := range actual {
        if _, ok := base[url]; !ok {
            res.OnlyInActual = append(res.OnlyInActual, url)
        }
    }

    return res
}

func PrintUrlDiff(d UrlDiffResult, actual UrlList, showUnchanged bool) string {
    var sb strings.Builder

    sort.Strings(d.OnlyInBase)
    sort.Strings(d.OnlyInActual)

    if len(d.OnlyInBase) > 0 {
        fmt.Fprintf(&sb, "\nURLs removed from check (%d):\n", len(d.OnlyInBase))
        for _, u := range d.OnlyInBase {
            fmt.Fprintf(&sb, "   - %s\n", u)
        }
    }

    if len(d.OnlyInActual) > 0 {
        fmt.Fprintf(&sb, "\nURLs added to check (%d):\n", len(d.OnlyInActual))
        for _, u := range d.OnlyInActual {
            fmt.Fprintf(&sb, "   + %s\n", u)
        }
    }

    if len(d.Changed) > 0 {
        fmt.Fprintf(&sb, "\nChanged URLs (%d):\n", len(d.Changed))
        sort.Slice(d.Changed, func(i, j int) bool {
            return d.Changed[i].URL < d.Changed[j].URL
        })
        for _, ud := range d.Changed {
            fmt.Fprintf(&sb, "   ~ %s\n", ud.URL)
            for _, ch := range ud.Changes {
                fmt.Fprintf(&sb, "       %-6s: %q --> %q\n", ch.Field, ch.Old, ch.New)
            }
            // Если сейчас URL недоступен — добавим тело/ошибку для контекста.
            if info, ok := actual[ud.URL]; ok && !info.CheckStatus {
                if info.Error != "" {
                    fmt.Fprintf(&sb, "       detail : %s\n", info.Error)
                } else if info.ResponseText != "" {
                    body := strings.TrimSpace(info.ResponseText)
                    if len(body) > 200 {
                        body = body[:200] + "..."
                    }
                    fmt.Fprintf(&sb, "       body   : %s\n", body)
                }
            }
        }
    }

    if showUnchanged && len(d.Unchanged) > 0 {
        fmt.Fprintf(&sb, "\nUnchanged URLs (%d):\n", len(d.Unchanged))
        sort.Strings(d.Unchanged)
        for _, u := range d.Unchanged {
            fmt.Fprintf(&sb, "   = %s\n", u)
        }
    }

    fmt.Fprintf(&sb, "\nSummary: removed %d, added %d, changed %d, all %d\n",
        len(d.OnlyInBase), len(d.OnlyInActual),
        len(d.Changed), len(d.Unchanged))

    if len(d.OnlyInBase) > 0 || len(d.OnlyInActual) > 0 || len(d.Changed) > 0 {
        return sb.String()
    }
    return ""
}


func cloneUrlList(src UrlList) UrlList {
    dst := make(UrlList, len(src))
    for k, v := range src {
        dst[k] = v
    }
    return dst
}

func checkURL(url string, client *http.Client) (UrlCheckInfo, error) {
    info := UrlCheckInfo{}

    req, err := http.NewRequest(http.MethodGet, url, nil)
    if err != nil {
        info.Error = err.Error()
        return info, err
    }
    req.Header.Set("User-Agent", "SIMon/1.0")

    resp, err := client.Do(req)
    if err != nil {
        info.Error = err.Error()
        return info, err
    }
    defer resp.Body.Close()

    info.ResponseCode = resp.StatusCode
    info.CheckStatus = resp.StatusCode >= 200 && resp.StatusCode < 300

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        info.Error = "reading body: " + err.Error()
        return info, err
    }
    info.ResponseText = string(body)

    return info, nil
}

func WebCheck(urls []string, previousUrlList UrlList) (UrlList, string) {
    log.Println("\nRunning checklibs/WebCheck")

    currentUrlList := make(UrlList)
    var errs []string

    client := &http.Client{
        Timeout: 15 * time.Second,
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
        },
    }

    for _, url := range urls {
        info, err := checkURL(url, client)
        if err != nil {
            errs = append(errs, fmt.Sprintf("%s: %v", url, err))
        }
        currentUrlList[url] = info
    }

    for _, e := range errs {
        log.Printf("warn: %s", e)
    }

    if previousUrlList == nil {
        log.Printf("Baseline collected: %d URLs", len(currentUrlList))
        return cloneUrlList(currentUrlList), ""
    }

    diff := CompareUrlLists(previousUrlList, currentUrlList)
    out := PrintUrlDiff(diff, currentUrlList, false)

    return cloneUrlList(currentUrlList), out
}

