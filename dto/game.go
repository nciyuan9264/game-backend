package dto

type RoomStatus string

const (
	RoomStatusWaiting       RoomStatus = "waiting"
	RoomStatusCreateCompany RoomStatus = "createCompany"
	RoomStatusBuyStock      RoomStatus = "buyStock"
	RoomStatusMerging       RoomStatus = "创建公司状态"
	RoomStatusGameOver      RoomStatus = "游戏结束"
)

type Company struct {
	Name       string `json:"name"`
	StockPrice int    `json:"stockPrice"`
	StockTotal int    `json:"stockTotal"`
	Tiles      int    `json:"tiles"`
	Valuation  int    `json:"valuation"`
}
