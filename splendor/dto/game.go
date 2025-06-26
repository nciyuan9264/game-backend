package dto

import "github.com/gorilla/websocket"

type ConnInterface interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
}
type RealConn struct {
	*websocket.Conn
}

func (r *RealConn) WriteMessage(messageType int, data []byte) error {
	return r.Conn.WriteMessage(messageType, data)
}

func (r *RealConn) Close() error {
	return r.Conn.Close()
}

// 玩家连接对象结构体
type PlayerConn struct {
	PlayerID string
	Conn     ConnInterface
	Online   bool // 新增：标记是否在线
}

type SettleData struct {
	Hoders    []string       `json:"hoders"`
	Dividends map[string]int `json:"dividends"`
}

type MergingSettleItem struct {
	Company        string `json:"company"`
	SellAmount     int    `json:"sellAmount"`
	ExchangeAmount int    `json:"exchangeAmount"`
}

type Company struct {
	Name       string `json:"name"`
	StockPrice int    `json:"stockPrice"`
	StockTotal int    `json:"stockTotal"`
	Tiles      int    `json:"tiles"`
}
