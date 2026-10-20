package state

import (
 "errors"
 "time"
)
const (
 Offline="OFFLINE"
 Online="ONLINE"
 Starting="STARTING"
 ShuttingDown="SHUTTING_DOWN"
 Restarting="RESTARTING"
 Unreachable="UNREACHABLE"
)
type Operation struct {
 ID string `json:"request_id"`
 Kind string `json:"operation"`
 Status string `json:"status"`
 Reason string `json:"reason,omitempty"`
 Actor string `json:"actor"`
 Started time.Time `json:"started_at"`
}
type Machine struct {
 State string `json:"state"`
 LastHeartbeat *time.Time `json:"last_heartbeat"`
 Operation *Operation `json:"operation"`
 Reach, Start, Restart, Shutdown time.Duration `json:"-"`
 connected, lost, acknowledged bool
 boot, requestBoot uint64
}
func New(reach,start,restart,shutdown time.Duration)*Machine{return &Machine{State:Offline,Reach:reach,Start:start,Restart:restart,Shutdown:shutdown}}
func (m *Machine) Busy()bool{return m.State==Starting||m.State==ShuttingDown||m.State==Restarting}
func (m *Machine) Request(kind,id,actor string,now time.Time)error {
 if m.Busy(){return errors.New("power operation in progress")}
 switch kind {
 case "on":if m.State!=Offline{return errors.New("power on requires OFFLINE")};m.State=Starting
 case "shutdown","restart":if m.State!=Online||!m.connected||m.LastHeartbeat==nil||now.Sub(*m.LastHeartbeat)>m.Reach{return errors.New("agent is not online")};if kind=="shutdown"{m.State=ShuttingDown}else{m.State=Restarting}
 default:return errors.New("unsupported operation")
 }
 m.Operation=&Operation{ID:id,Kind:kind,Status:"accepted",Actor:actor,Started:now.UTC()};m.lost=false;m.acknowledged=false;m.requestBoot=m.boot;return nil
}
func (m *Machine) Connected(boot uint64){m.connected=true;m.boot=boot}
func (m *Machine) Heartbeat(now time.Time){t:=now.UTC();m.LastHeartbeat=&t;m.connected=true
 switch m.State {
 case ShuttingDown:return
 case Restarting:if !m.lost||!m.acknowledged||m.boot==0||m.boot==m.requestBoot{return}
 }
 if m.Busy()&&m.Operation!=nil {m.Operation.Status="completed"}
 m.State=Online
}
func (m *Machine) Ack(id string,accepted bool,reason string)bool {
 if !m.Busy()||m.Operation==nil||m.Operation.ID!=id||m.Operation.Kind=="on"||m.acknowledged{return false}
 if !accepted {m.Fail(reason);return true}
 m.acknowledged=true;m.Operation.Status="agent_accepted";return true
}
func (m *Machine) Disconnected(){m.connected=false
 if m.State==ShuttingDown&&m.acknowledged {m.State=Offline;m.Operation.Status="disconnected";m.Operation.Reason="agent disconnected; physical power state is unverified";return}
 if m.State==Restarting {m.lost=true;return}
 if m.State==Online {m.State=Unreachable}
}
func (m *Machine) Fail(reason string){if m.Operation!=nil{m.Operation.Status="failed";m.Operation.Reason=reason};if m.connected&&m.LastHeartbeat!=nil&&time.Since(*m.LastHeartbeat)<m.Reach {m.State=Online}else{m.State=Offline}}
func (m *Machine) Tick(now time.Time){
 if m.Busy() {
  limit:=m.Start;if m.State==Restarting{limit=m.Restart};if m.State==ShuttingDown{limit=m.Shutdown}
  if now.Sub(m.Operation.Started)>limit {m.Fail("operation timed out")};return
 }
 if m.State==Online&&m.LastHeartbeat!=nil&&now.Sub(*m.LastHeartbeat)>m.Reach {m.State=Unreachable;m.connected=false}
}
