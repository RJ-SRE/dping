package internal

import (
	"fmt"
	"github.com/go-ping/ping"
	"github.com/olekukonko/tablewriter"
	"os"
	"sort"
	"sync"
	"time"
)

type DNSConfig struct {
	Dx map[string]ProvinceConfig `json:"电信"`
	Lt map[string]ProvinceConfig `json:"联通"`
	Yd map[string]ProvinceConfig `json:"移动"`
}

type ProvinceConfig struct {
	IPv4 []string `json:"IPv4"`
}

type PingStatistic struct {
	SrcIp     string
	DecIp     string
	Region    string
	Isp       string
	Statistic *ping.Statistics
}

// SummaryStatistic 存储汇总统计信息
type SummaryStatistic struct {
	DestIP                string
	Region                string
	Isp                   string
	TotalSent             int
	TotalRecv             int
	MinRtt                time.Duration
	MaxRtt                time.Duration
	AvgRtt                time.Duration
	MinRttAvg             time.Duration
	MaxRttAvg             time.Duration
	LastUpdated           time.Time
	PacketLoss            float64 //丢包
	PacketsRecvDuplicates int     //重传
}

type IspSummary struct {
	Isp           string
	RegionCount   int
	TotalSent     int
	TotalRecv     int
	TotalLoss     float64
	AvgPacketLoss float64
	AvgRtt        time.Duration
}

// PingStatsStore 线程安全的数据存储结构
type PingStatsStore struct {
	mu          sync.Mutex
	summaryData map[string]*SummaryStatistic // 按目标IP汇总
	recentStats []*PingStatistic             // 最近的记录
	maxRecent   int                          // 最大最近记录数
}

// NewPingStatsStore 创建新的数据存储
func NewPingStatsStore(maxRecent int) *PingStatsStore {
	return &PingStatsStore{
		summaryData: make(map[string]*SummaryStatistic),
		maxRecent:   maxRecent,
	}
}

// Add 添加新的Ping统计数据（补充最小/最大RTT平均计算）
func (s *PingStatsStore) Add(stat *PingStatistic) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 更新最近记录（原有逻辑保留）
	s.recentStats = append(s.recentStats, stat)
	if len(s.recentStats) > s.maxRecent {
		s.recentStats = s.recentStats[1:]
	}

	// 更新汇总数据
	key := stat.DecIp
	if _, exists := s.summaryData[key]; !exists {
		s.summaryData[key] = &SummaryStatistic{
			DestIP:                stat.DecIp,
			Region:                stat.Region,
			Isp:                   stat.Isp,
			MinRtt:                time.Hour, // 初始化为较大值
			MinRttAvg:             0,         // 初始化最小RTT平均
			MaxRttAvg:             0,         // 初始化最大RTT平均
			PacketLoss:            stat.Statistic.PacketLoss,
			PacketsRecvDuplicates: stat.Statistic.PacketsRecvDuplicates,
		}
	}

	sum := s.summaryData[key]
	statsData := stat.Statistic

	// 基础统计更新（原有逻辑保留）
	sum.TotalSent += statsData.PacketsSent
	sum.TotalRecv += statsData.PacketsRecv
	sum.LastUpdated = time.Now()

	// 更新RTT统计（补充最小/最大RTT平均计算）
	// 1. 最小RTT及平均值
	if statsData.MinRtt > 0 && statsData.MinRtt < sum.MinRtt {
		sum.MinRtt = statsData.MinRtt
	}
	// 计算最小RTT平均：(当前累计平均 * 已统计次数 + 新值) / (已统计次数 + 1)
	// 这里以"接收包数"作为统计次数（也可根据实际需求用其他维度）
	if sum.TotalRecv > statsData.PacketsRecv { // 避免首次计算时分母为0
		sum.MinRttAvg = (sum.MinRttAvg*time.Duration(sum.TotalRecv-statsData.PacketsRecv) + statsData.MinRtt*time.Duration(statsData.PacketsRecv)) / time.Duration(sum.TotalRecv)
	} else {
		sum.MinRttAvg = statsData.MinRtt // 首次统计直接赋值
	}
	// 2. 最大RTT及平均值
	if statsData.MaxRtt > sum.MaxRtt {
		sum.MaxRtt = statsData.MaxRtt
	}

	// 计算最大RTT平均（同最小RTT平均逻辑）
	if sum.TotalRecv > statsData.PacketsRecv {
		sum.MaxRttAvg = (sum.MaxRttAvg*time.Duration(sum.TotalRecv-statsData.PacketsRecv) + statsData.MaxRtt*time.Duration(statsData.PacketsRecv)) / time.Duration(sum.TotalRecv)
	} else {
		sum.MaxRttAvg = statsData.MaxRtt // 首次统计直接赋值
	}

	// 3. 原有平均RTT计算（保留）
	if sum.TotalRecv > 0 {
		sum.AvgRtt = (sum.AvgRtt*time.Duration(sum.TotalRecv-statsData.PacketsRecv) +
			statsData.AvgRtt*time.Duration(statsData.PacketsRecv)) /
			time.Duration(sum.TotalRecv)
	}
}

