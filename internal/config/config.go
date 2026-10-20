package config

import (
 "errors"
 "fmt"
 "net"
 "net/url"
 "os"
 "strings"
 "time"
 "gopkg.in/yaml.v3"
)
type Config struct {
 Listen string `yaml:"listen"`
 PublicURL string `yaml:"public_url"`
 PCName string `yaml:"pc_name"`
 MAC string `yaml:"mac"`
 Broadcast string `yaml:"broadcast"`
 AgentID string `yaml:"agent_id"`
 SecretsDir string `yaml:"secrets_dir"`
 LogDir string `yaml:"log_dir"`
 LogLevel string `yaml:"log_level"`
 LogBytes int64 `yaml:"log_max_bytes"`
 LogDays int `yaml:"log_retention_days"`
 Heartbeat time.Duration `yaml:"heartbeat_interval"`
 ReachTimeout time.Duration `yaml:"reachability_timeout"`
 StartTimeout time.Duration `yaml:"startup_timeout"`
 RestartTimeout time.Duration `yaml:"restart_timeout"`
 ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
 SampleInterval time.Duration `yaml:"telemetry_interval"`
 HistorySamples int `yaml:"history_samples"`
 ServerURL string `yaml:"server_url"`
 Interface string `yaml:"network_interface"`
 SensorURL string `yaml:"sensor_url"`
 Sensors map[string]string `yaml:"sensors"`
 GPUModel string `yaml:"gpu_model"`
 SunshinePort int `yaml:"sunshine_port"`
 TailnetAddress string `yaml:"tailnet_address"`
 ReconnectMax time.Duration `yaml:"reconnect_max"`
}
func Load(path string, agent bool) (Config,error) {
 c:=Config{Listen:"127.0.0.1:8080",PCName:"Windows PC",Broadcast:"255.255.255.255:9",AgentID:"desktop",SecretsDir:"secrets",LogDir:"data/logs",LogLevel:"INFO",LogBytes:20<<20,LogDays:7,Heartbeat:5*time.Second,ReachTimeout:15*time.Second,StartTimeout:120*time.Second,RestartTimeout:180*time.Second,ShutdownTimeout:120*time.Second,SampleInterval:time.Second,HistorySamples:120,SunshinePort:47989,ReconnectMax:30*time.Second}
 f,e:=os.Open(path); if e!=nil{return c,e}; defer f.Close()
 d:=yaml.NewDecoder(f);d.KnownFields(true); if e=d.Decode(&c);e!=nil{return c,e}
 if c.AgentID==""||len(c.AgentID)>128||c.Heartbeat<time.Second||c.SampleInterval<time.Second||c.ReconnectMax<time.Second||c.ReachTimeout<c.Heartbeat*2||c.StartTimeout<c.ReachTimeout||c.RestartTimeout<c.ReachTimeout||c.ShutdownTimeout<c.ReachTimeout||c.HistorySamples<60||c.HistorySamples>3600||c.LogBytes<1<<20||c.LogDays<1||c.LogDays>365 {return c,errors.New("invalid configuration limits")}
 if agent {
  u,e:=url.Parse(c.ServerURL); if e!=nil||u.Scheme!="https"||u.Host==""||u.User!=nil||u.RawQuery!=""||u.Path!="" {return c,errors.New("server_url must be an HTTPS origin")}
  if c.SunshinePort<1||c.SunshinePort>65535{return c,errors.New("invalid Sunshine port")}
  if c.SensorURL!="" {u,e:=url.Parse(c.SensorURL);if e!=nil||u.Scheme!="http"||!loopback(u.Hostname())||u.User!=nil {return c,errors.New("sensor_url must be loopback HTTP")}}
 } else {
  host,_,e:=net.SplitHostPort(c.Listen);if e!=nil||!loopback(host){return c,errors.New("listen must be a loopback address")}
  u,e:=url.Parse(c.PublicURL);if e!=nil||u.Scheme!="https"||u.Host==""||u.User!=nil||u.RawQuery!=""||strings.Trim(u.Path,"/")!="" {return c,errors.New("public_url must be the Tailscale HTTPS origin")}
  if mac,e:=net.ParseMAC(c.MAC);e!=nil{return c,fmt.Errorf("mac: %w",e)}else if len(mac)!=6{return c,errors.New("mac must be an Ethernet address")}
  if _,e:=net.ResolveUDPAddr("udp4",c.Broadcast);e!=nil{return c,e}
 }
 return c,nil
}
func loopback(s string) bool { ip:=net.ParseIP(s);return ip!=nil&&ip.IsLoopback() }
