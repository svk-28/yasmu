// *******************************************************
// YASMU - Yet Another System Monitoring Utility
// The utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
// Home page: https://nuk-svk.ru
// License: GPL v3
// *******************************************************
// Network connections monitoring module (rule-based).
//
// Unlike ports/processes/files, this module does NOT compare snapshots.
// Instead, it evaluates a set of rules against the current connections
// and reports violations. This avoids the constant noise of diffing
// short-lived TCP sessions while still catching suspicious activity.
// *******************************************************

package checklibs

import (
    "fmt"
    "log"
    "net"
    "sort"
    "strings"

    gopsnet "github.com/shirou/gopsutil/v4/net"
    gopsproc "github.com/shirou/gopsutil/v4/process"
)

// ConnMeta — метаданные одного соединения.
type ConnMeta struct {
    Proto      string
    LocalIP    string
    LocalPort  uint32
    RemoteIP   string
    RemotePort uint32
    State      string
    PID        int32
    Exe        string
    Process    string
}

type ConnList []ConnMeta

// ConnRule — правило проверки соединений.
//
// Все поля необязательны. Правило срабатывает, если соединение
// удовлетворяет ВСЕМ заданным условиям.
type ConnRule struct {
    Name          string
    Proto         string
    Port          uint32
    Processes     []string
    State         string
    AllowedRemote []string
    AlertOnMatch  bool
}

// ConnViolation — одно сработавшее правило.
type ConnViolation struct {
    RuleName string
    Conn     ConnMeta
    Reason   string
}

// CheckConnRules прогоняет все правила по всем соединениям и возвращает
// список нарушений. Порядок нарушений — сначала по имени правила,
// потом по процессу/PID — для стабильного вывода.
func CheckConnRules(conns ConnList, rules []ConnRule) []ConnViolation {
    var violations []ConnViolation

    for _, rule := range rules {
        for _, c := range conns {
            if !connMatchesRule(c, rule) {
                continue
            }
            // Если AlertOnMatch — совпадение и есть нарушение.
            if rule.AlertOnMatch {
                violations = append(violations, ConnViolation{
                    RuleName: rule.Name,
                    Conn:     c,
                    Reason:   "matched forbidden pattern",
                })
                continue
            }
            // Иначе правило описывает «разрешённый» трафик:
            // проверяем, что remote в AllowedRemote.
            if len(rule.AllowedRemote) > 0 && !remoteIsAllowed(c.RemoteIP, rule.AllowedRemote) {
                violations = append(violations, ConnViolation{
                    RuleName: rule.Name,
                    Conn:     c,
                    Reason:   fmt.Sprintf("remote %s not in allowed list", c.RemoteIP),
                })
            }
        }
    }

    sort.Slice(violations, func(i, j int) bool {
        if violations[i].RuleName != violations[j].RuleName {
            return violations[i].RuleName < violations[j].RuleName
        }
        if violations[i].Conn.Process != violations[j].Conn.Process {
            return violations[i].Conn.Process < violations[j].Conn.Process
        }
        return violations[i].Conn.PID < violations[j].Conn.PID
    })

    return violations
}

// connMatchesRule проверяет, попадает ли соединение под условия правила.
// Пустое поле = «любое значение».
func connMatchesRule(c ConnMeta, r ConnRule) bool {
    if r.Proto != "" && !strings.EqualFold(r.Proto, c.Proto) {
        return false
    }
    if r.Port != 0 && r.Port != c.LocalPort {
        return false
    }
    if r.State != "" && !strings.EqualFold(r.State, c.State) {
        return false
    }

    // Точное совпадение basename с одним из процессов в списке.
    if len(r.Processes) > 0 {
        procName := strings.ToLower(baseName(c.Exe))
        if procName == "" {
            procName = strings.ToLower(c.Process)
        }
        matched := false
        for _, p := range r.Processes {
            if procName == strings.ToLower(strings.TrimSpace(p)) {
                matched = true
                break
            }
        }
        if !matched {
            return false
        }
    }
    return true
}

// remoteIsAllowed проверяет, входит ли IP в один из CIDR/адресов списка.
// Поддерживает как «1.2.3.4», так и «10.0.0.0/8».
func remoteIsAllowed(remoteIP string, allowed []string) bool {
    if remoteIP == "" {
        return true // нет remote — нечего проверять
    }
    ip := net.ParseIP(remoteIP)
    if ip == nil {
        return false
    }
    for _, entry := range allowed {
        entry = strings.TrimSpace(entry)
        if entry == "" {
            continue
        }
        if strings.Contains(entry, "/") {
            _, cidr, err := net.ParseCIDR(entry)
            if err == nil && cidr.Contains(ip) {
                return true
            }
            continue
        }
        if other := net.ParseIP(entry); other != nil && other.Equal(ip) {
            return true
        }
    }
    return false
}