// GetRecent 获取最近的记录
func (s *PingStatsStore) GetRecent() []*PingStatistic {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 返回副本避免外部修改
	recent := make([]*PingStatistic, len(s.recentStats))
	copy(recent, s.recentStats)
	return recent
}

// GetSummary 获取汇总数据（补充MinRttAvg和MaxRttAvg的复制）
func (s *PingStatsStore) GetSummary() map[string]*SummaryStatistic {
	s.mu.Lock()
	defer s.mu.Unlock()

	summary := make(map[string]*SummaryStatistic)
	for k, v := range s.summaryData {
		summary[k] = &SummaryStatistic{
			DestIP:                v.DestIP,
			Region:                v.Region,
			Isp:                   v.Isp,
			TotalSent:             v.TotalSent,
			TotalRecv:             v.TotalRecv,
			MinRtt:                v.MinRtt,
			MaxRtt:                v.MaxRtt,
			AvgRtt:                v.AvgRtt,
			MinRttAvg:             v.MinRttAvg, // 补充复制最小RTT平均
			MaxRttAvg:             v.MaxRttAvg, // 补充复制最大RTT平均
			LastUpdated:           v.LastUpdated,
			PacketLoss:            v.PacketLoss,
			PacketsRecvDuplicates: v.PacketsRecvDuplicates,
		}
	}
	return summary
}

// GetSummarySorted 返回按指定字段排序的列表
func (s *PingStatsStore) GetSummarySorted(field string, descending bool) []*SummaryStatistic {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 拍平成 slice
	var statsList []*SummaryStatistic
	for _, v := range s.summaryData {
		statsList = append(statsList, &SummaryStatistic{
			DestIP:                v.DestIP,
			Region:                v.Region,
			Isp:                   v.Isp,
			TotalSent:             v.TotalSent,
			TotalRecv:             v.TotalRecv,
			MinRtt:                v.MinRtt,
			MaxRtt:                v.MaxRtt,
			AvgRtt:                v.AvgRtt,
			MinRttAvg:             v.MinRttAvg,
			MaxRttAvg:             v.MaxRttAvg,
			LastUpdated:           v.LastUpdated,
			PacketLoss:            v.PacketLoss,
			PacketsRecvDuplicates: v.PacketsRecvDuplicates,
		})
	}

	// 排序逻辑
	sort.Slice(statsList, func(i, j int) bool {
		var less bool
		switch field {
		case "minrtt":
			less = statsList[i].MinRtt < statsList[j].MinRtt
		case "maxrtt":
			less = statsList[i].MaxRtt < statsList[j].MaxRtt
		case "avgrtt":
			less = statsList[i].AvgRtt < statsList[j].AvgRtt
		default:
			less = statsList[i].PacketLoss < statsList[j].PacketLoss // 默认按丢包
		}
		if descending {
			return !less
		}
		return less
	})

	return statsList
}

