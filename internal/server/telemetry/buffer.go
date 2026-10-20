package telemetry
import (
 "sync"
 "time"
 "github.com/tas0dev/HomeNode/internal/protocol"
)
type Buffer struct {mu sync.RWMutex; items []protocol.Sample; next, count int}
func New(size int)*Buffer{return &Buffer{items:make([]protocol.Sample,size)}}
func (b *Buffer) Add(s protocol.Sample){b.mu.Lock();defer b.mu.Unlock();s.Sanitize();s.ReceivedAt=time.Now().UTC();b.items[b.next]=s;b.next=(b.next+1)%len(b.items);if b.count<len(b.items){b.count++}}
func (b *Buffer) History()[]protocol.Sample{b.mu.RLock();defer b.mu.RUnlock();out:=make([]protocol.Sample,0,b.count);for i:=0;i<b.count;i++{s:=b.items[(b.next-b.count+i+len(b.items))%len(b.items)];if time.Since(s.ReceivedAt)<=120*time.Second {out=append(out,s)}};return out}
func (b *Buffer) Latest()*protocol.Sample{b.mu.RLock();defer b.mu.RUnlock();if b.count==0{return nil};s:=b.items[(b.next-1+len(b.items))%len(b.items)];return &s}
