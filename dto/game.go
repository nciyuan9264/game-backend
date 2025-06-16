package dto

import "github.com/gorilla/websocket"

type RoomStatus string

// 玩家连接对象结构体
type PlayerConn struct {
	PlayerID string
	Conn     *websocket.Conn
	Online   bool // 新增：标记是否在线
}

const (
	RoomStatusWaiting          RoomStatus = "waiting"          // 等待玩家加入房间
	RoomStatusSetTile          RoomStatus = "setTile"          //等待玩家放置Tile
	RoomStatusCreateCompany    RoomStatus = "createCompany"    // 等待玩家创建公司
	RoomStatusBuyStock         RoomStatus = "buyStock"         // 等待玩家购买股票
	RoomStatusMerging          RoomStatus = "merging"          // 等待玩家选择并购公司
	RoomStatusMergingSelection RoomStatus = "mergingSelection" // 选择并购留下来的公司
	RoomStatusMergingSettle    RoomStatus = "mergingSettle"    // 结算并购
	RoomStatusEnd              RoomStatus = "end"
)

type SettleData struct {
	Hoders    []string       `json:"hoders"`
	Dividends map[string]int `json:"dividends"`
}

type MergingSettleItem struct {
	Company        string  `json:"company"`
	SellAmount     float64 `json:"sellAmount"`
	ExchangeAmount float64 `json:"exchangeAmount"`
}

type Company struct {
	Name       string `json:"name"`
	StockPrice int    `json:"stockPrice"`
	StockTotal int    `json:"stockTotal"`
	Tiles      int    `json:"tiles"`
}
