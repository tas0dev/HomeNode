//go:build windows

package collector

import (
 "strings"
 "unsafe"
 "golang.org/x/sys/windows"
 "github.com/tas0dev/HomeNode/internal/protocol"
)
var pdh=windows.NewLazySystemDLL("pdh.dll")
var openQuery=pdh.NewProc("PdhOpenQueryW")
var addCounter=pdh.NewProc("PdhAddEnglishCounterW")
var collectQuery=pdh.NewProc("PdhCollectQueryData")
var formattedArray=pdh.NewProc("PdhGetFormattedCounterArrayW")
var closeQuery=pdh.NewProc("PdhCloseQuery")
type gpuCounters struct{query,usage,vram uintptr;primed bool}
type counterItem struct{Name *uint16;Status uint32;Padding uint32;Value float64}
func newGPU()*gpuCounters {
 g:=&gpuCounters{};if r,_,_:=openQuery.Call(0,0,uintptr(unsafe.Pointer(&g.query)));r!=0{return g}
 for path,target:=range map[string]*uintptr{`\GPU Engine(*)\Utilization Percentage`:&g.usage,`\GPU Adapter Memory(*)\Dedicated Usage`:&g.vram}{p,_:=windows.UTF16PtrFromString(path);if r,_,_:=addCounter.Call(g.query,uintptr(unsafe.Pointer(p)),0,uintptr(unsafe.Pointer(target)));r!=0{*target=0}}
 return g
}
func values(counter uintptr)map[string]float64 {
 out:=map[string]float64{};if counter==0{return out};var size,count uint32
 r,_,_:=formattedArray.Call(counter,0x200,uintptr(unsafe.Pointer(&size)),uintptr(unsafe.Pointer(&count)),0)
 if r!=0x800007D2||size==0||size>4<<20{return out};b:=make([]byte,size)
 r,_,_=formattedArray.Call(counter,0x200,uintptr(unsafe.Pointer(&size)),uintptr(unsafe.Pointer(&count)),uintptr(unsafe.Pointer(&b[0])))
 if r!=0||int(count)>len(b)/int(unsafe.Sizeof(counterItem{})){return out}
 items:=unsafe.Slice((*counterItem)(unsafe.Pointer(&b[0])),int(count));for _,item:=range items{if item.Name!=nil&&(item.Status==0||item.Status==1){out[windows.UTF16PtrToString(item.Name)]=item.Value}};return out
}
func(g *gpuCounters) collect(s *protocol.Sample){if g.query==0{return};r,_,_:=collectQuery.Call(g.query);if r!=0{return};if !g.primed{g.primed=true;return}
 // Sum process contributions per engine; use the busiest engine rather than
 // summing simultaneous engines (which can exceed 100%).
 engines:=map[string]float64{};for name,v:=range values(g.usage){i:=strings.Index(name,"luid_");if i>=0{engines[name[i:]]+=v}}
 var max float64;for _,v:=range engines{if v>max{max=v}};if len(engines)>0{if max>100{max=100};s.GPU.Usage=protocol.Number(max)}
 adapters:=values(g.vram);if len(adapters)==1{for _,v:=range adapters{s.GPU.VRAMUsed=protocol.Number(v)}}
 // Multiple adapters are intentionally not merged into a fictional single GPU.
 if len(adapters)>1{s.GPU.Usage=nil;s.Availability["gpu"]="複数GPU: 明示的なセンサーマッピングが必要です"}
}
func(g *gpuCounters) close(){if g.query!=0{closeQuery.Call(g.query);g.query=0}}
