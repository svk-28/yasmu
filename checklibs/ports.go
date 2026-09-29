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

// PortMeta — метаданные одного сокета.
type PortMeta struct {
    Proto      string // "tcp" / "udp"
    LocalIP    string // локальный IP (0.0.0.0, ::, 127.0.0.1, ...)
    LocalPort  uint32 // локальный порт
    RemoteIP   string // удалённый IP (для TCP с установленным соединением)
    RemotePort uint32 // удалённый порт
    State      string // "LISTEN", "ESTABLISHED", ... (для UDP — пусто)
    PID        int32  // PID процесса-владельца
    Exe        string // путь к бинарнику процесса-владельца
}

// PortList — карта: ключ сокета → метаданные.
type PortList map[string]PortMeta

// -----------------------------------------------------------------------------
// Diff-типы
// -----------------------------------------------------------------------------

// PortDiff — изменения одного сокета.
type PortDiff struct {
    Key     string
    Changes []FieldChange
}

// PortDiffResult — результат сравнения двух снимков PortList.
type PortDiffResult struct {
    OnlyInBase   []string   // сокет закрылся
    OnlyInActual []string   // сокет открылся
    Changed      []PortDiff // изменилось состояние/владелец
    Unchanged    []string
}

// -----------------------------------------------------------------------------
// Ключ сокета
// -----------------------------------------------------------------------------

// portKey формирует стабильный ключ для сокета.
// Включает PID, чтобы различать сокеты на одном порту от разных процессов
// (SO_REUSEPORT, несколько bind'ов на 0.0.0.0 и 127.0.0.1 и т.п.).
func portKey(m PortMeta) string {
    return fmt.Sprintf("%s:%s:%d:%s:%d:%d",
        m.Proto, m.LocalIP, m.LocalPort, m.RemoteIP, m.RemotePort, m.PID)
}

// -----------------------------------------------------------------------------
// Сравнение
// -----------------------------------------------------------------------------

// ComparePortLists сравнивает два снимка PortList.
func ComparePortLists(base, actual PortList) PortDiffResult {
    var res PortDiffResult

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

// -----------------------------------------------------------------------------
// Вывод
// -----------------------------------------------------------------------------

// PrintPortsDiff формирует текстовый отчёт об изменениях в сокетах.
// Возвращает пустую строку, если изменений нет.
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

// formatPortMeta — человекочитаемое представление сокета для отчёта.
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

// -----------------------------------------------------------------------------
// Основная логика — аналог FilesCheck / UsersCheck / ProcessesCheck / ServicesCheck / WebCheck
// -----------------------------------------------------------------------------

// PortsCheck собирает текущее состояние сокетов и сравнивает с предыдущим снимком.
//
// previousPortList == nil на первом проходе — тогда diff не формируется,
// возвращается пустая строка отчёта.
//
// Возвращает новый снимок (для следующего вызова) и текст отчёта.
func PortsCheck(previousPortList PortList) (PortList, string) {
    log.Println("\nRunning checklibs/PortsCheck")

    currentPortList, errs := GetPortsList()
    for _, e := range errs {
        log.Printf("warn: %v", e)
    }

    if currentPortList == nil {
        return previousPortList, ""
    }

    // Первый проход — только эталон.
    if previousPortList == nil {
        log.Printf("Baseline collected: %d sockets", len(currentPortList))
        return clonePortList(currentPortList), ""
    }

    diff := ComparePortLists(previousPortList, currentPortList)
    out := PrintPortsDiff(diff, previousPortList, currentPortList, false)

    return clonePortList(currentPortList), out
}

// GetPortsList собирает TCP-листенеры и UDP-сокеты через gopsutil/net.
//
// TCP: только LISTEN.
// UDP: только "bind'нутые" сокеты — с известным PID и на порт >= 1024,
// исключая эфемерные клиентские сокеты, которые постоянно меняются.
//
// Ошибки на отдельных протоколах не прерывают сбор — они собираются в errs.
func GetPortsList() (PortList, []error) {
    result := make(PortList)
    var errs []error

    // Кэш PID → Exe, чтобы не дёргать /proc на каждый сокет.
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

    // 1. TCP — только LISTEN.
    tcpConns, err := gopsnet.Connections("tcp")
    if err != nil {
        errs = append(errs, fmt.Errorf("tcp: %w", err))
    } else {
        for _, c := range tcpConns {
            if c.Status != "LISTEN" {
                continue
            }
            meta := connToMeta("tcp", c)
            meta.Exe = resolveExe(meta.PID)
            result[portKey(meta)] = meta
        }
    }

    // 2. UDP — только "bind'нутые" сокеты.
    udpConns, err := gopsnet.Connections("udp")
    if err != nil {
        errs = append(errs, fmt.Errorf("udp: %w", err))
    } else {
        for _, c := range udpConns {
            if !isUDPListener(c) {
                continue
            }
            meta := connToMeta("udp", c)
            meta.Exe = resolveExe(meta.PID)
            result[portKey(meta)] = meta
        }
    }

    return result, errs
}

// isUDPListener решает, считать ли UDP-сокет «слушающим».
//
// UDP не имеет состояния LISTEN, поэтому используем эвристику:
//   - у сокета известен PID (значит, процесс жив и владеет сокетом);
//   - локальный порт в «привилегированном» или серверном диапазоне (>= 1024);
//   - удалённый адрес пуст (сокет не подключён к пиру).
//
// Это отсекает эфемерные клиентские UDP-сокеты, которые создаются
// на каждый DNS/NTP/QUIC-запрос и сразу закрываются.
func isUDPListener(c gopsnet.ConnectionStat) bool {
    // Нет PID — не можем подтвердить, что это серверный сокет.
    if c.Pid <= 0 {
        return false
    }

    // У «подключённого» UDP-сокета есть удалённый адрес — это клиент.
    if c.Raddr.IP != "" && c.Raddr.Port > 0 {
        return false
    }

    // Эфемерные порты (обычно 32768–60999) — клиентские.
    // Серверные сокеты bind'ятся на фиксированные порты.
    // Диапазон можно уточнить через /proc/sys/net/ipv4/ip_local_port_range,
    // но для фильтрации достаточно отсечь всё, что >= 32768.
    if c.Laddr.Port >= 32768 {
        return false
    }

    return true
}

// connToMeta превращает запись gopsutil/net в PortMeta.
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

    // Состояние (для TCP). Для UDP gopsutil возвращает "NONE".
    meta.State = c.Status
    if meta.State == "NONE" {
        meta.State = ""
    }

    return meta
}

// clonePortList — копия PortList.
// PortMeta не содержит срезов и указателей, поэтому поверхностной копии достаточно.
func clonePortList(src PortList) PortList {
    dst := make(PortList, len(src))
    for k, v := range src {
        dst[k] = v
    }
    return dst
}
