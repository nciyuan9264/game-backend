// virtual_conn.go
package ws

import (
	"fmt"
)

type VirtualConn struct {
	PlayerID string
	RoomID   string
}

func (v *VirtualConn) WriteMessage(messageType int, data []byte) error {
	// log.Printf("[AI:%s] 发送消息到房间 %s: %s\n", v.PlayerID, v.RoomID, string(data))
	MaybeRunAIIfNeeded(v.RoomID, data)
	return nil
}

func (v *VirtualConn) ReadMessage() (messageType int, p []byte, err error) {
	return 0, nil, fmt.Errorf("virtual connection cannot read")
}

func (v *VirtualConn) Close() error {
	return nil
}