// GetSummarySortedGroupedByIsp  根据 ISP 分组聚合，并按指定字段排序（loss/rtt/sent/recv）
func (s *PingStatsStore) GetSummarySortedGroupedByIsp(field string, descending bool) []*SummaryStatistic {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 先按 ISP 分组
	grouped := make(map[string][]*SummaryStatistic)
	for _, v := range s.summaryData {
		// 直接引用原始对象，保持完整信息
		grouped[v.Isp] = append(grouped[v.Isp], &SummaryStatistic{
			DestIP:                v.DestIP,
			Region:                v.Region,
			Isp:                   v.Isp,
			TotalSent:             v.TotalSent,
			TotalRecv:             v.TotalRecv,
			MinRtt:                v.MinRtt,
			MaxRtt:                v.MaxRtt,
			AvgRtt:                v.AvgRtt,
			MinRttAvg:             v.MinRttAvg,
			MaxRttAvg:             v.MaxRttAvg,
			LastUpdated:           v.LastUpdated,
			PacketLoss:            v.PacketLoss,
			PacketsRecvDuplicates: v.PacketsRecvDuplicates,
		})
	}

	var result []*SummaryStatistic

	// 对每个 ISP 内部做排序
	for _, list := range grouped {
		sort.Slice(list, func(i, j int) bool {
			var less bool
			switch field {
			case "loss":
				less = list[i].PacketLoss < list[j].PacketLoss
			case "rtt":
				less = list[i].AvgRtt < list[j].AvgRtt
			case "sent":
				less = list[i].TotalSent < list[j].TotalSent
			case "recv":
				less = list[i].TotalRecv < list[j].TotalRecv
			default:
				less = list[i].PacketLoss < list[j].PacketLoss
			}
			if descending {
				return !less
			}
			return less
		})

		// 按原样拼接回总结果
		result = append(result, list...)
	}
	return result
}

// GetLossOnlyGroupedByIspSorted 返回按 ISP 分组并排序后的丢包率不为 0 的数据
func (s *PingStatsStore) GetLossOnlyGroupedByIspSorted(sum []*SummaryStatistic, field string, descending bool) []*SummaryStatistic {

	var lossOnly []*SummaryStatistic
	for _, v := range sum {
		if v.PacketLoss > 0 {
			lossOnly = append(lossOnly, v)
		}
	}

	// 排序
	sort.Slice(lossOnly, func(i, j int) bool {
		var less bool
		switch field {
		case "rtt":
			less = lossOnly[i].AvgRtt < lossOnly[j].AvgRtt
		case "sent":
			less = lossOnly[i].TotalSent < lossOnly[j].TotalSent
		case "recv":
			less = lossOnly[i].TotalRecv < lossOnly[j].TotalRecv
		default:
			less = lossOnly[i].PacketLoss < lossOnly[j].PacketLoss // 默认按丢包
		}
		if descending {
			return !less
		}
		return less
	})

	return lossOnly
}

