package main

import (
	"dping/internal"
	"flag"
)

func main() {
	detection := flag.String("dt", "全国", "指定检测区域默认全国")
	isp := flag.String("isp", "all", "指定运营商")
	count := flag.Int("p", 3, "指定发包数量")
	eth := flag.String("eth", "nil", "指定发包网卡")
	maxConcurrency := flag.Int("C", 50, "指定并发ping数量")
	sort := flag.String("S", "loss", "指定排序类型|loss|minrtt|maxrtt|avgrtt")
	descending := flag.Bool("des", false, "指定排序|升序ture|降序false｜“类型")

	flag.Parse()
	internal.DPing(*isp, *detection, *maxConcurrency, *count, *eth, *sort, *descending)
}
