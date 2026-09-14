package checklibs

import (
    "bufio"
    "fmt"
    "os"
    "os/user"
    "strings"
)

func UsersCheck() {
    fmt.Println("\nRunning checklibs/UsersCheck\n")

    fmt.Println("  Users list\n")
    
/*
    // Текущий пользователь
    usr, _ := user.Current()
    fmt.Printf("\nТекущий пользователь: %s (UID: %s)\n", usr.Username, usr.Uid)
    
    // Группы текущего пользователя
    groupIds, _ := usr.GroupIds()
    fmt.Println("Его группы:")
    for _, gid := range groupIds {
        group, err := user.LookupGroupId(gid)
        if err == nil {
            fmt.Printf("  - %s (GID: %s)\n", group.Name, group.Gid)
        }
    }
*/
    
    // Все пользователи системы (из /etc/passwd)
    file, err := os.Open("/etc/passwd")
    if err != nil {
        fmt.Printf("Ошибка чтения /etc/passwd: %v\n", err)
        return
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
        if len(parts) >= 7 {
            username := parts[0]
            uid := parts[2]
            // Показываем только "реальных" пользователей (UID >= 1000)
            // или всех, если нужно
            fmt.Printf("\t%s (UID: %s)\n", username, uid)
            GetUserInfo(username)
            count++
        }
    }
    fmt.Printf("\tВсего найдено: %d пользователей\n", count)
}

func GetUserInfo(username string) {
    
    usr, err := user.Lookup(username)
    if err != nil {
        fmt.Printf("Пользователь %s не найден: %v", username, err)
    }

    fmt.Printf("\t\tПользователь: %s (UID: %s)\n", usr.Username, usr.Uid)
    fmt.Printf("\t\tДомашняя директория: %s\n", usr.HomeDir)
    
    groupIds, _ := usr.GroupIds()
    fmt.Println("\t\tГруппы пользователя:")
    for _, gid := range groupIds {
        group, err := user.LookupGroupId(gid)
        if err == nil {
            fmt.Printf("\t\t- %s (GID: %s)\n", group.Name, group.Gid)
        }
    }
}

func UsersPermissionCheck() {
    fmt.Println("\nRunning checklibs/UsersPermissonCheck")
}

func UsersSudoersCheck() {
    fmt.Println("\nRunning checklibs/UsersSudoersCheck")
}


