package state

import (
 "testing"
 "time"
)
func machine()*Machine{return New(15*time.Second,120*time.Second,180*time.Second,120*time.Second)}
func TestHeartbeatRequired(t *testing.T){m:=machine();m.Connected(1);if m.State!=Offline{t.Fatal("connection must not imply ONLINE")};m.Heartbeat(time.Now());if m.State!=Online{t.Fatal(m.State)}}
func TestRestartRequiresDifferentBoot(t *testing.T){
 m:=machine();now:=time.Now();m.Connected(1);m.Heartbeat(now)
 if e:=m.Request("restart","id","admin",now);e!=nil{t.Fatal(e)}
 if e:=m.Request("shutdown","duplicate","admin",now);e==nil{t.Fatal("concurrent operation accepted")}
 m.Ack("id",true,"");m.Heartbeat(now);if m.State!=Restarting{t.Fatal("completed before disconnect")}
 m.Disconnected();m.Connected(1);m.Heartbeat(now);if m.State!=Restarting{t.Fatal("same boot incorrectly completed restart")}
 m.Disconnected();m.Connected(2);m.Heartbeat(now);if m.State!=Online||m.Operation.Status!="completed"{t.Fatal("new boot not confirmed")}
}
func TestShutdownAcknowledgmentAndReachability(t *testing.T){
 m:=machine();now:=time.Now();m.Connected(1);m.Heartbeat(now);m.Request("shutdown","id","admin",now)
 if m.Ack("wrong",true,""){t.Fatal("unmatched ACK accepted")};m.Ack("id",true,"");m.Disconnected();if m.State!=Offline||m.Operation.Status!="disconnected"{t.Fatal("shutdown should only report disconnection")}
 m.Connected(1);m.Heartbeat(now);m.Tick(now.Add(16*time.Second));if m.State!=Unreachable{t.Fatal(m.State)}
 if e:=m.Request("on","unsafe","admin",now);e==nil{t.Fatal("power on allowed while unreachable")}
}
func TestStartTimeoutAndFailure(t *testing.T){m:=machine();now:=time.Now();if e:=m.Request("on","id","admin",now);e!=nil{t.Fatal(e)};m.Tick(now.Add(121*time.Second));if m.State!=Offline||m.Operation.Status!="failed"{t.Fatal("startup did not time out")}}
func TestNoShutdownCompletionWithoutAck(t *testing.T){m:=machine();now:=time.Now();m.Connected(1);m.Heartbeat(now);m.Request("shutdown","id","admin",now);m.Disconnected();if m.State!=ShuttingDown{t.Fatal("unacknowledged shutdown completed")};m.Tick(now.Add(121*time.Second));if m.Operation.Status!="failed"{t.Fatal("missing ACK not failed")}}
