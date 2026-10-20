package server

import (
 "context"
 "encoding/json"
 "errors"
 "io"
 "io/fs"
 "net"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "strconv"
 "strings"
 "sync"
 "time"

 "github.com/gorilla/websocket"
 "github.com/tas0dev/HomeNode/internal/agent/collector"
 "github.com/tas0dev/HomeNode/internal/config"
 "github.com/tas0dev/HomeNode/internal/protocol"
 "github.com/tas0dev/HomeNode/internal/server/auth"
 "github.com/tas0dev/HomeNode/internal/server/logging"
 "github.com/tas0dev/HomeNode/internal/server/power"
 "github.com/tas0dev/HomeNode/internal/server/state"
 "github.com/tas0dev/HomeNode/internal/server/telemetry"
)
type peer struct{ conn *websocket.Conn; mu sync.Mutex }
func(p *peer) send(v any)error{p.mu.Lock();defer p.mu.Unlock();p.conn.SetWriteDeadline(time.Now().Add(5*time.Second));return p.conn.WriteJSON(v)}
type Server struct {
 cfg config.Config
 mu sync.Mutex
 machine *state.Machine
 agent *peer
 sunshine protocol.Sunshine
 samples,node *telemetry.Buffer
 logs *logging.Store
 sessions *auth.Sessions
 limit *auth.Limiter
 password,token string
 viewers map[chan struct{}]struct{}
 done chan struct{}
 web fs.FS
 loginMu sync.Mutex
}
func New(c config.Config,web fs.FS)(*Server,error){
 hash,e:=os.ReadFile(filepath.Join(c.SecretsDir,"admin.hash"));if e!=nil{return nil,e};token,e:=os.ReadFile(filepath.Join(c.SecretsDir,"agent.token"));if e!=nil{return nil,e}
 h:=strings.TrimSpace(string(hash));t:=strings.TrimSpace(string(token));if !strings.HasPrefix(h,"$argon2id$")||len(t)<32{return nil,errors.New("invalid credentials; initialize secrets first")}
 logs,e:=logging.New(c.LogDir,c.LogBytes,c.LogDays,c.LogLevel);if e!=nil{return nil,e}
 return &Server{cfg:c,machine:state.New(c.ReachTimeout,c.StartTimeout,c.RestartTimeout,c.ShutdownTimeout),samples:telemetry.New(c.HistorySamples),node:telemetry.New(c.HistorySamples),logs:logs,sessions:auth.New(),limit:auth.NewLimiter(),password:h,token:t,viewers:map[chan struct{}]struct{}{},sunshine:protocol.Sunshine{State:"UNKNOWN"},web:web,done:make(chan struct{})},nil
}
func(s *Server) notifyLocked(){for ch:=range s.viewers{select{case ch<-struct{}{}:default:}}}
func(s *Server) record(level,component,id,message string,fields map[string]any){s.logs.Add(level,component,id,message,fields);s.notifyLocked()}
func(s *Server) transition(before string){if before!=s.machine.State {fields:=map[string]any{"from":before,"to":s.machine.State};if op:=s.machine.Operation;op!=nil{fields["request_id"]=op.ID;fields["actor"]=op.Actor;fields["operation"]=op.Kind;fields["result"]=op.Status;fields["reason"]=op.Reason};s.record("INFO","SERVER","state.changed","PCの状態が変わりました",fields)}}
func(s *Server) Run(ctx context.Context){
 defer close(s.done)
 c:=collector.New(s.cfg);defer c.Close();ticks:=time.NewTicker(s.cfg.SampleInterval);defer ticks.Stop();nodeFailed:=false
 s.mu.Lock();s.record("INFO","SERVER","server.started","HomeNodeを起動しました",nil);s.mu.Unlock()
 for{select{case <-ctx.Done():s.mu.Lock();if s.agent!=nil{s.agent.conn.Close()};s.record("INFO","SERVER","server.stopped","HomeNodeを終了します",nil);s.mu.Unlock();return
 case <-ticks.C:
  sample:=c.Collect(ctx);s.node.Add(sample)
  s.mu.Lock();failed:=sample.CPU.Usage==nil||sample.Memory.Total==nil;if failed!=nodeFailed{nodeFailed=failed;if failed{s.record("WARN","NODE","node.collection_failed","Piの一部の監視値を取得できません",nil)}else{s.record("INFO","NODE","node.collection_recovered","Piの監視値の取得が復旧しました",nil)}};before:=s.machine.State;s.machine.Tick(time.Now());if s.machine.State==state.Unreachable&&s.agent!=nil{s.agent.conn.Close()};s.transition(before);s.notifyLocked();s.mu.Unlock()
 }}
}
func jsonResponse(w http.ResponseWriter,status int,v any){w.Header().Set("Content-Type","application/json; charset=utf-8");w.WriteHeader(status);json.NewEncoder(w).Encode(v)}
func failure(w http.ResponseWriter,status int,code,message string){jsonResponse(w,status,map[string]string{"code":code,"message":message,"request_id":auth.Random()})}
func decode(w http.ResponseWriter,r *http.Request,v any)error {r.Body=http.MaxBytesReader(w,r.Body,protocol.MaxMessage);d:=json.NewDecoder(r.Body);d.DisallowUnknownFields();if e:=d.Decode(v);e!=nil{return e};if e:=d.Decode(&struct{}{});e!=io.EOF{return errors.New("trailing JSON")};return nil}
func(s *Server) origin(r *http.Request)bool{return r.Header.Get("Origin")==strings.TrimRight(s.cfg.PublicURL,"/")}
func(s *Server) session(r *http.Request)(auth.Session,bool){c,e:=r.Cookie("__Host-homenode");if e!=nil{return auth.Session{},false};return s.sessions.Get(c.Value)}
func(s *Server) Handler()http.Handler {
 mux:=http.NewServeMux()
 mux.HandleFunc("GET /api/v1/health",func(w http.ResponseWriter,r *http.Request){jsonResponse(w,200,map[string]string{"status":"ok"})})
 mux.HandleFunc("POST /api/v1/auth/login",s.login)
 mux.HandleFunc("/agent/v1/connect",s.agentConnect)
 mux.HandleFunc("/api/v1/",s.api)
 mux.HandleFunc("/",func(w http.ResponseWriter,r *http.Request){if r.Method!="GET"&&r.Method!="HEAD"{failure(w,405,"method","method not allowed");return};path:=strings.TrimPrefix(r.URL.Path,"/");if path==""{path="index.html"};b,e:=fs.ReadFile(s.web,path);if e!=nil{if strings.Contains(filepath.Base(path),"."){http.NotFound(w,r);return};b,e=fs.ReadFile(s.web,"index.html")};if e!=nil{http.Error(w,"web assets missing; run npm run build",503);return};switch filepath.Ext(path){case ".js":w.Header().Set("Content-Type","text/javascript; charset=utf-8");case ".css":w.Header().Set("Content-Type","text/css; charset=utf-8");case ".txt":w.Header().Set("Content-Type","text/plain; charset=utf-8");default:w.Header().Set("Content-Type","text/html; charset=utf-8")};w.Write(b)})
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("X-Content-Type-Options","nosniff");w.Header().Set("Referrer-Policy","no-referrer");w.Header().Set("Cache-Control","no-store");w.Header().Set("Content-Security-Policy","default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
  host,_,e:=net.SplitHostPort(r.RemoteAddr);if e!=nil||net.ParseIP(host)==nil||!net.ParseIP(host).IsLoopback(){failure(w,403,"tailnet_only","Tailscale Serve経由で接続してください");return}
  if r.URL.Path!="/api/v1/health"{u,_:=url.Parse(s.cfg.PublicURL);if r.Host!=u.Host{failure(w,403,"host","invalid host");return}}
  if r.Method!="GET"&&r.Method!="HEAD"&&r.URL.Path!="/agent/v1/connect" {if !s.origin(r)||!strings.HasPrefix(r.Header.Get("Content-Type"),"application/json"){failure(w,403,"origin","invalid origin or content type");return}}
  mux.ServeHTTP(w,r)
 })
}
func(s *Server) login(w http.ResponseWriter,r *http.Request){
 // A single bounded verifier prevents parallel Argon2 allocations.
 if !s.limit.Allow("login",5,time.Minute){failure(w,429,"rate_limit","しばらく待ってから再試行してください");return}
 var in struct{Password string `json:"password"`};if decode(w,r,&in)!=nil||len(in.Password)>1024{failure(w,400,"input","invalid input");return}
 s.loginMu.Lock();ok:=auth.Verify(s.password,in.Password);s.loginMu.Unlock()
 if !ok{s.mu.Lock();s.record("WARN","SERVER","auth.failed","ログイン認証に失敗しました",nil);s.mu.Unlock();failure(w,401,"authentication","パスワードが違います");return}
 id,v:=s.sessions.Create();http.SetCookie(w,&http.Cookie{Name:"__Host-homenode",Value:id,Path:"/",Secure:true,HttpOnly:true,SameSite:http.SameSiteStrictMode,MaxAge:43200});jsonResponse(w,200,map[string]string{"csrf_token":v.CSRF})
}
func(s *Server) api(w http.ResponseWriter,r *http.Request){
 session,ok:=s.session(r);if !ok{failure(w,401,"authentication","ログインしてください");return}
 if r.Method!="GET" {if !auth.Match(r.Header.Get("X-CSRF-Token"),session.CSRF){failure(w,403,"csrf","invalid CSRF token");return}}
 path:=strings.TrimPrefix(r.URL.Path,"/api/v1")
 if path=="/events"&&r.Method=="GET"{s.events(w,r,session);return}
 if path=="/auth/session"&&r.Method=="GET"{jsonResponse(w,200,map[string]string{"csrf_token":session.CSRF});return}
 if path=="/auth/logout"&&r.Method=="POST"{c,_:=r.Cookie("__Host-homenode");s.sessions.Delete(c.Value);http.SetCookie(w,&http.Cookie{Name:"__Host-homenode",Path:"/",MaxAge:-1,Secure:true,HttpOnly:true,SameSite:http.SameSiteStrictMode});jsonResponse(w,200,map[string]bool{"ok":true});return}
 if strings.HasPrefix(path,"/system/power/")&&r.Method=="POST"{s.powerRequest(w,r,session,strings.TrimPrefix(path,"/system/power/"));return}
 if r.Method!="GET"{failure(w,405,"method","method not allowed");return}
 s.mu.Lock();defer s.mu.Unlock()
 switch path {
 case "/system/status":jsonResponse(w,200,map[string]any{"telemetry_interval_seconds":s.cfg.SampleInterval.Seconds(),"stale_after_seconds":s.cfg.ReachTimeout.Seconds(),"name":s.cfg.PCName,"state":s.machine.State,"last_heartbeat":s.machine.LastHeartbeat,"operation":s.machine.Operation,"stale":s.machine.State!=state.Online||s.machine.LastHeartbeat==nil||time.Since(*s.machine.LastHeartbeat)>s.cfg.ReachTimeout})
 case "/system/metrics":v:=s.samples.Latest();stale:=s.machine.State!=state.Online||v==nil;if v!=nil{stale=stale||time.Since(v.ReceivedAt)>s.cfg.ReachTimeout};jsonResponse(w,200,map[string]any{"sample":v,"stale":stale})
 case "/system/metrics/history":jsonResponse(w,200,s.samples.History())
 case "/node/metrics":jsonResponse(w,200,map[string]any{"sample":s.node.Latest(),"history":s.node.History()})
 case "/sunshine/status":v:=s.sunshine;if s.machine.State!=state.Online{v.State="UNKNOWN"};jsonResponse(w,200,v)
 case "/logs":q:=r.URL.Query();level:=q.Get("level");component:=q.Get("component");if level!=""&&!contains([]string{"DEBUG","INFO","WARN","ERROR"},level)||component!=""&&!contains([]string{"SERVER","AGENT","SUNSHINE","NODE"},component){failure(w,400,"filter","invalid filter");return};page,_:=strconv.Atoi(q.Get("page"));if page<1{page=1};if page>100000{failure(w,400,"page","invalid page");return};items,more:=s.logs.List(level,component,page);jsonResponse(w,200,map[string]any{"items":items,"page":page,"has_more":more})
 default:failure(w,404,"not_found","not found")
 }
}
func contains(a []string,b string)bool{for _,v:=range a{if v==b{return true}};return false}
func(s *Server) powerRequest(w http.ResponseWriter,r *http.Request,session auth.Session,kind string){
 if !contains([]string{"on","shutdown","restart"},kind){failure(w,404,"operation","unsupported operation");return}
 var in struct{Password string `json:"password"`};if decode(w,r,&in)!=nil||len(in.Password)>1024{failure(w,400,"input","invalid input");return}
 if !s.limit.Allow("power",5,time.Minute){failure(w,429,"rate_limit","しばらく待ってから再試行してください");return}
 // Reauthenticate every power request, including requests from an old session.
 s.loginMu.Lock();valid:=auth.Verify(s.password,in.Password);s.loginMu.Unlock();if !valid{failure(w,401,"reauthentication","パスワードが違います");return}
 s.mu.Lock();defer s.mu.Unlock();id:=auth.Random();before:=s.machine.State
 if e:=s.machine.Request(kind,id,"admin",time.Now());e!=nil{failure(w,409,"state",e.Error());return}
 s.record("INFO","SERVER","power.requested","電源操作を受け付けました",map[string]any{"actor":"admin","target":s.cfg.PCName,"operation":kind,"request_id":id,"result":"accepted"})
 var e error
 if kind=="on"{e=power.Wake(s.cfg.MAC,s.cfg.Broadcast)}else if s.agent==nil {e=errors.New("agent disconnected")}else{e=s.agent.send(protocol.New("command",s.cfg.AgentID,id,protocol.Command{Operation:kind}))}
 if e!=nil{s.machine.Fail("delivery failed");s.record("ERROR","SERVER","power.delivery_failed","電源要求を送信できませんでした",map[string]any{"request_id":id,"actor":"admin","target":s.cfg.PCName,"operation":kind,"result":"failed"});s.transition(before);failure(w,502,"delivery","電源要求を送信できませんでした");return}
 s.transition(before);s.notifyLocked();jsonResponse(w,202,map[string]string{"request_id":id,"status":"accepted"})
}
func(s *Server) events(w http.ResponseWriter,r *http.Request,session auth.Session){
 if !s.origin(r){failure(w,403,"origin","invalid origin");return}
 s.mu.Lock();if len(s.viewers)>=32{s.mu.Unlock();failure(w,429,"connections","too many connections");return};ch:=make(chan struct{},1);s.viewers[ch]=struct{}{};s.mu.Unlock()
 defer func(){s.mu.Lock();delete(s.viewers,ch);s.mu.Unlock()}()
 conn,e:=(&websocket.Upgrader{CheckOrigin:s.origin}).Upgrade(w,r,nil);if e!=nil{return};defer conn.Close();conn.SetReadLimit(1024)
 done:=make(chan struct{});go func(){defer close(done);for{if _,_,e:=conn.ReadMessage();e!=nil{return}}}()
 ticker:=time.NewTicker(10*time.Second);defer ticker.Stop()
 for {
  if _,ok:=s.session(r);!ok||time.Now().After(session.Expires){return}
  select{case <-s.done:return;case <-done:return;case <-r.Context().Done():return;case <-ticker.C:case <-ch:}
  s.mu.Lock()
  latest:=s.samples.Latest()
  stale:=s.machine.State!=state.Online||latest==nil
  if latest!=nil{stale=stale||time.Since(latest.ReceivedAt)>s.cfg.ReachTimeout}
  sun:=s.sunshine;if s.machine.State!=state.Online{sun.State="UNKNOWN"}
  records,_:=s.logs.List("","",1)
  marker:="";if len(records)>0{marker=records[0].Timestamp.Format(time.RFC3339Nano)+records[0].ID}
  payload,err:=json.Marshal(map[string]any{
   "type":"snapshot","timestamp":time.Now().UTC(),
   "status":map[string]any{"name":s.cfg.PCName,"state":s.machine.State,"last_heartbeat":s.machine.LastHeartbeat,"operation":s.machine.Operation,"stale":stale,"stale_after_seconds":s.cfg.ReachTimeout.Seconds()},
   "metrics":map[string]any{"sample":latest,"stale":stale},
   "node":s.node.Latest(),"sunshine":sun,"log_marker":marker,
  })
  s.mu.Unlock()
  if err!=nil{return}
  conn.SetWriteDeadline(time.Now().Add(5*time.Second));if conn.WriteMessage(websocket.TextMessage,payload)!=nil{return}
 }
}
func(s *Server) agentConnect(w http.ResponseWriter,r *http.Request){
 if r.Method!="GET"{failure(w,405,"method","method not allowed");return}
 if !s.limit.Allow("agent-connect",30,time.Minute){failure(w,429,"rate_limit","too many connections");return}
 if !auth.Match(strings.TrimPrefix(r.Header.Get("Authorization"),"Bearer "),s.token)||!strings.HasPrefix(r.Header.Get("Authorization"),"Bearer "){s.mu.Lock();s.record("WARN","AGENT","agent.auth_failed","Agent認証に失敗しました",nil);s.mu.Unlock();failure(w,401,"authentication","invalid agent token");return}
 if r.Header.Get("Origin")!=""{failure(w,403,"origin","agent endpoint is not a browser endpoint");return}
 conn,e:=(&websocket.Upgrader{CheckOrigin:func(r *http.Request)bool{return r.Header.Get("Origin")==""}}).Upgrade(w,r,nil);if e!=nil{return};defer conn.Close();conn.SetReadLimit(protocol.MaxMessage);conn.SetReadDeadline(time.Now().Add(10*time.Second))
 var hello protocol.Message;if e=conn.ReadJSON(&hello);e!=nil||hello.Validate(s.cfg.AgentID)!=nil||hello.Type!="hello"{return}
 var info struct{Hostname string `json:"hostname"`;OS string `json:"os"`; BootTime uint64 `json:"boot_time"`};if json.Unmarshal(hello.Payload,&info)!=nil||info.OS!="windows"||len(info.Hostname)>255{return}
 p:=&peer{conn:conn};s.mu.Lock();if s.agent!=nil{s.mu.Unlock();conn.WriteControl(websocket.CloseMessage,websocket.FormatCloseMessage(websocket.ClosePolicyViolation,"already connected"),time.Now().Add(time.Second));return};s.agent=p;s.machine.Connected(info.BootTime);s.record("INFO","AGENT","agent.connected","Agentが接続しました",nil);s.mu.Unlock()
 defer func(){s.mu.Lock();if s.agent==p{before:=s.machine.State;s.agent=nil;s.machine.Disconnected();s.sunshine.State="UNKNOWN";s.record("WARN","AGENT","agent.disconnected","Agentとの接続が切れました",nil);s.transition(before)};s.mu.Unlock()}()
 conn.SetReadDeadline(time.Now().Add(s.cfg.ReachTimeout));lastTelemetry:=time.Time{};windowStart:=time.Now();windowCount:=0
 for {
  var m protocol.Message;if conn.ReadJSON(&m)!=nil{return};if m.Validate(s.cfg.AgentID)!=nil{return};if time.Since(windowStart)>=time.Second{windowStart=time.Now();windowCount=0};windowCount++;if windowCount>20{return}
  s.mu.Lock();before:=s.machine.State;bad:=false
  switch m.Type {
  case "heartbeat":s.machine.Heartbeat(time.Now());if p.send(protocol.New("heartbeat_ack",s.cfg.AgentID,"",struct{}{}))!=nil{bad=true};conn.SetReadDeadline(time.Now().Add(s.cfg.ReachTimeout))
  case "telemetry":
   var sample protocol.Sample;if json.Unmarshal(m.Payload,&sample)!=nil||sample.Timestamp.IsZero()||time.Since(sample.Timestamp)>time.Minute||time.Until(sample.Timestamp)>time.Minute {bad=true;break}
   if time.Since(lastTelemetry)<s.cfg.SampleInterval/2 {break};lastTelemetry=time.Now();s.samples.Add(sample)
  case "sunshine_status":var v protocol.Sunshine;if json.Unmarshal(m.Payload,&v)!=nil||!contains([]string{"UNKNOWN","STOPPED","RUNNING","READY"},v.State)||len(v.LAN)>255||len(v.Tailnet)>255{bad=true;break};if s.sunshine.State!=v.State{s.record("INFO","SUNSHINE","sunshine.changed","Sunshineの状態が変わりました",map[string]any{"state":v.State})};s.sunshine=v
  case "command_ack":var a protocol.Ack;if json.Unmarshal(m.Payload,&a)!=nil||!contains([]string{"","invalid_command","execution_uncertain","journal_unavailable","windows_api_failed"},a.Reason)||!s.machine.Ack(m.RequestID,a.Accepted,a.Reason){bad=true;break};s.record("INFO","AGENT","power.ack","Agentが電源要求に応答しました",map[string]any{"operation":s.machine.Operation.Kind,"request_id":m.RequestID,"accepted":a.Accepted,"reason":a.Reason,"actor":"admin","target":s.cfg.PCName,"result":s.machine.Operation.Status})
  case "event":if !s.limit.Allow("agent-event",30,time.Minute){break};var ev struct{Level string `json:"level"`;ID string `json:"event_id"`};if json.Unmarshal(m.Payload,&ev)!=nil||!contains([]string{"DEBUG","INFO","WARN","ERROR"},ev.Level)||!contains([]string{"collector.unavailable","collector.recovered","agent.started","agent.stopped"},ev.ID){bad=true;break};s.record(ev.Level,"AGENT","agent.event","Agentイベントを受信しました",map[string]any{"event_id":ev.ID})
  default:s.record("WARN","AGENT","protocol.unknown","未知のAgentメッセージを拒否しました",nil);bad=true
  }
  s.transition(before);s.notifyLocked();s.mu.Unlock();if bad{return}
 }
}