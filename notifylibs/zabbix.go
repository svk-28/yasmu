// *******************************************************
// YASMU - Yet Another System Monitoring Utility
// The utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
// License: GPL v3
// *******************************************************
// Zabbix sending module
// *******************************************************

package notifylibs

import (
    "time"
    "log"
    	"github.com/adubkov/go-zabbix"
)

type ZabbixServer struct {
    Address string
    Port    int
}

func ZabbixSender(zabbixServer ZabbixServer, zabbixHost string, metricName string, metricValue string) (bool, error) {
	var (
		metrics     []*zabbix.Metric
	)
	// Read and checkin zabbix server and port

	metrics = append(metrics, zabbix.NewMetric(zabbixHost, metricName, metricValue, time.Now().Unix()))
	metrics = append(metrics, zabbix.NewMetric(zabbixHost, "status", "OK"))

	// Create instance of Packet class
	packet := zabbix.NewPacket(metrics)
	// fmt.Println(zabbixHost, metric_name, metric_value, metrics)
	// Send packet to zabbix
	z := zabbix.NewSender(zabbixServer.Address, zabbixServer.Port)
	res, err := z.Send(packet)
    if err != nil {
        return false, err
    }
	log.Println(string(res))
	return true, nil
}