func PrintConnViolations(violations []ConnViolation) string {
    if len(violations) == 0 {
        return ""
    }

    var sb strings.Builder
    fmt.Fprintf(&sb, "\nConnection rule violations (%d):\n", len(violations))

    for _, v := range violations {
        fmt.Fprintf(&sb, "   ! [%s] %s\n", v.RuleName, v.Reason)
        fmt.Fprintf(&sb, "       %s\n", formatConnMeta(v.Conn))
    }

    return sb.String()
}

func formatConnMeta(c ConnMeta) string {
    var sb strings.Builder

    fmt.Fprintf(&sb, "%s %s:%d", c.Proto, c.LocalIP, c.LocalPort)
    if c.RemoteIP != "" && c.RemotePort > 0 {
        fmt.Fprintf(&sb, " -> %s:%d", c.RemoteIP, c.RemotePort)
    }
    if c.State != "" {
        fmt.Fprintf(&sb, " [%s]", c.State)
    }
    if c.Exe != "" {
        fmt.Fprintf(&sb, " [%s, pid=%d]", c.Exe, c.PID)
    } else if c.PID > 0 {
        fmt.Fprintf(&sb, " [pid=%d]", c.PID)
    }

    return sb.String()
}

func baseName(path string) string {
    if path == "" {
        return ""
    }
    idx := strings.LastIndex(path, "/")
    if idx < 0 {
        return path
    }
    return path[idx+1:]
}


// GetConnectionsList собирает все TCP/UDP-соединения через gopsutil/net.
// Возвращает срез и ошибки по каждому протоколу отдельно.
func GetConnectionsList() (ConnList, []error) {
    var result ConnList
    var errs []error

    exeCache := make(map[int32]string)
    resolveExe := func(pid int32) (string, string) {
        if pid <= 0 {
            return "", ""
        }
        if cached, ok := exeCache[pid]; ok {
            return cached, baseName(cached)
        }
        exe := ""
        if p, err := gopsproc.NewProcess(pid); err == nil {
            if path, err := p.Exe(); err == nil {
                exe = path
            }
        }
        exeCache[pid] = exe
        return exe, baseName(exe)
    }

    // TCP
    tcpConns, err := gopsnet.Connections("tcp")
    if err != nil {
        errs = append(errs, fmt.Errorf("tcp: %w", err))
    } else {
        for _, c := range tcpConns {
            exe, proc := resolveExe(c.Pid)
            result = append(result, ConnMeta{
                Proto:      "tcp",
                LocalIP:    c.Laddr.IP,
                LocalPort:  c.Laddr.Port,
                RemoteIP:   c.Raddr.IP,
                RemotePort: c.Raddr.Port,
                State:      c.Status,
                PID:        c.Pid,
                Exe:        exe,
                Process:    proc,
            })
        }
    }

    // UDP
    udpConns, err := gopsnet.Connections("udp")
    if err != nil {
        errs = append(errs, fmt.Errorf("udp: %w", err))
    } else {
        for _, c := range udpConns {
            exe, proc := resolveExe(c.Pid)
            state := c.Status
            if state == "NONE" {
                state = ""
            }
            result = append(result, ConnMeta{
                Proto:      "udp",
                LocalIP:    c.Laddr.IP,
                LocalPort:  c.Laddr.Port,
                RemoteIP:   c.Raddr.IP,
                RemotePort: c.Raddr.Port,
                State:      state,
                PID:        c.Pid,
                Exe:        exe,
                Process:    proc,
            })
        }
    }
    // log.Printf("[debug] total: %d", len(result))
    // for _, c := range result {
        // if strings.Contains(strings.ToLower(c.Exe), "bash") {
            // log.Printf("[debug] bash: %d -> %s:%d state=%s pid=%d exe=%s",
                // c.LocalPort, c.RemoteIP, c.RemotePort, c.State, c.PID, c.Exe)
        // }
    // }
    return result, errs
}


func ConnectionsCheck(rules []ConnRule) string {
    log.Println("Running checklibs/ConnectionsCheck")

    if len(rules) == 0 {
        log.Println("No connection rules configured, skipping")
        return ""
    }

    conns, errs := GetConnectionsList()
    for _, e := range errs {
        log.Printf("warn: %v", e)
    }

    violations := CheckConnRules(conns, rules)
    if len(violations) == 0 {
        log.Printf("No connection rule violations (%d connections checked)", len(conns))
        return ""
    }

    log.Printf("Connection rule violations: %d", len(violations))
    return PrintConnViolations(violations)
}
