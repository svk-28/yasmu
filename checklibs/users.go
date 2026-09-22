// ********************************************************************
// Watchdog - the utility for monitoring system integrity.
// And for web monitoring of websites availability.
// ********************************************************************
// Author: Sergey Kalinin
// License: GPL v3
// ********************************************************************
// User verification module.
// Allows you to control groups, password changes, and changes to the
// home directory.
// Control added and changes ssh keys for all users.
// ********************************************************************

package checklibs

import (
    "bufio"
    "fmt"
    "log"
    "maps"
    "os"
    "os/user"
    "sort"
    "strings"
    "path/filepath"
)

type UserList map[string]UserMeta

type UserMeta struct {
    Name    string
    UID     string
    HomeDir string
    Groups  []UserGroups
    SSHKeys []SSHKey // ключи из ~/.ssh/authorized_keys
    SSHDir  bool     // существует ли ~/.ssh
}

type UserGroups struct {
    GroupName string
    GID       string
}

// UserFieldChange — одно изменённое поле пользователя.
type UserFieldChange struct {
    Field string
    Old   string
    New   string
}

// UserDiff — изменения одного пользователя.
type UserDiff struct {
    Name    string
    Changes []UserFieldChange
}

// UserDiffResult — результат сравнения двух снимков UserList.
type UserDiffResult struct {
    OnlyInBase   []string   // удалённые пользователи
    OnlyInActual []string   // добавленные пользователи
    Changed      []UserDiff // изменённые
    Unchanged    []string   // без изменений
}

// SSHKey — один публичный ключ пользователя.
type SSHKey struct {
    Type    string // ssh-rsa, ssh-ed25519, ecdsa-sha2-...
    Comment string // комментарий из конца строки (обычно user@host)
    Finger  string // отпечаток, если удалось посчитать
}



func CompareUserLists(base, actual UserList) UserDiffResult {
    var res UserDiffResult

    for name, b := range base {
        a, ok := actual[name]
        if !ok {
            res.OnlyInBase = append(res.OnlyInBase, name)
            continue
        }

        var changes []UserFieldChange

        if b.UID != a.UID {
            changes = append(changes, UserFieldChange{"uid", b.UID, a.UID})
        }
        if b.HomeDir != a.HomeDir {
            changes = append(changes, UserFieldChange{"home", b.HomeDir, a.HomeDir})
        }

        bGroups := groupsToString(b.Groups)
        aGroups := groupsToString(a.Groups)
        if bGroups != aGroups {
            changes = append(changes, UserFieldChange{"groups", bGroups, aGroups})
        }
        // Появление/исчезновение каталога .ssh.
        if b.SSHDir != a.SSHDir {
            changes = append(changes, UserFieldChange{
                "ssh_dir",
                fmt.Sprintf("%v", b.SSHDir),
                fmt.Sprintf("%v", a.SSHDir),
            })
        }

        // Изменения в наборе ключей.
        bKeys := sshKeysToString(b.SSHKeys)
        aKeys := sshKeysToString(a.SSHKeys)

        if bKeys != aKeys {
            changes = append(changes, UserFieldChange{"ssh_keys", bKeys, aKeys})
        }

        if len(changes) > 0 {
            res.Changed = append(res.Changed, UserDiff{Name: name, Changes: changes})
        } else {
            res.Unchanged = append(res.Unchanged, name)
        }
    }

    // Новые пользователи.
    for name := range actual {
        if _, ok := base[name]; !ok {
            res.OnlyInActual = append(res.OnlyInActual, name)
        }
    }

    return res
}

// groupsToString превращает список групп в сортированную строку "g1:gid1,g2:gid2".
// Порядок фиксирован, чтобы сравнение не зависело от порядка в /etc/group.
func groupsToString(groups []UserGroups) string {
    parts := make([]string, 0, len(groups))
    for _, g := range groups {
        parts = append(parts, g.GroupName+":"+g.GID)
    }
    sort.Strings(parts)
    return strings.Join(parts, ",")
}

func PrintUsersDiff(d UserDiffResult, showUnchanged bool) string {
    var sb strings.Builder

    sort.Strings(d.OnlyInBase)
    sort.Strings(d.OnlyInActual)

    if len(d.OnlyInBase) > 0 {
        fmt.Fprintf(&sb, "\nDeleted users (%d):\n", len(d.OnlyInBase))
        for _, name := range d.OnlyInBase {
            fmt.Fprintf(&sb, "   - %s\n", name)
        }
    }

    if len(d.OnlyInActual) > 0 {
        fmt.Fprintf(&sb, "\nAdded users (%d):\n", len(d.OnlyInActual))
        for _, name := range d.OnlyInActual {
            fmt.Fprintf(&sb, "   + %s\n", name)
        }
    }

    if len(d.Changed) > 0 {
        fmt.Fprintf(&sb, "\nChanged users (%d):\n", len(d.Changed))
        sort.Slice(d.Changed, func(i, j int) bool {
            return d.Changed[i].Name < d.Changed[j].Name
        })
        for _, ud := range d.Changed {
            fmt.Fprintf(&sb, "   ~ %s\n", ud.Name)
            for _, ch := range ud.Changes {
                fmt.Fprintf(&sb, "       %-6s: %q --> %q\n", ch.Field, ch.Old, ch.New)
            }
        }
    }

    if showUnchanged && len(d.Unchanged) > 0 {
        fmt.Fprintf(&sb, "\nUnchanged users (%d):\n", len(d.Unchanged))
        sort.Strings(d.Unchanged)
        for _, name := range d.Unchanged {
            fmt.Fprintf(&sb, "   = %s\n", name)
        }
    }

    fmt.Fprintf(&sb, "\nSummary: deleted %d, added %d, changed %d, all %d\n",
        len(d.OnlyInBase), len(d.OnlyInActual),
        len(d.Changed), len(d.Unchanged))

    if len(d.OnlyInBase) > 0 || len(d.OnlyInActual) > 0 || len(d.Changed) > 0 {
        return sb.String()
    }
    return ""
}


