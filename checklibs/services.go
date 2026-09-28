// *******************************************************
// YASMU - Yet Another System Monitoring Utility
// The utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
// Home page: https://nuk-svk.ru
// License: GPL v3
// *******************************************************
// Service (systemd unit) monitoring module.
// Collects the state of all .service units via D-Bus and compares
// with the previous snapshot.
// ********************************************************************

package checklibs

import (
    "fmt"
    "log"
    "sort"
    "strings"

    "github.com/coreos/go-systemd/v22/dbus"
)

type ServiceMeta struct {
    Name        string // "sshd.service"
    LoadState   string // loaded / not-found / error
    ActiveState string // active / inactive / failed
    SubState    string // running / exited / failed
    MainPID     uint32 // PID of main process (0, if not running)
}

type ServiceList map[string]ServiceMeta

type ServiceDiff struct {
    Name    string
    Changes []FieldChange // reused type from files.go
}

type ServiceDiffResult struct {
    OnlyInBase   []string
    OnlyInActual []string
    Changed      []ServiceDiff
    Unchanged    []string
}

func CompareServiceLists(base, actual ServiceList) ServiceDiffResult {
    var res ServiceDiffResult

    for name, b := range base {
        a, ok := actual[name]
        if !ok {
            res.OnlyInBase = append(res.OnlyInBase, name)
            continue
        }

        var changes []FieldChange

        if b.LoadState != a.LoadState {
            changes = append(changes, FieldChange{"load", b.LoadState, a.LoadState})
        }
        if b.ActiveState != a.ActiveState {
            changes = append(changes, FieldChange{"active", b.ActiveState, a.ActiveState})
        }
        if b.SubState != a.SubState {
            changes = append(changes, FieldChange{"sub", b.SubState, a.SubState})
        }
        if b.MainPID != a.MainPID {
            changes = append(changes, FieldChange{
                "pid",
                fmt.Sprint(b.MainPID),
                fmt.Sprint(a.MainPID),
            })
        }

        if len(changes) > 0 {
            res.Changed = append(res.Changed, ServiceDiff{Name: name, Changes: changes})
        } else {
            res.Unchanged = append(res.Unchanged, name)
        }
    }

    for name := range actual {
        if _, ok := base[name]; !ok {
            res.OnlyInActual = append(res.OnlyInActual, name)
        }
    }

    return res
}

func PrintServicesDiff(d ServiceDiffResult, showUnchanged bool) string {
    var sb strings.Builder

    sort.Strings(d.OnlyInBase)
    sort.Strings(d.OnlyInActual)

    if len(d.OnlyInBase) > 0 {
        fmt.Fprintf(&sb, "\nStopped/removed services (%d):\n", len(d.OnlyInBase))
        for _, name := range d.OnlyInBase {
            fmt.Fprintf(&sb, "   - %s\n", name)
        }
    }

    if len(d.OnlyInActual) > 0 {
        fmt.Fprintf(&sb, "\nNew services (%d):\n", len(d.OnlyInActual))
        for _, name := range d.OnlyInActual {
            fmt.Fprintf(&sb, "   + %s\n", name)
        }
    }

    if len(d.Changed) > 0 {
        fmt.Fprintf(&sb, "\nChanged services (%d):\n", len(d.Changed))
        sort.Slice(d.Changed, func(i, j int) bool {
            return d.Changed[i].Name < d.Changed[j].Name
        })
        for _, sd := range d.Changed {
            fmt.Fprintf(&sb, "   ~ %s\n", sd.Name)
            for _, ch := range sd.Changes {
                fmt.Fprintf(&sb, "       %-6s: %q --> %q\n", ch.Field, ch.Old, ch.New)
            }
        }
    }

    if showUnchanged && len(d.Unchanged) > 0 {
        fmt.Fprintf(&sb, "\nUnchanged services (%d):\n", len(d.Unchanged))
        sort.Strings(d.Unchanged)
        for _, name := range d.Unchanged {
            fmt.Fprintf(&sb, "   = %s\n", name)
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

func GetServicesList(units []string) (ServiceList, []error) {
    result := make(ServiceList)
    var errs []error

    conn, err := dbus.NewSystemConnection()
    if err != nil {
        return nil, []error{fmt.Errorf("dbus connect: %w", err)}
    }
    defer conn.Close()

    // Если задан явный список — используем его.
    // Иначе — берём все .service из ListUnits.
    var names []string
    if len(units) > 0 {
        names = units
    } else {
        all, err := conn.ListUnits()
        if err != nil {
            return nil, []error{fmt.Errorf("list units: %w", err)}
        }
        for _, u := range all {
            if strings.HasSuffix(u.Name, ".service") {
                names = append(names, u.Name)
            }
        }
    }

    for _, name := range names {
        props, err := conn.GetUnitProperties(name)
        if err != nil {
            errs = append(errs, fmt.Errorf("%s: get properties: %w", name, err))
            continue
        }

        meta := ServiceMeta{Name: name}

        if v, ok := props["LoadState"].(string); ok {
            meta.LoadState = v
        }
        if v, ok := props["ActiveState"].(string); ok {
            meta.ActiveState = v
        }
        if v, ok := props["SubState"].(string); ok {
            meta.SubState = v
        }

        // MainPID — в интерфейсе Service, отдельный запрос.
        prop, err := conn.GetUnitTypeProperty(name, "Service", "MainPID")
        if err == nil && prop != nil {
            var pid uint32
            if err := prop.Value.Store(&pid); err == nil {
                meta.MainPID = pid
            }
        }

        result[name] = meta
    }

    return result, errs
}

func cloneServiceList(src ServiceList) ServiceList {
    dst := make(ServiceList, len(src))
    for k, v := range src {
        dst[k] = v
    }
    return dst
}

func ServicesCheck(units []string, previousServiceList ServiceList) (ServiceList, string) {
    log.Println("\nRunning checklibs/ServicesCheck")

    currentServiceList, errs := GetServicesList(units)
    for _, e := range errs {
        log.Printf("warn: %v", e)
    }

    if currentServiceList == nil {
        return previousServiceList, ""
    }

    if previousServiceList == nil {
        log.Printf("Baseline collected: %d services", len(currentServiceList))
        return cloneServiceList(currentServiceList), ""
    }

    diff := CompareServiceLists(previousServiceList, currentServiceList)
    out := PrintServicesDiff(diff, false)

    return cloneServiceList(currentServiceList), out
}
