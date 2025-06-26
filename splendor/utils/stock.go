package utils

type StockInfo struct {
	TileRange   [2]int
	Price       int
	BonusFirst  int
	BonusSecond int
}

// 定义三种通用配置
var premiumStock = []StockInfo{
	{[2]int{0, 0}, 0, 0, 0},
	{[2]int{2, 2}, 400, 4000, 2000},
	{[2]int{3, 3}, 500, 5000, 2500},
	{[2]int{4, 4}, 600, 6000, 3000},
	{[2]int{5, 5}, 700, 7000, 3500},
	{[2]int{6, 10}, 800, 8000, 4000},
	{[2]int{11, 20}, 900, 9000, 4500},
	{[2]int{21, 30}, 1000, 10000, 5000},
	{[2]int{31, 40}, 1100, 11000, 5500},
	{[2]int{41, 1000}, 1200, 12000, 6000},
}

var mediumStock = []StockInfo{
	{[2]int{0, 0}, 0, 0, 0},
	{[2]int{2, 2}, 300, 3000, 1500},
	{[2]int{3, 3}, 400, 4000, 2000},
	{[2]int{4, 4}, 500, 5000, 2500},
	{[2]int{5, 5}, 600, 6000, 3000},
	{[2]int{6, 10}, 700, 7000, 3500},
	{[2]int{11, 20}, 800, 8000, 4000},
	{[2]int{21, 30}, 900, 9000, 4500},
	{[2]int{31, 40}, 1000, 10000, 5000},
	{[2]int{41, 1000}, 1100, 11000, 5500},
}

var lowStock = []StockInfo{
	{[2]int{0, 0}, 0, 0, 0},
	{[2]int{2, 2}, 200, 2000, 1000},
	{[2]int{3, 3}, 300, 3000, 1500},
	{[2]int{4, 4}, 400, 4000, 2000},
	{[2]int{5, 5}, 500, 5000, 2500},
	{[2]int{6, 10}, 600, 6000, 3000},
	{[2]int{11, 20}, 700, 7000, 3500},
	{[2]int{21, 30}, 800, 8000, 4000},
	{[2]int{31, 40}, 900, 9000, 4500},
	{[2]int{41, 1000}, 1000, 10000, 5000},
}

// 公司 -> 配置映射表
var StockData = map[string][]StockInfo{
	"Continental": premiumStock,
	"Imperial":    premiumStock,

	"American":  mediumStock,
	"Festival":  mediumStock,
	"Worldwide": mediumStock,

	"Tower":   lowStock,
	"Sackson": lowStock,
}

func GetStockInfo(company string, tileCount int) *StockInfo {
	for _, info := range StockData[company] {
		if tileCount >= info.TileRange[0] && tileCount <= info.TileRange[1] {
			return &info
		}
	}
	return nil
}
