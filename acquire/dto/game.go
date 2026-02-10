package dto

import (
	"time"

	"github.com/gorilla/websocket"
)

type RoomStatus string

type ConnInterface interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
}

type RealConn struct {
	*websocket.Conn
	PingInterval time.Duration
	LastPongTime time.Time
	Done         chan struct{}
}

func (r *RealConn) WriteMessage(messageType int, data []byte) error {
	return r.Conn.WriteMessage(messageType, data)
}

func (r *RealConn) Close() error {
	close(r.Done)
	return r.Conn.Close()
}

func NewRealConn(conn *websocket.Conn) *RealConn {
	return &RealConn{
		Conn:         conn,
		PingInterval: 30 * time.Second,
		LastPongTime: time.Now(),
		Done:         make(chan struct{}),
	}
}

func (r *RealConn) StartHeartbeat() {
	go func() {
		ticker := time.NewTicker(r.PingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if time.Since(r.LastPongTime) > r.PingInterval*2 {
					r.Close()
					return
				}
				r.WriteMessage(websocket.PingMessage, []byte{})
			case <-r.Done:
				return
			}
		}
	}()

	r.Conn.SetPongHandler(func(string) error {
		r.LastPongTime = time.Now()
		return nil
	})
}

// 玩家连接对象结构体

const (
	RoomStatusMatch            RoomStatus = "match"            // 初始化房间状态
	RoomStatusWaiting          RoomStatus = "waiting"          // 等待玩家加入房间
	RoomStatusSetTile          RoomStatus = "setTile"          // 等待玩家放置Tile
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
