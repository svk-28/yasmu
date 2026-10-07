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
    "yasmu/config"
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
        if b.Suspicious != a.Suspicious {
            changes = append(changes, FieldChange{
                "suspicious",
                fmt.Sprint(b.Suspicious),
                fmt.Sprint(a.Suspicious),
            })
        }
        
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
            fmt.Fprintf(&sb, "   PID %d%s\n", pid, name)
        }
    }

    if len(d.OnlyInActual) > 0 {
        fmt.Fprintf(&sb, "\nNew processes (%d):\n", len(d.OnlyInActual))
        for _, pid := range d.OnlyInActual {
            meta, ok := actual[pid]
            name := ""
            if ok && meta.Name != "" {
                name = " (" + meta.Name + ", " + meta.Exe + ")"
                if meta.Owner != "" {
                    name += ", owner=" + meta.Owner
                }
            }
            fmt.Fprintf(&sb, "   PID %d%s\n", pid, name)
    
            // Предупреждение о подозрительном процессе — один раз при обнаружении.
            if ok && meta.Suspicious {
                fmt.Fprintf(&sb, "       !!! WARNING: suspicious process detected\n")
                if meta.Deleted {
                    fmt.Fprintf(&sb, "       !!! reason: executable file has been deleted\n")
                } else if meta.Exe == "" {
                    fmt.Fprintf(&sb, "       !!! reason: executable path is not available\n")
                }
            }
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
            fmt.Fprintf(&sb, "   PID %d%s\n", pd.PID, name)
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

// isKernelThread определяет, является ли процесс kernel thread.
//
// Логика:
//  1. Если CheckPPID && (ppid == 0 || ppid == 2) — да.
//  2. Если name == "" — да.
//  3. Если Exe доступен (exeErr == nil) — НЕТ (kernel threads не имеют Exe).
//  4. Если name начинается с одного из Prefixes — да.
//  5. Иначе — нет.
func isKernelThread(name string, ppid int32, exeErr error, cfg config.KernelThreadsConfig) bool {
	if cfg.CheckPPID && (ppid == 0 || ppid == 2) {
		return true
	}
	if name == "" {
		return true
	}
	if exeErr == nil {
		return false
	}
	for _, p := range cfg.Prefixes {
		p = strings.TrimSpace(p)
		if p != "" && strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func GetProcessesList(cfg config.ProcessCheck) (ProcessList, []error) {
	result := make(ProcessList)
	var errs []error

	procs, err := process.Processes()
	if err != nil {
		return nil, []error{fmt.Errorf("list processes: %w", err)}
	}

	for _, p := range procs {
		name, _ := p.Name()

		var ppid int32
		if v, err := p.Ppid(); err == nil {
			ppid = v
		}

		rawExe, exeErr := p.Exe()

		if isKernelThread(name, ppid, exeErr, cfg.KernelThreads) {
			continue
		}

		meta := ProcessMeta{
			PID:  p.Pid,
			PPID: ppid,
			Name: name,
		}

		if rawExe != "" {
			meta.Exe = strings.TrimSuffix(rawExe, " (deleted)")
			meta.Deleted = strings.Contains(rawExe, "(deleted)")
            if meta.Deleted {
                meta.Suspicious = true   // удалённый бинарник — тоже подозрительно
            }
		} else if name != "" {
			// Пустой Exe у НЕ-kernel-thread — подозрительно.
			meta.Suspicious = true
		}

		if user, err := p.Username(); err == nil {
			meta.Owner = user
		}

		if isProcessExcluded(meta.Name, cfg.Exclude) {
			continue
		}

		result[p.Pid] = meta
	}

	return result, errs
}

func ProcessesCheck(cfg config.ProcessCheck, previousProcessList ProcessList) (ProcessList, string) {
    log.Println("Running checklibs/ProcessesCheck")

    currentProcessList, errs := GetProcessesList(cfg)
    for _, e := range errs {
        log.Printf("warn: %v", e)
    }

    if currentProcessList == nil {
        return previousProcessList, ""
    }

    if previousProcessList == nil {
        log.Printf("Baseline collected: %d processes", len(currentProcessList))

        var outString strings.Builder
        var suspicious []int32
        for pid, meta := range currentProcessList {
            if meta.Suspicious {
                suspicious = append(suspicious, pid)
            }
        }
        if len(suspicious) > 0 {
            sort.Slice(suspicious, func(i, j int) bool { return suspicious[i] < suspicious[j] })
            fmt.Fprintf(&outString, "\nSuspicious processes detected (%d):\n", len(suspicious))
            for _, pid := range suspicious {
                meta := currentProcessList[pid]
                fmt.Fprintf(&outString, "   PID %d (%s, %s), owner=%s\n",
                    pid, meta.Name, meta.Exe, meta.Owner)
                if meta.Deleted {
                    fmt.Fprintf(&outString, "       !!! WARNING: executable file has been deleted\n")
                } else if meta.Exe == "" {
                    fmt.Fprintf(&outString, "       !!! WARNING: executable path is not available\n")
                }
            }
        }

        return cloneProcessList(currentProcessList), outString.String()
    }

    diff := CompareProcessLists(previousProcessList, currentProcessList)
    out := PrintProcessesDiff(diff, previousProcessList, currentProcessList, false)

    return cloneProcessList(currentProcessList), out
}