// 打印排序后结果
func printSummaryList(summaryList []*SummaryStatistic) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{
		"目标IP", "地区", "运营商",
		"发", "收", "丢包%", "重传",
		"MinRTT", "MaxRTT", "AvgRTT", "更新时间",
	})
	table.SetAutoFormatHeaders(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetBorder(false)
	table.SetRowLine(false)
	table.SetColumnSeparator(" ")
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

	formatDuration := func(d time.Duration) string {
		ms := float64(d) / float64(time.Millisecond)
		return fmt.Sprintf("%.1fms", ms)
	}

	// ANSI 颜色码
	green := "\x1b[32m"
	yellow := "\x1b[33m"
	red := "\x1b[31m"
	reset := "\x1b[0m"
	blue := "\033[38;2;0;180;255m"
	purple := "\033[38;2;144;86;255m"
	green2 := "\033[38;2;57;255;20m"

	colorForPacketLoss := func(loss float64) string {
		switch {
		case loss < 5.0:
			return green
		case loss < 10.0:
			return yellow
		default:
			return red
		}
	}

	colorForISP := func(isp string) string {
		switch {
		case isp == "联通":
			return blue
		case isp == "移动":
			return purple
		case isp == "电信":
			return green2
		default:
			return blue
		}
	}
	totalSent, totalRecv := 0, 0
	totalLoss := 0.0
	totalDuplicates := 0
	rttCount := 0
	globalMinRtt := time.Duration(0)
	globalMaxRtt := time.Duration(0)
	globalAvgRtt := time.Duration(0)

	for _, sum := range summaryList {
		totalSent += sum.TotalSent
		totalRecv += sum.TotalRecv
		totalLoss += sum.PacketLoss
		totalDuplicates += sum.PacketsRecvDuplicates
		rttCount += sum.TotalRecv

		globalMinRtt += sum.MinRtt * time.Duration(sum.TotalRecv)
		globalMaxRtt += sum.MaxRtt * time.Duration(sum.TotalRecv)
		globalAvgRtt += sum.AvgRtt * time.Duration(sum.TotalRecv)

		ispColor := colorForISP(sum.Isp)
		coloredIsp := fmt.Sprintf("%s%s%s", ispColor, sum.Isp, reset)
		lossColor := colorForPacketLoss(sum.PacketLoss)
		lossStr := fmt.Sprintf("%.1f%%", sum.PacketLoss)
		lossColored := fmt.Sprintf("%s%s%s", lossColor, lossStr, reset)

		table.Append([]string{
			sum.DestIP,
			sum.Region,
			coloredIsp,
			fmt.Sprintf("%d", sum.TotalSent),
			fmt.Sprintf("%d", sum.TotalRecv),
			lossColored,
			fmt.Sprintf("%d", sum.PacketsRecvDuplicates),
			formatDuration(sum.MinRtt),
			formatDuration(sum.MaxRtt),
			formatDuration(sum.AvgRtt),
			sum.LastUpdated.Format("15:04:05"),
		})
	}

	var avgLoss float64
	if len(summaryList) > 0 {
		avgLoss = totalLoss / float64(len(summaryList))
	}

	if rttCount > 0 {
		globalMinRtt /= time.Duration(rttCount)
		globalMaxRtt /= time.Duration(rttCount)
		globalAvgRtt /= time.Duration(rttCount)
	}

	table.SetFooter([]string{
		"", "", "总计",
		fmt.Sprintf("%d", totalSent),
		fmt.Sprintf("%d", totalRecv),
		fmt.Sprintf("%.1f%%", avgLoss),
		fmt.Sprintf("%d", totalDuplicates),
		formatDuration(globalMinRtt),
		formatDuration(globalMaxRtt),
		formatDuration(globalAvgRtt),
		"",
	})

	table.Render()
}

