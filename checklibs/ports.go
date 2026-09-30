// *******************************************************
// YASMU - Yet Another System Monitoring Utility
// The utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
// Home page: https://nuk-svk.ru
// License: GPL v3
// *******************************************************
// Listening ports (sockets) monitoring module.
// Collects TCP listeners and UDP sockets via gopsutil/net, compares
// with the previous snapshot to detect changes.
// *******************************************************

package checklibs

import (
    "fmt"
    "log"
    "sort"
    "strings"

    gopsnet "github.com/shirou/gopsutil/v4/net"
    gopsproc "github.com/shirou/gopsutil/v4/process"
)

type PortMeta struct {
    Proto      string // "tcp" / "udp"
    LocalIP    string
    LocalPort  uint32
    RemoteIP   string
    RemotePort uint32
    State      string
    PID        int32
    Exe        string
}

type PortList map[string]PortMeta

type PortsOptions struct {
    ExcludePorts     []uint32
    ExcludeProcesses []string
    IgnorePIDChanges bool
}

type PortDiff struct {
    Key     string
    Changes []FieldChange
}

type PortDiffResult struct {
    OnlyInBase   []string
    OnlyInActual []string
    Changed      []PortDiff
    Unchanged    []string
}

// portKey формирует ключ для сокета.
// Включает PID, чтобы различать сокеты на одном порту от разных процессов
// (SO_REUSEPORT). При IgnorePIDChanges логика сравнения отдельная —
// см. ComparePortLists.
func portKey(m PortMeta) string {
    return fmt.Sprintf("%s:%s:%d:%s:%d:%d",
        m.Proto, m.LocalIP, m.LocalPort, m.RemoteIP, m.RemotePort, m.PID)
}

// portKeyNoPID — ключ без PID. Используется, когда IgnorePIDChanges = true,
// чтобы перезапуски процессов не выглядели как «закрытие/открытие» сокета.
func portKeyNoPID(m PortMeta) string {
    return fmt.Sprintf("%s:%s:%d:%s:%d",
        m.Proto, m.LocalIP, m.LocalPort, m.RemoteIP, m.RemotePort)
}

// Если opts.IgnorePIDChanges = true:
//   - ключ не содержит PID → перезапуск процесса не считается изменением;
//   - поля PID и Exe не сравниваются.
func ComparePortLists(base, actual PortList, opts PortsOptions) PortDiffResult {
    var res PortDiffResult

    if opts.IgnorePIDChanges {
        // Пересобираем карты с ключом без PID.
        base = rekeyWithoutPID(base)
        actual = rekeyWithoutPID(actual)
    }

    for key, b := range base {
        a, ok := actual[key]
        if !ok {
            res.OnlyInBase = append(res.OnlyInBase, key)
            continue
        }

        var changes []FieldChange

        if b.State != a.State {
            changes = append(changes, FieldChange{"state", b.State, a.State})
        }

        if !opts.IgnorePIDChanges {
            if b.PID != a.PID {
                changes = append(changes, FieldChange{
                    "pid",
                    fmt.Sprint(b.PID),
                    fmt.Sprint(a.PID),
                })
            }
            if b.Exe != a.Exe {
                changes = append(changes, FieldChange{"exe", b.Exe, a.Exe})
            }
        }

        if len(changes) > 0 {
            res.Changed = append(res.Changed, PortDiff{Key: key, Changes: changes})
        } else {
            res.Unchanged = append(res.Unchanged, key)
        }
    }

    for key := range actual {
        if _, ok := base[key]; !ok {
            res.OnlyInActual = append(res.OnlyInActual, key)
        }
    }

    return res
}

// rekeyWithoutPID пересобирает карту, используя ключ без PID.
// Если два сокета совпадают по всему, кроме PID (SO_REUSEPORT),
// они схлопнутся — оставляем первый.
func rekeyWithoutPID(src PortList) PortList {
    dst := make(PortList, len(src))
    for _, m := range src {
        dst[portKeyNoPID(m)] = m
    }
    return dst
}

func PrintPortsDiff(d PortDiffResult, base, actual PortList, showUnchanged bool) string {
    var sb strings.Builder

    sort.Strings(d.OnlyInBase)
    sort.Strings(d.OnlyInActual)

    if len(d.OnlyInBase) > 0 {
        fmt.Fprintf(&sb, "\nClosed sockets (%d):\n", len(d.OnlyInBase))
        for _, key := range d.OnlyInBase {
            m := base[key]
            fmt.Fprintf(&sb, "   - %s\n", formatPortMeta(m))
        }
    }

    if len(d.OnlyInActual) > 0 {
        fmt.Fprintf(&sb, "\nOpened sockets (%d):\n", len(d.OnlyInActual))
        for _, key := range d.OnlyInActual {
            m := actual[key]
            fmt.Fprintf(&sb, "   + %s\n", formatPortMeta(m))
        }
    }

    if len(d.Changed) > 0 {
        fmt.Fprintf(&sb, "\nChanged sockets (%d):\n", len(d.Changed))
        sort.Slice(d.Changed, func(i, j int) bool {
            return d.Changed[i].Key < d.Changed[j].Key
        })
        for _, pd := range d.Changed {
            m := actual[pd.Key]
            fmt.Fprintf(&sb, "   ~ %s\n", formatPortMeta(m))
            for _, ch := range pd.Changes {
                fmt.Fprintf(&sb, "       %-7s: %q --> %q\n", ch.Field, ch.Old, ch.New)
            }
        }
    }

    if showUnchanged && len(d.Unchanged) > 0 {
        fmt.Fprintf(&sb, "\nUnchanged sockets (%d):\n", len(d.Unchanged))
        sort.Strings(d.Unchanged)
        for _, key := range d.Unchanged {
            m := actual[key]
            fmt.Fprintf(&sb, "   = %s\n", formatPortMeta(m))
        }
    }

    fmt.Fprintf(&sb, "\nSummary: closed %d, opened %d, changed %d, all %d\n",
        len(d.OnlyInBase), len(d.OnlyInActual),
        len(d.Changed), len(d.Unchanged))

    if len(d.OnlyInBase) > 0 || len(d.OnlyInActual) > 0 || len(d.Changed) > 0 {
        return sb.String()
    }
    return ""
}

