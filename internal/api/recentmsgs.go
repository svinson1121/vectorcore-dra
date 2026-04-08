package api

import (
	"sync"
	"time"

	"github.com/svinson1121/vectorcore-dra/internal/diameter/message"
)

const recentMsgCap = 20

// RecentMessage is one entry in the recent-messages ring buffer.
type RecentMessage struct {
	Timestamp   time.Time `json:"timestamp"`
	Direction   string    `json:"direction"` // "in" | "out"
	FromPeer    string    `json:"from_peer"`
	ToPeer      string    `json:"to_peer"`
	CommandCode uint32    `json:"command_code"`
	CommandName string    `json:"command_name"`
	AppID       uint32    `json:"app_id"`
	AppName     string    `json:"app_name"`
	IsRequest   bool      `json:"is_request"`
	ResultCode  uint32    `json:"result_code,omitempty"` // 0 = not present / request
	SessionID   string    `json:"session_id,omitempty"`
}

// RecentMsgBuf is a fixed-capacity ring buffer of recent Diameter messages.
// It is safe for concurrent use.
type RecentMsgBuf struct {
	mu  sync.Mutex
	buf [recentMsgCap]RecentMessage
	n   int // total written (use mod for index)
}

// Add appends a message, evicting the oldest when full.
func (r *RecentMsgBuf) Add(m RecentMessage) {
	r.mu.Lock()
	r.buf[r.n%recentMsgCap] = m
	r.n++
	r.mu.Unlock()
}

// RecordMsg satisfies peermgr.RecentMsgRecorder and adds a message to the buffer.
func (r *RecentMsgBuf) RecordMsg(timestamp time.Time, direction, fromPeer, toPeer string, commandCode uint32, appID uint32, isRequest bool, resultCode uint32, sessionID string) {
	r.Add(RecentMessage{
		Timestamp:   timestamp,
		Direction:   direction,
		FromPeer:    fromPeer,
		ToPeer:      toPeer,
		CommandCode: commandCode,
		CommandName: message.CommandName(appID, commandCode),
		AppID:       appID,
		AppName:     recentAppName(appID),
		IsRequest:   isRequest,
		ResultCode:  resultCode,
		SessionID:   sessionID,
	})
}

// Snapshot returns up to recentMsgCap entries, newest first.
func (r *RecentMsgBuf) Snapshot() []RecentMessage {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := r.n
	if count > recentMsgCap {
		count = recentMsgCap
	}
	out := make([]RecentMessage, count)
	for i := 0; i < count; i++ {
		// Walk backwards from newest
		idx := (r.n - 1 - i + recentMsgCap*2) % recentMsgCap
		out[i] = r.buf[idx]
	}
	return out
}

// appIDName returns the short name for a Diameter Application-ID.
// Reuses the same map as the peer response helpers.
func recentAppName(id uint32) string {
	return appIDName(id)
}