var JsonData string = `{
    "电信": {
        "北京": {
            "IPv4": [
                "219.141.136.10",
                "219.141.140.10"
            ]
        },
        "上海": {
            "IPv4": [
                "202.96.209.133",
                "116.228.111.118",
                "202.96.209.5",
                "180.168.255.118",
                "203.62.139.69"
            ]
        },
        "天津": {
            "IPv4": [
                "219.150.32.132",
                "219.146.0.132"
            ]
        },
        "重庆": {
            "IPv4": [
                "61.128.192.68",
                "61.128.128.68"
            ]
        },
        "安徽": {
            "IPv4": [
                "61.132.163.68",
                "202.102.213.68",
                "202.102.192.68"
            ]
        },
        "福建": {
            "IPv4": [
                "218.85.152.99",
                "218.85.157.99"
            ]
        },
        "甘肃": {
            "IPv4": [
                "202.100.64.68",
                "61.178.0.93"
            ]
        },
        "广东": {
            "IPv4": [
                "202.96.128.86",
                "202.96.128.166",
                "202.96.134.133",
                "202.96.128.68",
                "202.96.154.8",
                "202.96.154.15"
            ]
        },
        "广西": {
            "IPv4": [
                "202.103.225.68",
                "202.103.224.68"
            ]
        },
        "贵州": {
            "IPv4": [
                "202.98.192.67",
                "202.98.198.167"
            ]
        },
        "河南": {
            "IPv4": [
                "222.88.88.88",
                "222.85.85.85",
                "219.150.150.150",
                "222.88.93.126"
            ]
        },
        "黑龙江": {
            "IPv4": [
                "219.147.198.230",
                "219.147.198.242",
                "112.100.100.100"
            ]
        },
        "湖北": {
            "IPv4": [
                "202.103.24.68",
                "202.103.0.68",
                "202.103.44.150"
            ]
        },
        "湖南": {
            "IPv4": [
                "59.51.78.211",
                "59.51.78.210",
                "222.246.129.80",
                "222.246.129.81"
            ]
        },
        "江苏": {
            "IPv4": [
                "218.2.2.2",
                "218.4.4.4",
                "61.147.37.1",
                "218.2.135.1"
            ]
        },
        "江西": {
            "IPv4": [
                "202.101.224.69",
                "202.101.226.68",
                "202.101.226.69"
            ]
        },
        "内蒙古": {
            "IPv4": [
                "219.148.162.31",
                "222.74.39.50",
                "222.74.1.200"
            ]
        },
        "山东": {
            "IPv4": [
                "219.146.1.66",
                "219.147.1.66"
            ]
        },
        "山西": {
            "IPv4": [
                "59.49.49.49"
            ]
        },
        "陕西": {
            "IPv4": [
                "218.30.19.40",
                "61.134.1.4"
            ]
        },
        "四川": {
            "IPv4": [
                "61.139.2.69",
                "218.6.200.139"
            ]
        },
        "云南": {
            "IPv4": [
                "222.172.200.68",
                "61.166.150.123"
            ]
        },
        "浙江": {
            "IPv4": [
                "202.101.172.35",
                "202.101.172.47",
                "61.153.81.75",
                "61.153.177.196",
                "60.191.134.206",
                "60.191.244.5"
            ]
        },
        "河北": {
            "IPv4": [
                "222.222.202.202"
            ]
        },
        "海南": {
            "IPv4": [
                "202.100.192.68"
            ]
        },
        "辽宁": {
            "IPv4": [
                "219.148.204.66"
            ]
        },
        "吉林": {
            "IPv4": [
                "219.149.194.55"
            ]
        },
        "新疆": {
            "IPv4": [
                "61.128.114.167"
            ]
        }
    },
    "联通": {
        "北京": {
            "IPv4": [
                "123.123.123.123",
                "123.123.123.124",
                "202.106.0.20",
                "202.106.195.68"
            ]
        },
        "上海": {
            "IPv4": [
                "210.22.70.3",
                "210.22.84.3",
                "210.22.70.225"
            ]
        },
        "天津": {
            "IPv4": [
                "202.99.104.68",
                "202.99.96.68"
            ]
        },
        "重庆": {
            "IPv4": [
                "221.5.203.98",
                "221.7.92.98"
            ]
        },
        "广东": {
            "IPv4": [
                "210.21.196.6",
                "221.5.88.88",
                "210.21.4.130"
            ]
        },
        "河北": {
            "IPv4": [
                "202.99.160.68",
                "202.99.166.4"
            ]
        },
        "河南": {
            "IPv4": [
                "202.102.224.68",
                "202.102.227.68"
            ]
        },
        "黑龙江": {
            "IPv4": [
                "202.97.224.69",
                "202.97.224.68"
            ]
        },
        "吉林": {
            "IPv4": [
                "202.98.0.68",
                "202.98.5.68"
            ]
        },
        "江苏": {
            "IPv4": [
                "221.6.4.66",
                "221.6.4.67",
                "58.240.57.33"
            ]
        },
        "内蒙古": {
            "IPv4": [
                "202.99.224.68",
                "202.99.224.8"
            ]
        },
        "山东": {
            "IPv4": [
                "202.102.128.68",
                "202.102.152.3",
                "202.102.134.68",
                "202.102.154.3"
            ]
        },
        "山西": {
            "IPv4": [
                "202.99.192.66",
                "202.99.192.68",
                "202.97.131.178"
            ]
        },
        "陕西": {
            "IPv4": [
                "221.11.1.67",
                "221.11.1.68"
            ]
        },
        "四川": {
            "IPv4": [
                "119.6.6.6",
                "124.161.87.155"
            ]
        },
        "浙江": {
            "IPv4": [
                "221.12.1.227",
                "221.12.33.227",
                "221.12.65.227"
            ]
        },
        "辽宁": {
            "IPv4": [
                "202.96.69.38",
                "202.96.64.68"
            ]
        },
        "贵州": {
            "IPv4": [
                "221.13.30.242"
            ]
        },
        "甘肃": {
            "IPv4": [
                "221.7.34.11"
            ]
        },
        "宁夏": {
            "IPv4": [
                "221.199.12.157"
            ]
        },
        "江西": {
            "IPv4": [
                "220.248.192.12"
            ]
        },
        "广西": {
            "IPv4": [
                "221.7.128.68"
            ]
        },
        "西藏": {
            "IPv4": [
                "221.13.65.34"
            ]
        },
        "海南": {
            "IPv4": [
                "221.11.132.2"
            ]
        },
        "湖南": {
            "IPv4": [
                "58.20.127.238"
            ]
        },
        "湖北": {
            "IPv4": [
                "218.104.111.122"
            ]
        },
        "安徽": {
            "IPv4": [
                "218.104.78.2",
                "58.242.2.2"
            ]
        },
        "福建": {
            "IPv4": [
                "218.104.128.106"
            ]
        },
        "新疆": {
            "IPv4": [
                "221.7.1.20"
            ]
        },
        "云南": {
            "IPv4": [
                "221.3.131.11"
            ]
        }
    },
    "移动": {
        "北京": {
            "IPv4": [
                "211.138.30.66",
                "211.136.17.107",
                "211.136.28.231",
                "211.136.28.234",
                "211.136.28.237",
                "211.136.28.228",
                "221.130.32.103",
                "221.130.32.100",
                "221.130.32.106",
                "221.130.32.109",
                "221.176.3.70",
                "221.176.3.73",
                "221.176.3.76",
                "221.176.3.79",
                "221.176.3.83",
                "221.176.3.85",
                "221.176.4.6",
                "221.176.4.9",
                "221.176.4.12",
                "221.176.4.15",
                "221.176.4.18",
                "221.176.4.21",
                "221.130.33.52",
                "221.179.155.193"
            ]
        },
        "上海": {
            "IPv4": [
                "211.136.112.50",
                "211.136.150.66",
                "211.136.18.171"
            ]
        },
        "天津": {
            "IPv4": [
                "211.137.160.50",
                "211.137.160.185"
            ]
        },
        "重庆": {
            "IPv4": [
                "218.201.4.3",
                "218.201.21.132",
                "218.201.17.2"
            ]
        },
        "安徽": {
            "IPv4": [
                "211.138.180.2",
                "211.138.180.3"
            ]
        },
        "山东": {
            "IPv4": [
                "218.201.96.130",
                "211.137.191.26",
                "218.201.124.18",
                "218.201.124.19"
            ]
        },
        "山西": {
            "IPv4": [
                "211.138.106.2",
                "211.138.106.3",
                "211.138.106.18",
                "211.138.106.19",
                "211.138.106.7"
            ]
        },
        "江苏": {
            "IPv4": [
                "221.131.143.69",
                "112.4.0.55",
                "221.130.13.133",
                "211.103.55.50",
                "221.130.56.241",
                "211.103.13.101",
                "211.138.200.69"
            ]
        },
        "浙江": {
            "IPv4": [
                "211.140.13.188",
                "211.140.188.188",
                "211.140.10.2"
            ]
        },
        "湖南": {
            "IPv4": [
                "211.142.210.98",
                "211.142.210.99",
                "211.142.210.100",
                "211.142.210.101",
                "211.142.211.124",
                "211.142.236.87"
            ]
        },
        "湖北": {
            "IPv4": [
                "211.137.58.20",
                "211.137.64.163"
            ]
        },
        "江西": {
            "IPv4": [
                "211.141.90.68",
                "211.141.90.69",
                "211.141.85.68"
            ]
        },
        "陕西": {
            "IPv4": [
                "211.137.130.3",
                "211.137.130.19",
                "218.200.6.139"
            ]
        },
        "四川": {
            "IPv4": [
                "211.137.82.4",
                "211.137.96.205"
            ]
        },
        "广东": {
            "IPv4": [
                "211.136.20.203",
                "211.136.20.204",
                "211.136.192.6",
                "211.139.136.68",
                "211.139.163.6",
                "120.196.165.24"
            ]
        },
        "广西": {
            "IPv4": [
                "211.138.245.180",
                "211.136.17.108",
                "211.138.240.100"
            ]
        },
        "贵州": {
            "IPv4": [
                "211.139.5.29",
                "211.139.5.30"
            ]
        },
        "福建": {
            "IPv4": [
                "211.138.151.161",
                "211.138.156.66",
                "218.207.217.241",
                "218.207.217.242",
                "211.143.181.178",
                "211.143.181.179",
                "218.207.128.4",
                "218.207.130.118",
                "211.138.145.194"
            ]
        },
        "河北": {
            "IPv4": [
                "211.143.60.56",
                "211.138.13.66",
                "111.11.1.1"
            ]
        },
        "河南": {
            "IPv4": [
                "211.138.24.66"
            ]
        },
        "甘肃": {
            "IPv4": [
                "218.203.160.194",
                "218.203.160.195",
                "211.139.80.6"
            ]
        },
        "黑龙江": {
            "IPv4": [
                "211.137.241.34",
                "211.137.241.35",
                "218.203.59.216"
            ]
        },
        "吉林": {
            "IPv4": [
                "211.141.16.99",
                "211.141.0.99"
            ]
        },
        "辽宁": {
            "IPv4": [
                "211.137.32.178",
                "211.140.197.58"
            ]
        },
        "云南": {
            "IPv4": [
                "211.139.29.68",
                "211.139.29.69",
                "211.139.29.150",
                "211.139.29.170",
                "218.202.1.166"
            ]
        },
        "海南": {
            "IPv4": [
                "221.176.88.95",
                "211.138.164.6"
            ]
        },
        "内蒙古": {
            "IPv4": [
                "211.138.91.1",
                "211.138.91.2"
            ]
        },
        "新疆": {
            "IPv4": [
                "218.202.152.130",
                "218.202.152.131"
            ]
        },
        "西藏": {
            "IPv4": [
                "211.139.73.34",
                "211.139.73.35",
                "211.139.73.50"
            ]
        },
        "青海": {
            "IPv4": [
                "211.138.75.123"
            ]
        },
        "宁夏": {
            "IPv4": [
                "218.203.123.116"
            ]
        },
        "香港": {
            "IPv4": [
                "203.142.100.18",
                "203.142.100.21"
            ]
        }
    }
}`
