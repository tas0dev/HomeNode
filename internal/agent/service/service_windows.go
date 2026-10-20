//go:build windows
package service

import (
 "context"
 "errors"
 "log/slog"
 "os"
 "unsafe"
 "golang.org/x/sys/windows"
 "golang.org/x/sys/windows/svc"
 "golang.org/x/sys/windows/svc/eventlog"
)
type blob struct{Size uint32;Data *byte}
func protect(data []byte,encrypt bool)([]byte,error){
 if len(data)==0{return nil,errors.New("empty secret")};input:=blob{uint32(len(data)),&data[0]};var output blob;proc:=windows.NewLazySystemDLL("crypt32.dll").NewProc("CryptProtectData");flags:=uintptr(0x4|0x1);if !encrypt{proc=windows.NewLazySystemDLL("crypt32.dll").NewProc("CryptUnprotectData");flags=1}
 r,_,e:=proc.Call(uintptr(unsafe.Pointer(&input)),0,0,0,0,flags,uintptr(unsafe.Pointer(&output)));if r==0{return nil,e};defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))));out:=append([]byte(nil),unsafe.Slice(output.Data,int(output.Size))...);return out,nil
}
func Protect(data []byte)([]byte,error){return protect(data,true)}
func Unprotect(data []byte)([]byte,error){return protect(data,false)}
func Log(level,message string){l,e:=eventlog.Open("HomeNodeAgent");if e!=nil{slog.Info(message,"level",level);return};defer l.Close();switch level{case "ERROR":l.Error(3,message);case "WARN":l.Warning(2,message);default:l.Info(1,message)}}
type handler struct{run func(context.Context)error}
func(h handler) Execute(args []string,requests <-chan svc.ChangeRequest,status chan<- svc.Status)(bool,uint32){
 status<-svc.Status{State:svc.StartPending};ctx,cancel:=context.WithCancel(context.Background());defer cancel();done:=make(chan error,1);go func(){done<-h.run(ctx)}();current:=svc.Status{State:svc.Running,Accepts:svc.AcceptStop|svc.AcceptShutdown};status<-current
 for{select{case e:=<-done:if e!=nil{Log("ERROR","Agentサービスを開始できませんでした");return true,1};return false,0
 case req:=<-requests:switch req.Cmd {case svc.Interrogate:status<-current;case svc.Stop,svc.Shutdown:status<-svc.Status{State:svc.StopPending,WaitHint:15000};cancel();<-done;return false,0}}}
}
func Run(run func(context.Context)error)error{is,e:=svc.IsWindowsService();if e!=nil{return e};if is{return svc.Run("HomeNodeAgent",handler{run})};ctx,cancel:=context.WithCancel(context.Background());defer cancel();interrupt:=make(chan os.Signal,1);watchInterrupt(interrupt);go func(){<-interrupt;cancel()}();return run(ctx)}
