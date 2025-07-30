package internal_test

import (
	"dping/internal"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/go-ping/ping"
)

func TestDping(t *testing.T) {
	var DnsBuffer internal.DNSConfig
	err := json.Unmarshal([]byte(internal.JsonData), &DnsBuffer)
	if err != nil {
		fmt.Println("Dns-Buffer-解析异常:", err)
		return
	}
	statsStore := internal.NewPingStatsStore(25)
	var wg sync.WaitGroup
	var ChStatistics = make(chan *internal.PingStatistic, 20)

	go internal.HandleDPing(ChStatistics, statsStore, 2*time.Second)
	var soureIP = &net.IP{100, 100, 20, 30}
	for Region, IpLists := range DnsBuffer.Yd {
		for _, Ip := range IpLists.IPv4 {
			wg.Add(1)
			go internal.Ping(net.ParseIP(Ip), Region, "移动", *soureIP, ChStatistics)
		}
	}

	wg.Wait()
	close(ChStatistics)
}

func TestPing(t *testing.T) {

	pinger, err := ping.NewPinger("211.136.18.171")
	pinger.SetPrivileged(true)
	if err != nil {
		fmt.Printf("Ping Start Error: %v", err)
	}
	pinger.Count = 3
	pinger.Timeout = 10 * time.Second
	err = pinger.Run()
	if err != nil {
		fmt.Printf("Ping Run Error: %v", err)
	}
	stats := pinger.Statistics()
	fmt.Print(stats)
}