func cloneUserList(src UserList) UserList {
    dst := make(UserList, len(src))
    for k, v := range src {
        v.Groups = append([]UserGroups(nil), v.Groups...)
        dst[k] = v
    }
    return dst
}

func UsersCheck(previousUsersList UserList) (UserList, string) {
    log.Println("\nRunning checklibs/UsersCheck\n")

    currentUsersList := make(UserList)

    file, err := os.Open("/etc/passwd")
    if err != nil {
        log.Printf("Reading error /etc/passwd: %v\n", err)
        return currentUsersList, ""
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    count := 0

    for scanner.Scan() {
        line := scanner.Text()
        if strings.HasPrefix(line, "#") {
            continue
        }
        parts := strings.Split(line, ":")
        if len(parts) < 7 {
            continue
        }

        userName := parts[0]

        meta, err := GetUserInfo(userName)
        if err != nil {
            fmt.Printf("\t\t%v\n", err)
            continue
        }
        currentUsersList[userName] = meta
        count++
    }

    if err := scanner.Err(); err != nil {
        log.Printf("Scanner error: %v\n", err)
    }

    log.Printf("\tTotal found: %d users\n", count)

    if previousUsersList == nil {
        return maps.Clone(currentUsersList), ""
    }

    diff := CompareUserLists(previousUsersList, currentUsersList)
    out := PrintUsersDiff(diff, false)

    return cloneUserList(currentUsersList), out
}

func GetUserInfo(userName string) (UserMeta, error) {
    var meta UserMeta

    usr, err := user.Lookup(userName)
    if err != nil {
        return meta, fmt.Errorf("user %q not found: %w", userName, err)
    }

    meta.Name = usr.Username
    meta.UID = usr.Uid
    meta.HomeDir = usr.HomeDir

    groupIds, err := usr.GroupIds()
    if err != nil {
        return meta, nil
    }

    for _, gid := range groupIds {
        group, err := user.LookupGroupId(gid)
        if err != nil {
            continue
        }
        meta.Groups = append(meta.Groups, UserGroups{
            GroupName: group.Name,
            GID:       group.Gid,
        })
    }
    
    // SSH-ключи и каталог .ssh.
    keys, sshDir, err := ReadSSHKeys(usr.HomeDir)
    if err != nil {
        log.Printf("[warn] reading SSH keys for %s: %v", userName, err)
    }
    meta.SSHKeys = keys
    meta.SSHDir = sshDir
    
    return meta, nil
}

// ReadSSHKeys читает ~/.ssh/authorized_keys пользователя.
// Возвращает список ключей, флаг существования каталога .ssh и ошибку.
// Отсутствие каталога или файла — не ошибка: возвращается пустой список.
func ReadSSHKeys(homeDir string) ([]SSHKey, bool, error) {
    if homeDir == "" {
        return nil, false, nil
    }

    sshDir := filepath.Join(homeDir, ".ssh")

    // Проверяем, существует ли каталог .ssh.
    info, err := os.Stat(sshDir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, false, nil // каталога нет — это не ошибка
        }
        return nil, false, err
    }
    if !info.IsDir() {
        return nil, false, fmt.Errorf("%s is not a directory", sshDir)
    }

    authFile := filepath.Join(sshDir, "authorized_keys")
    f, err := os.Open(authFile)
    if err != nil {
        if os.IsNotExist(err) {
            // .ssh есть, но authorized_keys нет — каталог считаем существующим.
            return nil, true, nil
        }
        return nil, true, err
    }
    defer f.Close()

    var keys []SSHKey
    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        keys = append(keys, parseSSHKey(line))
    }
    if err := scanner.Err(); err != nil {
        return keys, true, err
    }

    return keys, true, nil
}

// parseSSHKey разбирает строку authorized_keys.
// Формат: "ssh-rsa AAAA... comment" или "ssh-ed25519 AAAA... comment".
// Нам достаточно типа и комментария — сам ключ длинный и меняется редко.
func parseSSHKey(line string) SSHKey {
    fields := strings.Fields(line)
    if len(fields) == 0 {
        return SSHKey{}
    }

    key := SSHKey{Type: fields[0]}
    // Всё, что после base64-тела — комментарий.
    // Fields: [type, base64, comment, ...] — комментарий склеиваем обратно.
    if len(fields) >= 3 {
        key.Comment = strings.Join(fields[2:], " ")
    }
    return key
}

// sshKeysToString — сортированное представление ключей для сравнения.
// Формат: "type comment;type comment;...".
func sshKeysToString(keys []SSHKey) string {
    parts := make([]string, 0, len(keys))
    for _, k := range keys {
        parts = append(parts, k.Type+" "+k.Comment)
    }
    sort.Strings(parts)
    return strings.Join(parts, "; ")
}

