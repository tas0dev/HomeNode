package agent

import (
 "context"
 "encoding/json"
 "errors"
 "math/rand/v2"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "runtime"
 "sync"
 "time"

 "github.com/gorilla/websocket"
 "github.com/shirou/gopsutil/v4/host"
 "github.com/tas0dev/HomeNode/internal/agent/collector"
 "github.com/tas0dev/HomeNode/internal/agent/power"
 "github.com/tas0dev/HomeNode/internal/agent/service"
 "github.com/tas0dev/HomeNode/internal/agent/sunshine"
 "github.com/tas0dev/HomeNode/internal/config"
 "github.com/tas0dev/HomeNode/internal/protocol"
)
type journalEntry struct {At time.Time `json:"at"`;Ack protocol.Ack `json:"ack"`}
type Agent struct {cfg config.Config;token string;seen map[string]journalEntry;collector *collector.Collector}
func New(c config.Config)(*Agent,error){b,e:=os.ReadFile(filepath.Join(c.SecretsDir,"agent.token.dpapi"));if e!=nil{return nil,e};token,e:=service.Unprotect(b);if e!=nil{return nil,e};if len(token)<32||len(token)>1024{return nil,errors.New("invalid enrolled token")};a:=&Agent{cfg:c,token:string(token),seen:map[string]journalEntry{},collector:collector.New(c)}
 b,e=os.ReadFile(filepath.Join(c.SecretsDir,"requests.json"));if e==nil{if json.Unmarshal(b,&a.seen)!=nil{return nil,errors.New("invalid request journal")}}else if !os.IsNotExist(e){return nil,e};return a,nil
}
func(a *Agent) persist()error {
 for id,v:=range a.seen{if time.Since(v.At)>7*24*time.Hour{delete(a.seen,id)}}
 if len(a.seen)>4096{return errors.New("request journal limit reached")};b,e:=json.Marshal(a.seen);if e!=nil{return e};path:=filepath.Join(a.cfg.SecretsDir,"requests.json");f,e:=os.OpenFile(path+".tmp",os.O_WRONLY|os.O_CREATE|os.O_TRUNC,0600);if e!=nil{return e};_,e=f.Write(b);if e==nil{e=f.Sync()};ce:=f.Close();if e!=nil{return e};if ce!=nil{return ce};return os.Rename(path+".tmp",path)
}
func(a *Agent) command(m protocol.Message)protocol.Ack {
 var command protocol.Command
 if m.RequestID==""||json.Unmarshal(m.Payload,&command)!=nil||(command.Operation!="shutdown"&&command.Operation!="restart"){return protocol.Ack{Reason:"invalid_command"}}
 if v,ok:=a.seen[m.RequestID];ok{return v.Ack}
 // Reserve durably before invoking the API. A crash cannot turn a replay into
 // a second shutdown. A crash between reservation and execution remains rejected.
 a.seen[m.RequestID]=journalEntry{At:time.Now().UTC(),Ack:protocol.Ack{Reason:"execution_uncertain"}}
 if a.persist()!=nil{return protocol.Ack{Reason:"journal_unavailable"}}
 ack:=protocol.Ack{Accepted:true};if power.Execute(command.Operation)!=nil{ack=protocol.Ack{Reason:"windows_api_failed"};service.Log("ERROR","電源操作をWindows APIが拒否しました")}
 a.seen[m.RequestID]=journalEntry{At:time.Now().UTC(),Ack:ack};if a.persist()!=nil {service.Log("ERROR","電源操作の結果を保存できませんでした")};return ack
}
func(a *Agent) Run(ctx context.Context)error {
 defer a.collector.Close();service.Log("INFO","HomeNode Agentを起動しました");defer service.Log("INFO","HomeNode Agentを終了します")
 delay:=time.Second
 for{if ctx.Err()!=nil{return nil};started:=time.Now();a.connect(ctx);if ctx.Err()!=nil{return nil};service.Log("WARN","Serverとの接続が切れました。再接続します")
 if time.Since(started)>time.Minute{delay=time.Second};wait:=delay+time.Duration(rand.Int64N(int64(delay/4+1)));timer:=time.NewTimer(wait);select{case <-ctx.Done():timer.Stop();return nil;case <-timer.C:};delay*=2;if delay>a.cfg.ReconnectMax{delay=a.cfg.ReconnectMax}}
}
func(a *Agent) connect(ctx context.Context)error {
 u,_:=url.Parse(a.cfg.ServerURL);u.Scheme="wss";u.Path="/agent/v1/connect";dialer:=websocket.Dialer{HandshakeTimeout:10*time.Second,Proxy:nil}
 conn,res,e:=dialer.DialContext(ctx,u.String(),http.Header{"Authorization":[]string{"Bearer "+a.token}});if e!=nil {if res!=nil&&res.Body!=nil{res.Body.Close()};return errors.New("server connection failed")};defer conn.Close();conn.SetReadLimit(protocol.MaxMessage)
 var mu sync.Mutex;send:=func(kind,id string,v any)error {mu.Lock();defer mu.Unlock();conn.SetWriteDeadline(time.Now().Add(5*time.Second));return conn.WriteJSON(protocol.New(kind,a.cfg.AgentID,id,v))}
 hostname,_:=os.Hostname();boot,_:=host.BootTimeWithContext(ctx)
 if e=send("hello","",map[string]any{"hostname":hostname,"os":runtime.GOOS,"boot_time":boot});e!=nil{return e}
 service.Log("INFO","Serverに接続しました")
 connectionCtx,cancel:=context.WithCancel(ctx);defer cancel();go func(){<-connectionCtx.Done();conn.Close()}()
 incoming:=make(chan protocol.Message,4);readError:=make(chan error,1)
 go func(){for{conn.SetReadDeadline(time.Now().Add(a.cfg.ReachTimeout*2));var m protocol.Message;if e:=conn.ReadJSON(&m);e!=nil {readError<-e;return};if m.Validate(a.cfg.AgentID)!=nil{readError<-errors.New("invalid server envelope");return};select{case incoming<-m:case <-connectionCtx.Done():return}}}()
 heart:=time.NewTicker(a.cfg.Heartbeat);defer heart.Stop();sample:=time.NewTicker(a.cfg.SampleInterval);defer sample.Stop()
 if send("heartbeat","",struct{}{})!=nil{return errors.New("heartbeat failed")}
 lastSunshine:=protocol.Sunshine{};lastProbe:=time.Time{};collectorFailed:=false
 for{select{
 case <-ctx.Done():return nil
 case e:=<-readError:return e
 case m:=<-incoming:switch m.Type {case "heartbeat_ack":case "command":ack:=a.command(m);if send("command_ack",m.RequestID,ack)!=nil{return errors.New("ack delivery failed")};default:return errors.New("unsupported server message")}
 case <-heart.C:if send("heartbeat","",struct{}{})!=nil{return errors.New("heartbeat failed")}
 case <-sample.C:
  v:=a.collector.Collect(connectionCtx)
  failed:=v.CPU.Usage==nil||v.Memory.Total==nil||v.Network.RXTotal==nil||v.Availability["sensors"]!=""
  if failed!=collectorFailed {collectorFailed=failed;level,id,message:="INFO","collector.recovered","監視データの取得が復旧しました";if failed{level,id,message="WARN","collector.unavailable","一部の監視データを取得できません"};service.Log(level,message);if send("event","",map[string]string{"level":level,"event_id":id})!=nil{return errors.New("event delivery failed")}}
  if send("telemetry","",v)!=nil{return errors.New("telemetry failed")}
  if time.Since(lastProbe)>=5*time.Second{lastProbe=time.Now();sun:=sunshine.Probe(connectionCtx,a.cfg.SunshinePort,v.Network.IPv4,a.cfg.TailnetAddress);if sun!=lastSunshine{lastSunshine=sun;if send("sunshine_status","",sun)!=nil{return errors.New("Sunshine delivery failed")}}}
 }}
}