func formatPortMeta(m PortMeta) string {
    var sb strings.Builder

    fmt.Fprintf(&sb, "%s %s:%d", m.Proto, m.LocalIP, m.LocalPort)
    if m.RemoteIP != "" && m.RemotePort > 0 {
        fmt.Fprintf(&sb, " -> %s:%d", m.RemoteIP, m.RemotePort)
    }
    if m.State != "" {
        fmt.Fprintf(&sb, " [%s]", m.State)
    }
    if m.Exe != "" {
        fmt.Fprintf(&sb, " [%s, pid=%d]", m.Exe, m.PID)
    } else if m.PID > 0 {
        fmt.Fprintf(&sb, " [pid=%d]", m.PID)
    }
    return sb.String()
}

func isUDPListener(c gopsnet.ConnectionStat) bool {
    if c.Pid <= 0 {
        return false
    }
    if c.Raddr.IP != "" && c.Raddr.Port > 0 {
        return false
    }
    if c.Laddr.Port >= 32768 {
        return false
    }
    return true
}

func connToMeta(proto string, c gopsnet.ConnectionStat) PortMeta {
    meta := PortMeta{
        Proto:      proto,
        LocalIP:    c.Laddr.IP,
        LocalPort:  c.Laddr.Port,
        RemoteIP:   c.Raddr.IP,
        RemotePort: c.Raddr.Port,
        PID:        c.Pid,
    }
    if meta.LocalIP == "" {
        meta.LocalIP = "0.0.0.0"
    }
    meta.State = c.Status
    if meta.State == "NONE" {
        meta.State = ""
    }
    return meta
}

func clonePortList(src PortList) PortList {
    dst := make(PortList, len(src))
    for k, v := range src {
        dst[k] = v
    }
    return dst
}

func isExcludedProc(exe string, opts PortsOptions) bool {
    if len(opts.ExcludeProcesses) == 0 {
        return false
    }
    lower := strings.ToLower(exe)
    for _, ex := range opts.ExcludeProcesses {
        ex = strings.TrimSpace(ex)
        if ex == "" {
            continue
        }
        if strings.Contains(lower, strings.ToLower(ex)) {
            return true
        }
    }
    return false
}


func GetPortsList(opts PortsOptions) (PortList, []error) {
    result := make(PortList)
    var errs []error

    exeCache := make(map[int32]string)
    resolveExe := func(pid int32) string {
        if pid <= 0 {
            return ""
        }
        if exe, ok := exeCache[pid]; ok {
            return exe
        }
        exe := ""
        if p, err := gopsproc.NewProcess(pid); err == nil {
            if path, err := p.Exe(); err == nil {
                exe = path
            }
        }
        exeCache[pid] = exe
        return exe
    }

    skipPort := make(map[uint32]bool, len(opts.ExcludePorts))
    for _, p := range opts.ExcludePorts {
        skipPort[p] = true
    }

    // TCP — LISTEN only.
    tcpConns, err := gopsnet.Connections("tcp")
    if err != nil {
        errs = append(errs, fmt.Errorf("tcp: %w", err))
    } else {
        for _, c := range tcpConns {
            if c.Status != "LISTEN" {
                continue
            }
            if skipPort[c.Laddr.Port] {
                continue
            }
            meta := connToMeta("tcp", c)
            meta.Exe = resolveExe(meta.PID)
            if isExcludedProc(meta.Exe, opts) {
                continue
            }
            result[portKey(meta)] = meta
        }
    }

    // UDP — bind only.
    udpConns, err := gopsnet.Connections("udp")
    if err != nil {
        errs = append(errs, fmt.Errorf("udp: %w", err))
    } else {
        for _, c := range udpConns {
            if !isUDPListener(c) {
                continue
            }
            if skipPort[c.Laddr.Port] {
                continue
            }
            meta := connToMeta("udp", c)
            meta.Exe = resolveExe(meta.PID)
            if isExcludedProc(meta.Exe, opts) {
                continue
            }
            result[portKey(meta)] = meta
        }
    }

    return result, errs
}

func PortsCheck(opts PortsOptions, previousPortList PortList) (PortList, string) {
    log.Println("Running checklibs/PortsCheck")

    currentPortList, errs := GetPortsList(opts)
    for _, e := range errs {
        log.Printf("warn: %v", e)
    }

    if currentPortList == nil {
        return previousPortList, ""
    }

    if previousPortList == nil {
        log.Printf("Baseline collected: %d sockets", len(currentPortList))
        return clonePortList(currentPortList), ""
    }

    diff := ComparePortLists(previousPortList, currentPortList, opts)
    out := PrintPortsDiff(diff, previousPortList, currentPortList, false)

    return clonePortList(currentPortList), out
}

