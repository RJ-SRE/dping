package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/go-ping/ping"
)

var (
	statsStore    = NewPingStatsStore(25) // 初始化数据存储，最多保存25条最近记录
	wgHandleDPing sync.WaitGroup
)

func DPing(isp string, detection string, maxConcurrency int, count int, eth string, sort string, des bool) {

	sem := make(chan struct{}, maxConcurrency) //限制并发数
	// 获取指定网卡IP
	localIP, _ := getPrimaryLocalIP(eth)

	// 解析DNS配置
	DnsBuffer := &DNSConfig{}
	if err := json.Unmarshal([]byte(JsonData), DnsBuffer); err != nil {
		fmt.Println("Dns-Buffer-解析异常:", err)
		return
	}

	// 验证并处理运营商参数
	validIsps := map[string]bool{"电信": true, "联通": true, "移动": true, "all": true}
	ispVal := isp
	if !validIsps[ispVal] {
		log.Printf("⚠️  不支持的运营商 '%s'，已使用默认值 'all'\n", ispVal)
		ispVal = "all"
	}

	// 验证并处理区域参数
	regionVal := detection
	if regionVal != "全国" && !isRegionExist(ispVal, regionVal, DnsBuffer) {
		log.Printf("⚠️  区域 '%s' 不存在于运营商 '%s' 中，已使用默认值 '全国'\n", regionVal, ispVal)
		regionVal = "全国"
	}

	// 显示使用的本地IP
	localIPStr := "系统默认"
	if localIP != nil {
		localIPStr = localIP.String()
	}
	fmt.Printf("✅ 最终使用参数：区域=%s，运营商=%s，源IP=%s\n",
		regionVal, ispVal, localIPStr)

	// 初始化并发控制和统计通道
	var wg sync.WaitGroup
	ChStatistics := make(chan *PingStatistic, 20)

	wgHandleDPing.Add(1) // 标记 HandleDPing 任务开始
	go HandleDPing(ChStatistics, statsStore, &wgHandleDPing, sort, des)

	// 确定目标运营商列表
	targetIsps := []string{ispVal}
	if ispVal == "all" {
		targetIsps = []string{"电信", "联通", "移动"}
	}

	// 定义获取IP列表的函数
	getIPList := func(ispName, region string) []string {
		switch ispName {
		case "电信":
			if regionData, ok := DnsBuffer.Dx[region]; ok {
				return regionData.IPv4
			}
		case "联通":
			if regionData, ok := DnsBuffer.Lt[region]; ok {
				return regionData.IPv4
			}
		case "移动":
			if regionData, ok := DnsBuffer.Yd[region]; ok {
				return regionData.IPv4
			}
		}
		return nil
	}

	// 处理IP Ping任务的函数，使用本地IP作为源IP
	processIPs := func(ipList []string, region, ispName string, srcIP net.IP) {
		if len(ipList) == 0 {
			log.Printf("⚠️ 区域 %s 下运营商 %s 无 IP", region, ispName)
			return
		}
		for _, ip := range ipList {
			wg.Add(1)

			go func(ip string) {
				sem <- struct{}{} //通过管道限制并发次数，不然大量的并发ping，会消耗系统的socket资源，导致系统误判
				defer func() {
					<-sem
					wg.Done()
				}()
				Ping(net.ParseIP(ip), region, ispName, srcIP, ChStatistics, count)
			}(ip)
		}
	}

	// 根据区域参数执行不同的Ping逻辑
	if regionVal != "全国" {
		for _, ispName := range targetIsps {
			ipList := getIPList(ispName, regionVal)
			processIPs(ipList, regionVal, ispName, localIP)
		}
		wg.Wait()
		close(ChStatistics)
	} else {
		// 处理全国区域的情况
		ispRegions := map[string]map[string]ProvinceConfig{
			"电信": DnsBuffer.Dx,
			"联通": DnsBuffer.Lt,
			"移动": DnsBuffer.Yd,
		}

		for ispName, regions := range ispRegions {
			if ispVal != "all" && ispName != ispVal {
				continue
			}
			for region, ipLists := range regions {
				processIPs(ipLists.IPv4, region, ispName, localIP)
			}
		}
		wg.Wait()
		close(ChStatistics)
	}
	// 等待 HandleDPing 完成
	wgHandleDPing.Wait()
}

func Ping(to net.IP, Region string, Isp string, sourceIP net.IP, ChStatistics chan<- *PingStatistic, count int) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(err)
		}
	}()

	pinger, err := ping.NewPinger(to.String())
	if err != nil {
		fmt.Printf("Ping Start Error: %v", err)
		return
	}

	// 如果获取到了本地IP，则设置为源IP
	if sourceIP != nil {
		pinger.Source = sourceIP.String()
	}

	pinger.SetPrivileged(true)
	pinger.Count = count
	pinger.Timeout = time.Duration(count+5) * time.Second
	err = pinger.Run()
	if err != nil {
		fmt.Printf("Ping Run Error: %v", err)
		return
	}
	stats := pinger.Statistics()
	ChStatistics <- &PingStatistic{
		SrcIp:     pinger.Source, // 显示实际使用的源IP
		DecIp:     to.String(),
		Region:    Region,
		Isp:       Isp,
		Statistic: stats,
	}
}

// HandleDPing 处理统计数据并以表格形式展示
func HandleDPing(ChStatistics <-chan *PingStatistic, store *PingStatsStore, wg *sync.WaitGroup, sort string, des bool) {
	defer wg.Done()

	processedCount := 0
	// 进度条刷新函数
	printProgress := func(processed int) {
		fmt.Printf("\r进度:%d", processed)
	}

	for {
		select {
		case stats, ok := <-ChStatistics:
			if !ok {
				// 通道关闭，打印换行和最终结果
				fmt.Println()
				//		fmt.Println("====== 最终汇总统计结果 ======")
				//		printSummaryList(store.GetSummarySorted(sort, des))
				fmt.Println("====== 汇总统计结果 ======")
				SummaryStatistic := store.GetSummarySortedGroupedByIsp(sort, des)
				printSummaryList(SummaryStatistic)
				fmt.Println("====== 丢包汇总统计结果 ======")
				printSummaryList(store.GetLossOnlyGroupedByIspSorted(SummaryStatistic, sort, des))

				return
			}
			PacketLoss := stats.Statistic.PacketLoss

			if PacketLoss != 100 {
				store.Add(stats)
			}
			processedCount++
			printProgress(processedCount)
		}
	}
}

func isRegionExist(isp string, region string, dns *DNSConfig) bool {
	switch isp {
	case "电信":
		_, ok := dns.Dx[region]
		return ok
	case "联通":
		_, ok := dns.Lt[region]
		return ok
	case "移动":
		_, ok := dns.Yd[region]
		return ok
	case "all":
		_, ok1 := dns.Dx[region]
		_, ok2 := dns.Lt[region]
		_, ok3 := dns.Yd[region]
		return ok1 || ok2 || ok3
	default:
		return false
	}
}

// 获取指定网卡的主IPv4地址
func getPrimaryLocalIP(eth string) (net.IP, error) {
	iface, err := net.InterfaceByName(eth)
	if err != nil {
		return nil, fmt.Errorf("获取网卡 %s 失败: %v", eth, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("获取网卡 %s 的地址失败: %v", eth, err)
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			ip := ipnet.IP.To4()
			if ip != nil && !ip.IsLoopback() {
				return ip, nil
			}
		}
	}

	return nil, fmt.Errorf("网卡 %s 无有效的 IPv4 地址", eth)
}
