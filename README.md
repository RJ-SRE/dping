# dping

例子

```
sudo go run ./main.go -h

  -C int
    	指定并发ping数量 (default 50)
  -S string
    	指定排序类型|loss|minrtt|maxrtt|avgrtt (default "loss")
  -des
    	指定排序|升序ture|降序false｜“类型
  -dt string
    	指定检测区域默认全国 (default "全国")
  -eth string
    	指定发包网卡 (default "nil")
  -isp string
    	指定运营商 (default "all")
  -p int
    	指定发包数量 (default 3)
```

### 可以根据不同的系统进行编译执行

例如：`GOOS=linux GOARCH=amd64 go build -o dping main.go`

执行例图

<img src="/Users/h/Library/Application Support/typora-user-images/image-20250730162758499.png" alt="image-20250730162758499" style="zoom:50%;" />

<img src="/Users/h/Library/Application Support/typora-user-images/image-20250730162843216.png" alt="image-20250730162843216" style="zoom:50%;" />
