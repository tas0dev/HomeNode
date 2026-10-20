package protocol

import (
 "encoding/json"
 "errors"
 "math"
 "time"
)

const Version = 1
const MaxMessage = 64 << 10

type Message struct {
 Version int `json:"version"`
 Type string `json:"type"`
 AgentID string `json:"agent_id"`
 Timestamp time.Time `json:"timestamp"`
 RequestID string `json:"request_id,omitempty"`
 Payload json.RawMessage `json:"payload"`
}
func New(kind, id, request string, value any) Message {
 b, _ := json.Marshal(value)
 return Message{Version, kind, id, time.Now().UTC(), request, b}
}
func (m Message) Validate(id string) error {
 if m.Version != Version || m.AgentID != id || m.Timestamp.IsZero() || len(m.RequestID)>128 || len(m.Payload)==0 || time.Since(m.Timestamp)>time.Minute || time.Until(m.Timestamp)>time.Minute { return errors.New("invalid envelope") }
 return nil
}
type CPU struct {
 Usage *float64 `json:"usage_percent"`
 Temperature *float64 `json:"temperature_c"`
 Clock *float64 `json:"clock_mhz"`
 Model string `json:"model"`
}
type GPU struct {
 Usage *float64 `json:"usage_percent"`
 Temperature *float64 `json:"temperature_c"`
 Clock *float64 `json:"clock_mhz"`
 VRAMUsed *float64 `json:"vram_used_bytes"`
 VRAMTotal *float64 `json:"vram_total_bytes"`
 Power *float64 `json:"power_w"`
 Model string `json:"model"`
}
type Memory struct {
 Used *float64 `json:"used_bytes"`
 Total *float64 `json:"total_bytes"`
 Available *float64 `json:"available_bytes"`
 Usage *float64 `json:"usage_percent"`
}
type Network struct {
 Interface string `json:"interface"`
 IPv4 string `json:"ipv4"`
 LinkSpeed *float64 `json:"link_speed_bps"`
 RX *float64 `json:"rx_bytes_per_sec"`
 TX *float64 `json:"tx_bytes_per_sec"`
 RXTotal *float64 `json:"rx_total_bytes"`
 TXTotal *float64 `json:"tx_total_bytes"`
}
type Sample struct {
 Timestamp time.Time `json:"timestamp"`
 ReceivedAt time.Time `json:"received_at"`
 CPU CPU `json:"cpu"`
 GPU GPU `json:"gpu"`
 Memory Memory `json:"memory"`
 Network Network `json:"network"`
 Uptime *float64 `json:"uptime_seconds"`
 Wifi string `json:"wifi"`
 Availability map[string]string `json:"availability,omitempty"`
}
type Sunshine struct {
 State string `json:"state"`
 LAN string `json:"lan_address"`
 Tailnet string `json:"tailnet_address"`
}
type Command struct { Operation string `json:"operation"` }
type Ack struct { Accepted bool `json:"accepted"`; Reason string `json:"reason,omitempty"` }
func Number(v float64) *float64 { if math.IsNaN(v)||math.IsInf(v,0)||v<0 {return nil}; return &v }
func clean(p **float64, max float64) { if *p!=nil && (math.IsNaN(**p)||math.IsInf(**p,0)||**p<0||**p>max) { *p=nil } }
func (s *Sample) Sanitize() {
 for _, p := range []**float64{&s.CPU.Usage,&s.GPU.Usage,&s.Memory.Usage} {clean(p,100)}
 for _, p := range []**float64{&s.CPU.Temperature,&s.GPU.Temperature} {clean(p,150)}
 for _, p := range []**float64{&s.CPU.Clock,&s.GPU.Clock,&s.GPU.VRAMUsed,&s.GPU.VRAMTotal,&s.GPU.Power,&s.Memory.Used,&s.Memory.Total,&s.Memory.Available,&s.Network.RX,&s.Network.TX,&s.Network.RXTotal,&s.Network.TXTotal,&s.Network.LinkSpeed,&s.Uptime} {clean(p,math.MaxFloat64)}
 if s.Memory.Total!=nil && s.Memory.Used!=nil && *s.Memory.Used>*s.Memory.Total {s.Memory.Used=nil; s.Memory.Usage=nil}
 if s.GPU.VRAMTotal!=nil && s.GPU.VRAMUsed!=nil && *s.GPU.VRAMUsed>*s.GPU.VRAMTotal {s.GPU.VRAMUsed=nil}
}
