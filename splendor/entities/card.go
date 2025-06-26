package entities

const (
	CardStateHidden   = 0 // 未被翻开，牌堆中
	CardStateRevealed = 1 // 翻开在桌面，可被购买
	CardStateBought   = 2 // 已被某玩家购买
)

type NormalCard struct {
	ID     int            `json:"id"`     // 卡牌ID
	Level  int            `json:"level"`  // 1/2/3
	Bonus  string         `json:"bonus"`  // 折扣颜色：emerald, diamond, sapphire, onyx, ruby
	Points int            `json:"points"` // 荣誉分
	Cost   map[string]int `json:"cost"`   // 五色费用
	State  int            `json:"state"`  // 0: 未被选中, 1: 已被选中
}

type NobleCard struct {
	ID     string         `json:"id"`     // e.g., "N1"
	Cost   map[string]int `json:"cost"`   // 奖励条件，如{"emerald":4,"sapphire":4}
	Points int            `json:"points"` // 固定 3 分
	State  int            `json:"state"`  // 如果有状态管理：0=未分配, 1=已分配
}
