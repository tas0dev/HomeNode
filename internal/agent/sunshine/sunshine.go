package sunshine

import (
 "context"
 "net"
 "strconv"
 "time"
 "github.com/tas0dev/HomeNode/internal/protocol"
)

func Probe(ctx context.Context,port int,lan,tailnet string)protocol.Sunshine {
 s:=protocol.Sunshine{State:"UNKNOWN",LAN:lan,Tailnet:tailnet}
 running,known:=processRunning(ctx)
 if !known{return s}
 if !running{s.State="STOPPED";return s}
 s.State="RUNNING"
 d:=net.Dialer{Timeout:300*time.Millisecond}
 c,e:=d.DialContext(ctx,"tcp",net.JoinHostPort("127.0.0.1",strconv.Itoa(port)))
 if e==nil{c.Close();s.State="READY"}
 return s
}
