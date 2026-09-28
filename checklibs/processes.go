// *******************************************************
// YASMU - Yet Another System Monitoring Utility
// The utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
// Home page: https://nuk-svk.ru
// License: GPL v3
// *******************************************************
// Process monitoring module.
// Collects running processes and compares with the
// previous snapshot to detect changes.
// ********************************************************************

package checklibs

import (
    "fmt"
    "log"
    "sort"
    "strings"

    "github.com/shirou/gopsutil/v4/process"
    // For bsd systems
    // "github.com/tklauser/ps"
)

type ProcessMeta struct {
    PID        int32  // process ID
    PPID       int32  // parent process id
    Name       string // process name
    Owner      string // process owner
    Exe        string // path to executable file
    Deleted    bool   // process with deleted exe
    Suspicious bool
}

type ProcessList map[int32]ProcessMeta

type ProcessDiff struct {
    PID     int32
    Changes []FieldChange
}

type ProcessDiffResult struct {
    OnlyInBase   []int32
    OnlyInActual []int32
    Changed      []ProcessDiff
    Unchanged    []int32
}

func CompareProcessLists(base, actual ProcessList) ProcessDiffResult {
    var res ProcessDiffResult

    for pid, b := range base {
        a, ok := actual[pid]
        if !ok {
            res.OnlyInBase = append(res.OnlyInBase, pid)
            continue
        }

        var changes []FieldChange

        if b.PPID != a.PPID {
            changes = append(changes, FieldChange{
                "ppid",
                fmt.Sprint(b.PPID),
                fmt.Sprint(a.PPID),
            })
        }
        if b.Name != a.Name {
            changes = append(changes, FieldChange{"name", b.Name, a.Name})
        }
        if b.Owner != a.Owner {
            changes = append(changes, FieldChange{"owner", b.Owner, a.Owner})
        }
        if b.Exe != a.Exe {
            changes = append(changes, FieldChange{"exe", b.Exe, a.Exe})
        }

        if b.Deleted != a.Deleted {
            changes = append(changes, FieldChange{
                "deleted",
                fmt.Sprint(b.Deleted),
                fmt.Sprint(a.Deleted),
            })
        }
        // if b.Suspicious != a.Suspicious {
            // changes = append(changes, FieldChange{
                // "suspicious",
                // fmt.Sprint(b.Suspicious),
                // fmt.Sprint(a.Suspicious),
            // })
        // }
        
        if len(changes) > 0 {
            res.Changed = append(res.Changed, ProcessDiff{PID: pid, Changes: changes})
        } else {
            res.Unchanged = append(res.Unchanged, pid)
        }
    }

    for pid := range actual {
        if _, ok := base[pid]; !ok {
            res.OnlyInActual = append(res.OnlyInActual, pid)
        }
    }

    return res
}

// PrintProcessesDiff generates a text report on changes in processes.
// Returns an empty string if there are no changes.
// For terminated processes, it attempts to extract the name from base — 
// so that the report contains not just “PID 1234”, but “PID 1234 (nginx)”.
// base — the previous snapshot (for the names of stopped processes).
// actual — the current snapshot (for the names of new and modified processes).
func PrintProcessesDiff(d ProcessDiffResult, base, actual ProcessList, showUnchanged bool) string {
    var sb strings.Builder

    sort.Slice(d.OnlyInBase, func(i, j int) bool { return d.OnlyInBase[i] < d.OnlyInBase[j] })
    sort.Slice(d.OnlyInActual, func(i, j int) bool { return d.OnlyInActual[i] < d.OnlyInActual[j] })

    if len(d.OnlyInBase) > 0 {
        fmt.Fprintf(&sb, "\nStopped processes (%d):\n", len(d.OnlyInBase))
        for _, pid := range d.OnlyInBase {
            name := ""
            if meta, ok := base[pid]; ok && meta.Name != "" {
                name = " (" + meta.Name + ", " + meta.Exe + ")"
            }
            fmt.Fprintf(&sb, "   - PID %d%s\n", pid, name)
        }
    }

    if len(d.OnlyInActual) > 0 {
        fmt.Fprintf(&sb, "\nNew processes (%d):\n", len(d.OnlyInActual))
        for _, pid := range d.OnlyInActual {
            name := ""
            if meta, ok := actual[pid]; ok && meta.Name != "" {
                name = " (" + meta.Name + ", " + meta.Exe + ")"
                if meta.Owner != "" {
                    name += ", owner=" + meta.Owner
                }
            }
            fmt.Fprintf(&sb, "   + PID %d%s\n", pid, name)
        }
    }

    if len(d.Changed) > 0 {
        fmt.Fprintf(&sb, "\nChanged processes (%d):\n", len(d.Changed))
        sort.Slice(d.Changed, func(i, j int) bool {
            return d.Changed[i].PID < d.Changed[j].PID
        })
        for _, pd := range d.Changed {
            name := ""
            if meta, ok := actual[pd.PID]; ok && meta.Name != "" {
                name = " (" + meta.Name + ", " + meta.Exe + ")"
            }
            fmt.Fprintf(&sb, "   ~ PID %d%s\n", pd.PID, name)
            for _, ch := range pd.Changes {
                fmt.Fprintf(&sb, "       %-6s: %q --> %q\n", ch.Field, ch.Old, ch.New)
            }
        }
    }

    if showUnchanged && len(d.Unchanged) > 0 {
        fmt.Fprintf(&sb, "\nUnchanged processes (%d):\n", len(d.Unchanged))
        sort.Slice(d.Unchanged, func(i, j int) bool { return d.Unchanged[i] < d.Unchanged[j] })
        for _, pid := range d.Unchanged {
            name := ""
            if meta, ok := actual[pid]; ok && meta.Name != "" {
                name = " (" + meta.Name + ")"
            }
            fmt.Fprintf(&sb, "   = PID %d%s\n", pid, name)
        }
    }

    fmt.Fprintf(&sb, "\nSummary: stopped %d, started %d, changed %d, all %d\n",
        len(d.OnlyInBase), len(d.OnlyInActual),
        len(d.Changed), len(d.Unchanged))

    if len(d.OnlyInBase) > 0 || len(d.OnlyInActual) > 0 || len(d.Changed) > 0 {
        return sb.String()
    }
    return ""
}

