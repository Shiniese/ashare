# ashare

> Go 版 A 股日线行情库：多数据源自动降级 + 统一前复权。

## 简介

`ashare` 是一个用 Go 实现的 A 股日线（OHLCV）行情获取库。它同时从多个公开数据源拉取数据，按「封 IP 风险」由低到高自动降级，并在客户端用同一套**前复权（前复权）模型**统一处理，保证无论命中哪个源，返回的前复权价都 1:1 一致。

- **多源降级**：tencent → mootdx → eastmoney → baostock → akshare → tushare
- **统一前复权**：所有源返回的均为原始价（不复权），再用 TDX 的 XDXR 分红送配事件集中做前复权
- **限流保护**：Eastmoney / akshare 共享进程级节流闸门，自动加抖动避免同步打爆
- **灵活配置**：可用 `Use` 自定义数据源链，用 `WithHTTP*` 调整超时、间隔、重试

## 安装

```bash
go get github.com/Shiniese/ashare
```

## 快速开始

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Shiniese/ashare"
)

func main() {
	c := ashare.New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 取贵州茅台 2023 全年日线（前复权）
	bars, src, err := c.Daily(ctx, "600519.SH", "2023-01-01", "2023-12-31")
	if err != nil {
		// 所有源都失败时返回 *ashare.NoAvailableSourceError，含各源失败原因
		panic(err)
	}
	fmt.Printf("served by: %s
", src)
	fmt.Printf("first bar: %v open=%.2f close=%.2f
",
		bars[0].Date.Format("2006-01-02"), bars[0].Open, bars[0].Close)
}
```

## 符号格式

`ParseSymbol` 接受多种写法，并会按 Tushare 约定推断交易所（5/6/9 → SH，0/2/3 → SZ，4/8 → BJ）：

| 输入           | 解析结果       |
| -------------- | -------------- |
| `600519.SH`    | 沪市 贵州茅台  |
| `SH600519`     | 沪市 贵州茅台  |
| `sh.600519`    | 沪市 贵州茅台  |
| `000001`       | 深市 平安银行  |

## 自定义数据源

```go
c := ashare.New()
// 只使用腾讯和新浪相邻的源，剔除不稳定的
if err := c.Use(ashare.NewTencent(), ashare.NewEastmoney()); err != nil {
	panic(err)
}

// 注入测试的假分红事件源
c.UseEvents(func(ctx context.Context, sym ashare.Symbol) ([]ashare.DividendEvent, error) {
	return nil, nil
})
```

HTTP 类源（eastmoney / akshare / tushare）支持通过 `Option` 配置：

```go
ashare.NewEastmoney(
	ashare.WithHTTPTimeout(20*time.Second), // 默认 15s
	ashare.WithMinInterval(2*time.Second),  // 默认 1s，0 关闭节流
)
ashare.NewTushare(token,
	ashare.WithRateLimitBackoff(10*time.Second), // 限流重试档位
)
```

## 数据源说明

| 名称      | 协议/接口                         | 鉴权 | 备注                               |
| --------- | --------------------------------- | ---- | ---------------------------------- |
| tencent   | 腾讯财经 HTTP                     | 无   | 默认链首选                         |
| mootdx    | 通达信二进制 TCP（gotdx）         | 无   | 不受 HTTP 限流/封 IP 影响；不支持北交所 |
| eastmoney | 东方财富 push2his HTTP            | 无   | 与 akshare 共享节流桶              |
| baostock  | Baostock HTTP                     | 无   |                                    |
| akshare   | 复刻 AKShare `stock_zh_a_hist`    | 无   | 与 eastmoney 同接口、同节流桶      |
| tushare   | Tushare API                       | Token | 需设置 `TUSHARE_TOKEN`，否则自动跳过 |

## 前复权模型

所有数据源返回原始价，由客户端用同一个线性模型做前复权（与通达信 / 腾讯 qfq 表一致，已对真实 TDX 服务器验证，1500 天最大误差 0.00）：

```
qfq(d) = raw(d) · M(d) − N(d)
M(d) = Π (1 + 送转_e + 配股_e)     对所有 e > d
N(d) = Σ B_e · Π (1 + 送转 + 配股) 对所有 e > d，B_e = 红利_e − 配股价_e·配股_e
```

复权因子来自 TDX 的 XDXR 分红送配事件（按 24h 缓存），因此不同源之间结果可逐日对齐。

## 返回结构

```go
type Bar struct {
	Date   time.Time // 交易日
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64 // 成交量（股）
	Amount float64 // 成交额（元），源未提供时为 0
}
```

## 许可证

见仓库 LICENSE。
