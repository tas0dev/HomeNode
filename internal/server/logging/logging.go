package logging

import (
 "bufio"
 "encoding/json"
 "fmt"
 "log/slog"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "sync"
 "time"
)

type Event struct {
 Timestamp time.Time `json:"timestamp"`
 Level string `json:"level"`
 Component string `json:"component"`
 ID string `json:"event_id"`
 Message string `json:"message"`
 Fields map[string]any `json:"fields"`
}

type Store struct {
 mu sync.Mutex
 dir string
 max int64
 days int
 level slog.Level
 items []Event
 sizes []int64
 bytes int64
}

func New(dir string,max int64,days int,level string)(*Store,error) {
 if e:=os.MkdirAll(dir,0700);e!=nil{return nil,e}
 var l slog.Level
 if e:=l.UnmarshalText([]byte(level));e!=nil{return nil,e}
 s:=&Store{dir:dir,max:max,days:days,level:l}
 s.prune()
 entries,_:=os.ReadDir(dir)
 for _,entry:=range entries {
  if entry.IsDir()||!strings.HasSuffix(entry.Name(),".jsonl"){continue}
  f,e:=os.Open(filepath.Join(dir,entry.Name()));if e!=nil{return nil,e}
  scan:=bufio.NewScanner(f);scan.Buffer(make([]byte,4096),128<<10)
  for scan.Scan(){var v Event;if json.Unmarshal(scan.Bytes(),&v)==nil&&time.Since(v.Timestamp)<=time.Duration(days)*24*time.Hour{s.items=append(s.items,v)}}
  e=scan.Err();f.Close();if e!=nil{return nil,e}
 }
 sort.SliceStable(s.items,func(i,j int)bool{return s.items[i].Timestamp.Before(s.items[j].Timestamp)})
 for _,v:=range s.items{b,_:=json.Marshal(v);size:=int64(len(b)+1);s.sizes=append(s.sizes,size);s.bytes+=size}
 s.trim()
 return s,nil
}

func(s *Store) trim(){
 remove:=0
 for remove<len(s.items)&&(s.bytes>s.max||time.Since(s.items[remove].Timestamp)>time.Duration(s.days)*24*time.Hour){s.bytes-=s.sizes[remove];remove++}
 if remove>0{s.items=append([]Event(nil),s.items[remove:]...);s.sizes=append([]int64(nil),s.sizes[remove:]...)}
}

func(s *Store) prune(){
 entries,_:=os.ReadDir(s.dir)
 type item struct{path string;size int64;at time.Time}
 var files []item
 var total int64
 for _,e:=range entries {
  if e.IsDir()||!strings.HasSuffix(e.Name(),".jsonl"){continue}
  i,err:=e.Info();if err!=nil{continue}
  path:=filepath.Join(s.dir,e.Name())
  if time.Since(i.ModTime())>time.Duration(s.days)*24*time.Hour {os.Remove(path);continue}
  files=append(files,item{path,i.Size(),i.ModTime()});total+=i.Size()
 }
 sort.Slice(files,func(i,j int)bool{return files[i].at.Before(files[j].at)})
 for _,f:=range files{if total<s.max{return};if os.Remove(f.path)==nil{total-=f.size}}
}

func(s *Store) Add(level,component,id,message string,fields map[string]any)Event {
 e:=Event{time.Now().UTC(),level,component,id,message,fields}
 s.mu.Lock();defer s.mu.Unlock()
 var l slog.Level
 if l.UnmarshalText([]byte(level))!=nil||l<s.level{return e}
 s.prune()
 name:=filepath.Join(s.dir,"current.jsonl")
 // Rotate on each UTC day as well as size, so quiet installations do not keep
 // appending to one old file indefinitely.
 if info,err:=os.Stat(name);err==nil&&(info.Size()>s.max/8||info.ModTime().UTC().Format("2006-01-02")!=e.Timestamp.Format("2006-01-02")) {
  if err=os.Rename(name,filepath.Join(s.dir,fmt.Sprintf("%d.jsonl",time.Now().UnixNano())));err!=nil{slog.Error("log rotation failed")}
 }
 b,err:=json.Marshal(e)
 if err!=nil{slog.Error("log encoding failed");return e}
 f,err:=os.OpenFile(name,os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600)
 if err==nil{_,err=f.Write(append(b,'\n'));if err==nil{err=f.Sync()};f.Close()}
 if err!=nil{slog.Error("persistent log write failed")}
 s.items=append(s.items,e);size:=int64(len(b)+1);s.sizes=append(s.sizes,size);s.bytes+=size;s.trim();s.prune()
 return e
}

func(s *Store) List(level,component string,page int)([]Event,bool){
 s.mu.Lock();defer s.mu.Unlock();s.trim();s.prune()
 start:=(page-1)*50;skipped:=0;out:=make([]Event,0,50)
 for i:=len(s.items)-1;i>=0;i--{
  v:=s.items[i]
  if level!=""&&level!=v.Level||component!=""&&component!=v.Component{continue}
  if skipped<start{skipped++;continue}
  if len(out)==50{return out,true}
  out=append(out,v)
 }
 return out,false
}