func cloneProcessList(src ProcessList) ProcessList {
    dst := make(ProcessList, len(src))
    for k, v := range src {
        dst[k] = v
    }
    return dst
}

func isProcessExcluded(name string, exclude []string) bool {
    if len(exclude) == 0 {
        return false
    }
    lower := strings.ToLower(name)
    for _, ex := range exclude {
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

func isLinuxKernelThread(name string) bool {
    if name == "" {
        return true
    }
    prefixes := []string{
        "kworker",        // workqueue workers
        "ksoftirqd",      // softirq daemon
        "kthreadd",       // kernel thread daemon
        "migration",      // CPU migration
        "rcu_",           // RCU callbacks
        "kcompactd",      // memory compaction
        "kswapd",         // swap daemon
        "khugepaged",     // huge pages
        "cpuhp/",         // CPU hotplug
        "idle_inject/",   // idle injection
        "scsi_",          // SCSI error handler
        "irq/",           // IRQ threads
        "kdevtmpfs",      // devtmpfs
        "netns",          // network namespaces
        "kauditd",        // audit daemon
        "kblockd",        // block device workers
        "kintegrityd",    // integrity
        "kdmflush",       // device-mapper
        "kaluad",         // ALUA
        "kmpath_",        // multipath
    }
    for _, p := range prefixes {
        if strings.HasPrefix(name, p) {
            return true
        }
    }
    return false
}

func GetProcessesList(exclude []string) (ProcessList, []error) {
    result := make(ProcessList)
    var errs []error

    procs, err := process.Processes()
    if err != nil {
        return nil, []error{fmt.Errorf("list processes: %w", err)}
    }

    for _, p := range procs {
        rawExe, exeErr := p.Exe()
        name, _ := p.Name()

        if exeErr != nil && isLinuxKernelThread(name) {
            continue
        }

        meta := ProcessMeta{
            PID: p.Pid,
            Name: name,
        }

        if rawExe != "" {
            meta.Exe = strings.TrimSuffix(rawExe, " (deleted)")
            meta.Deleted = strings.Contains(rawExe, "(deleted)")
        } else if name != "" {
            // An empty EXE in a non‑kernel‑thread process is suspicious.
            meta.Suspicious = true
        }

        if ppid, err := p.Ppid(); err == nil {
            meta.PPID = ppid
        }

        if user, err := p.Username(); err == nil {
            meta.Owner = user
        }

        if isProcessExcluded(meta.Name, exclude) {
            continue
        }

        result[p.Pid] = meta
    }

    return result, errs
}

func ProcessesCheck(exclude []string, previousProcessList ProcessList) (ProcessList, string) {
    log.Println("\nRunning checklibs/ProcessesCheck")

    currentProcessList, errs := GetProcessesList(exclude)
    for _, e := range errs {
        log.Printf("warn: %v", e)
    }

    if currentProcessList == nil {
        return previousProcessList, ""
    }

    if previousProcessList == nil {
        log.Printf("Baseline collected: %d processes", len(currentProcessList))
        return cloneProcessList(currentProcessList), ""
    }

    diff := CompareProcessLists(previousProcessList, currentProcessList)
    out := PrintProcessesDiff(diff, previousProcessList, currentProcessList, false)

    return cloneProcessList(currentProcessList), out
}
