package collector

import (
 "context"
 "encoding/json"
 "fmt"
 "io"
 "net"
 "net/http"
 "os"
 "runtime"
 "strconv"
 "strings"
 "time"

 "github.com/shirou/gopsutil/v4/cpu"
 "github.com/shirou/gopsutil/v4/host"
 "github.com/shirou/gopsutil/v4/mem"
 psnet "github.com/shirou/gopsutil/v4/net"
 "github.com/tas0dev/HomeNode/internal/config"
 "github.com/tas0dev/HomeNode/internal/protocol"
)
type Collector struct {cfg config.Config;last psnet.IOCountersStat;lastTime time.Time;model string;client *http.Client;gpu *gpuCounters}
func New(c config.Config)*Collector{tr:=&http.Transport{Proxy:nil,DisableKeepAlives:false};return &Collector{cfg:c,client:&http.Client{Timeout:750*time.Millisecond,Transport:tr,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}},gpu:newGPU()}}
func(c *Collector) Close(){c.gpu.close();c.client.CloseIdleConnections()}
func(c *Collector) Collect(ctx context.Context)protocol.Sample {
 s:=protocol.Sample{Timestamp:time.Now().UTC(),Availability:map[string]string{},Wifi:"UNKNOWN"};s.GPU.Model=c.cfg.GPUModel
 if v,e:=cpu.PercentWithContext(ctx,0,false);e==nil&&len(v)>0{s.CPU.Usage=protocol.Number(v[0])}
 if v,e:=cpu.InfoWithContext(ctx);e==nil&&len(v)>0{s.CPU.Model=v[0].ModelName;if v[0].Mhz>0{s.CPU.Clock=protocol.Number(v[0].Mhz)}}
 if v,e:=mem.VirtualMemoryWithContext(ctx);e==nil{s.Memory=protocol.Memory{Used:protocol.Number(float64(v.Used)),Total:protocol.Number(float64(v.Total)),Available:protocol.Number(float64(v.Available)),Usage:protocol.Number(v.UsedPercent)}}
 if v,e:=host.UptimeWithContext(ctx);e==nil{s.Uptime=protocol.Number(float64(v))}
 name:=c.cfg.Interface
 if name==""{if interfaces,e:=net.Interfaces();e==nil{for _,i:=range interfaces{if i.Flags&net.FlagUp!=0&&i.Flags&net.FlagLoopback==0&&!strings.HasPrefix(i.Name,"tailscale")&&!strings.HasPrefix(i.Name,"docker"){name=i.Name;break}}}}
 s.Network.Interface=name
 if iface,e:=net.InterfaceByName(name);e==nil{addresses,_:=iface.Addrs();for _,a:=range addresses{ip,_,_:=net.ParseCIDR(a.String());if ip!=nil&&ip.To4()!=nil{s.Network.IPv4=ip.String();break}}}
 if stats,e:=psnet.IOCountersWithContext(ctx,true);e==nil{for _,v:=range stats{if v.Name!=name{continue};s.Network.RXTotal=protocol.Number(float64(v.BytesRecv));s.Network.TXTotal=protocol.Number(float64(v.BytesSent));elapsed:=s.Timestamp.Sub(c.lastTime).Seconds();if !c.lastTime.IsZero()&&elapsed>0&&c.last.Name==v.Name {if v.BytesRecv>=c.last.BytesRecv{s.Network.RX=protocol.Number(float64(v.BytesRecv-c.last.BytesRecv)/elapsed)};if v.BytesSent>=c.last.BytesSent{s.Network.TX=protocol.Number(float64(v.BytesSent-c.last.BytesSent)/elapsed)}};c.last=v;c.lastTime=s.Timestamp}}
 platformNetwork(&s)
 c.gpu.collect(&s)
 if runtime.GOOS=="linux" {
  if b,e:=os.ReadFile("/sys/class/thermal/thermal_zone0/temp");e==nil {v,e:=strconv.ParseFloat(strings.TrimSpace(string(b)),64);if e==nil{s.CPU.Temperature=protocol.Number(v/1000)}}
  if b,e:=os.ReadFile("/proc/net/wireless");e==nil {s.Wifi="DISCONNECTED";for _,line:=range strings.Split(string(b),"\n"){if strings.Contains(line,":"){s.Wifi="CONNECTED";break}}}
  if b,e:=os.ReadFile("/sys/class/net/"+name+"/speed");e==nil{if v,e:=strconv.ParseFloat(strings.TrimSpace(string(b)),64);e==nil&&v>0{s.Network.LinkSpeed=protocol.Number(v*1e6)}}
 }
 if c.cfg.SensorURL!=""{c.sensors(ctx,&s)}
 if s.CPU.Temperature==nil{s.Availability["cpu.temperature_c"]="センサープロバイダー未対応または取得失敗"}
 if s.GPU.Temperature==nil{s.Availability["gpu.temperature_c"]="センサープロバイダー未対応または取得失敗"}
 s.Sanitize();return s
}
// LibreHardwareMonitor's optional loopback-only JSON tree. Explicit sensor IDs
// avoid selecting the wrong CPU/GPU or mixing temperatures from different chips.
type sensor struct {ID any `json:"id"`;SensorID string `json:"SensorId"`;Value any `json:"Value"`;Children []sensor `json:"Children"`}
func(c *Collector) sensors(ctx context.Context,s *protocol.Sample){
 req,e:=http.NewRequestWithContext(ctx,"GET",c.cfg.SensorURL,nil);if e!=nil{return};res,e:=c.client.Do(req);if e!=nil{s.Availability["sensors"]="センサープロバイダーに接続できません";return};defer res.Body.Close();if res.StatusCode!=200{return};var root sensor;if json.NewDecoder(io.LimitReader(res.Body,2<<20)).Decode(&root)!=nil{return}
 fields:=map[string]**float64{"cpu.temperature_c":&s.CPU.Temperature,"cpu.clock_mhz":&s.CPU.Clock,"gpu.usage_percent":&s.GPU.Usage,"gpu.temperature_c":&s.GPU.Temperature,"gpu.clock_mhz":&s.GPU.Clock,"gpu.vram_used_bytes":&s.GPU.VRAMUsed,"gpu.vram_total_bytes":&s.GPU.VRAMTotal,"gpu.power_w":&s.GPU.Power}
 var visit func(sensor,int);visit=func(n sensor,depth int){if depth>32{return};for key,id:=range c.cfg.Sensors{if id==""||n.SensorID!=id&&fmt.Sprint(n.ID)!=id{continue};p,ok:=fields[key];if !ok{continue};var value float64;var err error;switch v:=n.Value.(type){case float64:value=v;case string:parts:=strings.Fields(v);if len(parts)==0{continue};numeric:=parts[0];if strings.Contains(numeric,",")&&!strings.Contains(numeric,"."){numeric=strings.ReplaceAll(numeric,",",".")};value,err=strconv.ParseFloat(numeric,64);if strings.HasSuffix(v,"GHz"){value*=1000}else if strings.HasSuffix(v,"GB"){value*=1024*1024*1024}else if strings.HasSuffix(v,"MB"){value*=1024*1024};default:continue};if err==nil{*p=protocol.Number(value)}};for _,child:=range n.Children{visit(child,depth+1)}};visit(root,0)
}
