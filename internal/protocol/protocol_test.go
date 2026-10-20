package protocol
import("math";"testing";"time")
func TestSanitize(t *testing.T){negative:=-1.0;high:=101.0;nan:=math.NaN();s:=Sample{CPU:CPU{Usage:&high,Temperature:&nan},Network:Network{RX:&negative}};s.Sanitize();if s.CPU.Usage!=nil||s.CPU.Temperature!=nil||s.Network.RX!=nil{t.Fatal("invalid metrics survived")};zero:=0.0;s.CPU.Usage=&zero;s.Sanitize();if s.CPU.Usage==nil{t.Fatal("valid zero was treated as missing")}}
func TestEnvelope(t *testing.T){m:=New("heartbeat","desktop","",struct{}{});if m.Validate("desktop")!=nil{t.Fatal("valid envelope rejected")};if m.Validate("other")==nil{t.Fatal("wrong agent accepted")};m.Timestamp=time.Now().Add(-2*time.Minute);if m.Validate("desktop")==nil{t.Fatal("stale message accepted")}}
