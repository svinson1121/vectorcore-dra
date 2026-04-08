package message

import (
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/svinson1121/vectorcore-dra/internal/diameter/avp"
)

var (
	hopByHopCounter uint32
	endToEndBase    uint32 // high 24 bits = startup time, low 8 bits = counter
	endToEndCounter uint32
)

func init() {
	// Initialize hop-by-hop with a random starting value
	hopByHopCounter = rand.Uint32()

	// End-to-End: high 24 bits = lower 24 bits of unix time at startup
	startTime := uint32(time.Now().Unix()) & 0x00FFFFFF
	endToEndBase = startTime << 8
	endToEndCounter = 0
}

// NextHopByHop returns the next hop-by-hop identifier (atomically incremented).
func NextHopByHop() uint32 {
	return atomic.AddUint32(&hopByHopCounter, 1)
}

// NextEndToEnd returns the next end-to-end identifier per RFC 6733.
// High 24 bits = startup time, low 8 bits = monotonically increasing counter.
func NextEndToEnd() uint32 {
	cnt := atomic.AddUint32(&endToEndCounter, 1) & 0xFF
	return endToEndBase | cnt
}

// Builder constructs a Diameter message fluently.
type Builder struct {
	msg *Message
}

// NewRequest creates a new request message builder.
func NewRequest(commandCode uint32, appID uint32) *Builder {
	b := &Builder{
		msg: &Message{
			Header: Header{
				Version:     1,
				Flags:       FlagRequest | FlagProxiable,
				CommandCode: commandCode,
				AppID:       appID,
				HopByHop:    NextHopByHop(),
				EndToEnd:    NextEndToEnd(),
			},
		},
	}
	return b
}

// NewAnswer creates a new answer message builder, copying identifiers from the request.
// The R flag is cleared; P flag is preserved from request.
func NewAnswer(req *Message) *Builder {
	flags := req.Header.Flags &^ FlagRequest // clear R bit
	flags &^= FlagRetransmit                 // clear T bit on answers
	b := &Builder{
		msg: &Message{
			Header: Header{
				Version:     1,
				Flags:       flags,
				CommandCode: req.Header.CommandCode,
				AppID:       req.Header.AppID,
				HopByHop:    req.Header.HopByHop,
				EndToEnd:    req.Header.EndToEnd,
			},
		},
	}
	return b
}

// Add appends AVPs to the message.
func (b *Builder) Add(avps ...*avp.AVP) *Builder {
	b.msg.AVPs = append(b.msg.AVPs, avps...)
	return b
}

// NonProxiable clears the Proxiable (P) flag. Required for peer-to-peer messages
// that must not be forwarded: CER/CEA, DWR/DWA, DPR/DPA (RFC 6733 sec 6.1.8).
func (b *Builder) NonProxiable() *Builder {
	b.msg.Header.Flags &^= FlagProxiable
	return b
}

// Build returns the constructed Message.
func (b *Builder) Build() *Message {
	return b.msg
}
