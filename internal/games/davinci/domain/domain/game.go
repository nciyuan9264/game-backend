package domain

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type RealConn struct {
	*websocket.Conn
	PingInterval time.Duration
	LastPongTime time.Time
	Done         chan struct{}
	closeOnce    sync.Once
}

func (r *RealConn) WriteMessage(messageType int, data []byte) error {
	return r.Conn.WriteMessage(messageType, data)
}

func (r *RealConn) Close() error {
	r.closeOnce.Do(func() {
		close(r.Done)
		r.Conn.Close()
	})
	return nil
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
				log.Println("✅ 发送心跳包")
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

func NewRealConn(conn *websocket.Conn) *RealConn {
	return &RealConn{
		Conn:         conn,
		PingInterval: 30 * time.Second,
		LastPongTime: time.Now(),
		Done:         make(chan struct{}),
	}
}

// 玩家连接对象结构体
type RoomStatus string

const (
	RoomStatusMatch     RoomStatus = "match"     // 初始化房间状态
	RoomStatusWaiting   RoomStatus = "waiting"   // 等待玩家加入房间
	RoomStatusGetCard   RoomStatus = "getCard"   // 等待玩家获取牌
	RoomStatusGuessCard RoomStatus = "guessCard" // 等待玩家猜牌
	RoomStatusSetCard   RoomStatus = "setCard"   // 等待玩家设置牌
	RoomStatusEnd       RoomStatus = "end"
)
