package checklibs

import (
    "fmt"
    "github.com/sokurenko/go-netstat/netstat"
)

func NetworkCheck() {
    fmt.Println("\nRunning checklibs/NetworkCheck\n")

    fmt.Println("  Active connections\n")
    
    // TCP активные (все состояния, кроме LISTEN)
    tcp, err := netstat.TCPSocks(func(s *netstat.SockTabEntry) bool {
        // Просто исключаем LISTEN
        return s.State != netstat.Listen
    })
    if err != nil {
        fmt.Printf("Ошибка при получении TCP: %v\n", err)
    } else {
        for _, s := range tcp {
            remote := "0.0.0.0:0"
            if s.RemoteAddr != nil {
                remote = fmt.Sprintf("%s:%d", s.RemoteAddr.IP, s.RemoteAddr.Port)
            }
            
            procInfo := ""
            if s.Process != nil {
                procInfo = fmt.Sprintf(" [PID: %d, %s]", s.Process.Pid, s.Process.Name)
            }
            
            fmt.Printf("\tTCP  %s:%d -> %s [%s]%s\n",
                s.LocalAddr.IP, s.LocalAddr.Port,
                remote,
                s.State.String(),
                procInfo,
            )
        }
        if len(tcp) == 0 {
            fmt.Println("  (  нет активных TCP соединений)")
        }
    }
    
    // UDP активные (есть удалённый адрес)
    udp, err := netstat.UDPSocks(func(s *netstat.SockTabEntry) bool {
        return s.RemoteAddr != nil && 
               s.RemoteAddr.IP != nil && 
               !s.RemoteAddr.IP.IsUnspecified()
    })
    if err != nil {
        fmt.Printf("Ошибка при получении UDP: %v\n", err)
    } else {
        for _, s := range udp {
            remote := fmt.Sprintf("%s:%d", s.RemoteAddr.IP, s.RemoteAddr.Port)
            
            procInfo := ""
            if s.Process != nil {
                procInfo = fmt.Sprintf(" [PID: %d, %s]", s.Process.Pid, s.Process.Name)
            }
            
            fmt.Printf("\tUDP  %s:%d -> %s%s\n",
                s.LocalAddr.IP, s.LocalAddr.Port,
                remote,
                procInfo,
            )
        }
        if len(udp) == 0 {
            fmt.Println("  (  нет активных UDP соединений)")
        }
    }
}
