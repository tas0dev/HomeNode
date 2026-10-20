package auth

import (
 "crypto/rand"
 "crypto/sha256"
 "crypto/subtle"
 "encoding/base64"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "time"
 "unicode/utf8"
 "golang.org/x/crypto/argon2"
)
func Random()string {b:=make([]byte,32);if _,e:=rand.Read(b);e!=nil{panic(e)};return base64.RawURLEncoding.EncodeToString(b)}
func Hash(password string)string {salt:=make([]byte,16);if _,e:=rand.Read(salt);e!=nil{panic(e)};h:=argon2.IDKey([]byte(password),salt,3,64*1024,2,32);return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s",base64.RawStdEncoding.EncodeToString(salt),base64.RawStdEncoding.EncodeToString(h))}
func Verify(hash,password string)bool {p:=strings.Split(hash,"$");if len(p)!=6||p[1]!="argon2id"||p[2]!="v=19"||p[3]!="m=65536,t=3,p=2"{return false};s,e:=base64.RawStdEncoding.DecodeString(p[4]);if e!=nil||len(s)!=16{return false};expected,e:=base64.RawStdEncoding.DecodeString(p[5]);if e!=nil||len(expected)!=32{return false};actual:=argon2.IDKey([]byte(password),s,3,64*1024,2,32);return subtle.ConstantTimeCompare(expected,actual)==1}
func Match(a,b string)bool{x:=sha256.Sum256([]byte(a));y:=sha256.Sum256([]byte(b));return subtle.ConstantTimeCompare(x[:],y[:])==1}
func Init(dir string,password []byte)error {
 if !utf8.Valid(password)||utf8.RuneCount(password)<12||len(password)>1024{return errors.New("password must contain 12–1024 bytes")}
 if e:=os.MkdirAll(dir,0700);e!=nil{return e}
 for name,value:=range map[string]string{"admin.hash":Hash(string(password)),"agent.token":Random()} {
  f,e:=os.OpenFile(filepath.Join(dir,name),os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if e!=nil{return e};_,e=f.WriteString(value+"\n");ce:=f.Close();if e!=nil{return e};if ce!=nil{return ce}
 };return nil
}
type Session struct {CSRF string;Expires,Authenticated time.Time}
type Sessions struct {mu sync.Mutex;items map[string]Session}
func New()*Sessions{return &Sessions{items:map[string]Session{}}}
func (s *Sessions) Create()(string,Session){s.mu.Lock();defer s.mu.Unlock();now:=time.Now();for k,v:=range s.items{if now.After(v.Expires){delete(s.items,k)}};if len(s.items)>=128 {for k:=range s.items{delete(s.items,k);break}};id:=Random();v:=Session{Random(),now.Add(12*time.Hour),now};s.items[id]=v;return id,v}
func (s *Sessions) Get(id string)(Session,bool){s.mu.Lock();defer s.mu.Unlock();v,ok:=s.items[id];if ok&&time.Now().After(v.Expires){delete(s.items,id);ok=false};return v,ok}
func (s *Sessions) Delete(id string){s.mu.Lock();delete(s.items,id);s.mu.Unlock()}
type Limiter struct {mu sync.Mutex;items map[string][]time.Time}
func NewLimiter()*Limiter{return &Limiter{items:map[string][]time.Time{}}}
func (l *Limiter) Allow(key string,count int,window time.Duration)bool {l.mu.Lock();defer l.mu.Unlock();now:=time.Now();if len(l.items)>2048{for k,v:=range l.items{if len(v)==0||now.Sub(v[len(v)-1])>window{delete(l.items,k)}};if len(l.items)>2048{return false}};v:=l.items[key];i:=0;for i<len(v)&&now.Sub(v[i])>window{i++};v=v[i:];if len(v)>=count{l.items[key]=v;return false};l.items[key]=append(v,now);return true}
