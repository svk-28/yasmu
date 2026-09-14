package checklibs

import (
    "fmt"
    // "net"
    "github.com/sokurenko/go-netstat/netstat"
)

func PortsCheck() {
    fmt.Println("\nRunning checklibs/PortsCheck\n")
    fmt.Println("  Listening ports\n")

    // 1. TCP - только слушающие (LISTEN)
    fmt.Println("\tTCP")
    tcpListeners, err := netstat.TCPSocks(func(s *netstat.SockTabEntry) bool {
        return s.State == netstat.Listen
    })
    if err != nil {
        fmt.Printf("Ошибка при получении TCP: %v\n", err)
    } else {
        for _, s := range tcpListeners {
            printSocketInfo("TCP", s)
        }
        if len(tcpListeners) == 0 {
            fmt.Println("  (нет слушающих TCP портов)")
        }
    }

    // 2. UDP - все открытые сокеты
    fmt.Println("\n\tUDP")
    udpSockets, err := netstat.UDPSocks(netstat.NoopFilter)
    if err != nil {
        fmt.Printf("Ошибка при получении UDP: %v\n", err)
    } else {
        for _, s := range udpSockets {
            printSocketInfo("UDP", s)
        }
        if len(udpSockets) == 0 {
            fmt.Println("  (нет открытых UDP сокетов)")
        }
    }
}

// printSocketInfo выводит информацию о сокете в читаемом виде
func printSocketInfo(proto string, s netstat.SockTabEntry) {
    // Форматируем локальный адрес
    localIP := s.LocalAddr.IP.String()
    if localIP == "" {
        localIP = "0.0.0.0"
    }
    localPort := s.LocalAddr.Port
    
    // Форматируем удалённый адрес (если есть)
    remoteInfo := ""
    if s.RemoteAddr != nil && s.RemoteAddr.IP != nil {
        remoteIP := s.RemoteAddr.IP.String()
        if remoteIP == "" {
            remoteIP = "*"
        }
        remotePort := s.RemoteAddr.Port
        if remotePort > 0 {
            remoteInfo = fmt.Sprintf(" -> %s:%d", remoteIP, remotePort)
        }
    }
    
    // Информация о процессе (если доступна)
    processInfo := ""
    if s.Process != nil {
        processInfo = fmt.Sprintf(" [PID: %d, %s]", s.Process.Pid, s.Process.Name)
    }
    
    // Статус для TCP
    status := ""
    if s.State != 0 {
        status = fmt.Sprintf(" [%s]", s.State.String())
    }
    
    fmt.Printf("\t%s, %d, %s, %s, %s\n", 
        localIP, localPort,
        status,
        remoteInfo,
        processInfo,
    )
}
