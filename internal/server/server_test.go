package server
import (
 "bytes"
 "context"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
 "testing/fstest"
 "time"
 "github.com/gorilla/websocket"
 "github.com/tas0dev/HomeNode/internal/config"
 "github.com/tas0dev/HomeNode/internal/protocol"
 "github.com/tas0dev/HomeNode/internal/server/auth"
 "github.com/tas0dev/HomeNode/internal/server/state"
)
func testServer(t *testing.T)*Server{t.Helper();dir:=t.TempDir();if e:=auth.Init(dir,[]byte("test-password-123"));e!=nil{t.Fatal(e)};c:=config.Config{PCName:"test",PublicURL:"https://homenode.test",AgentID:"desktop",SecretsDir:dir,LogDir:filepath.Join(dir,"logs"),LogLevel:"INFO",LogBytes:20<<20,LogDays:7,ReachTimeout:15*time.Second,StartTimeout:120*time.Second,RestartTimeout:180*time.Second,ShutdownTimeout:120*time.Second,SampleInterval:time.Second,HistorySamples:120};s,e:=New(c,fstest.MapFS{"index.html":{Data:[]byte("HomeNode")}});if e!=nil{t.Fatal(e)};return s}
func request(s *Server,method,path,body,token,csrf string)*httptest.ResponseRecorder{r:=httptest.NewRequest(method,"https://homenode.test"+path,bytes.NewBufferString(body));r.RemoteAddr="127.0.0.1:1234";r.Header.Set("Origin","https://homenode.test");r.Header.Set("Content-Type","application/json");if token!=""{r.AddCookie(&http.Cookie{Name:"__Host-homenode",Value:token})};if csrf!=""{r.Header.Set("X-CSRF-Token",csrf)};w:=httptest.NewRecorder();s.Handler().ServeHTTP(w,r);return w}
func TestAuthenticationAndCSRF(t *testing.T){s:=testServer(t);if w:=request(s,"POST","/api/v1/system/power/on",`{"password":"test-password-123"}`,"","");w.Code!=401{t.Fatal(w.Code)}
 login:=request(s,"POST","/api/v1/auth/login",`{"password":"test-password-123"}`,"","");if login.Code!=200{t.Fatal(login.Code)};cookie:=login.Result().Cookies()[0];if !cookie.Secure||!cookie.HttpOnly||cookie.SameSite!=http.SameSiteStrictMode{t.Fatal("unsafe cookie")}
 id,v:=s.sessions.Create();if w:=request(s,"POST","/api/v1/system/power/on",`{"password":"test-password-123"}`,id,"");w.Code!=403{t.Fatal("missing CSRF accepted",w.Code)}
 if w:=request(s,"POST","/api/v1/auth/logout",`{}`,id,v.CSRF);w.Code!=200{t.Fatal(w.Code)};if w:=request(s,"GET","/api/v1/system/status","",id,"");w.Code!=401{t.Fatal("session remains valid")}
 r:=httptest.NewRequest("GET","https://homenode.test/api/v1/health",nil);r.RemoteAddr="192.168.1.3:1234";w:=httptest.NewRecorder();s.Handler().ServeHTTP(w,r);if w.Code!=403{t.Fatal("non-loopback accepted")}
}
func TestAgentHandshakeAndHeartbeat(t *testing.T){s:=testServer(t);h:=httptest.NewServer(s.Handler());defer h.Close();url:="ws"+strings.TrimPrefix(h.URL,"http")+"/agent/v1/connect"
 header:=http.Header{"Authorization":[]string{"Bearer "+s.token},"Host":[]string{"homenode.test"}}
 c,_,e:=websocket.DefaultDialer.DialContext(context.Background(),url,header);if e!=nil{t.Fatal(e)};defer c.Close()
 c.WriteJSON(protocol.New("hello","desktop","",map[string]any{"hostname":"test","os":"windows","boot_time":1}));c.WriteJSON(protocol.New("heartbeat","desktop","",struct{}{}));c.SetReadDeadline(time.Now().Add(time.Second));var reply protocol.Message;if e=c.ReadJSON(&reply);e!=nil||reply.Type!="heartbeat_ack"{t.Fatal("heartbeat not acknowledged",e)}
 s.mu.Lock();online:=s.machine.State==state.Online;s.mu.Unlock();if !online{t.Fatal("valid heartbeat did not bring Agent online")}
 c.Close();deadline:=time.Now().Add(time.Second);for time.Now().Before(deadline){s.mu.Lock();unreachable:=s.machine.State==state.Unreachable;s.mu.Unlock();if unreachable{return};time.Sleep(time.Millisecond)};t.Fatal("disconnect not observed")
}
