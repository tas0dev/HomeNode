//go:build windows

package power
import (
 "errors"
 "unsafe"
 "golang.org/x/sys/windows"
)
func Execute(operation string)error {
 if operation!="shutdown"&&operation!="restart"{return errors.New("unsupported power operation")}
 var token windows.Token;if e:=windows.OpenProcessToken(windows.CurrentProcess(),windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY,&token);e!=nil{return e};defer token.Close()
 name,_:=windows.UTF16PtrFromString("SeShutdownPrivilege");var luid windows.LUID;if e:=windows.LookupPrivilegeValue(nil,name,&luid);e!=nil{return e}
 privileges:=windows.Tokenprivileges{PrivilegeCount:1};privileges.Privileges[0]=windows.LUIDAndAttributes{Luid:luid,Attributes:windows.SE_PRIVILEGE_ENABLED}
 var previous windows.Tokenprivileges;var size uint32
 if e:=windows.AdjustTokenPrivileges(token,false,&privileges,uint32(unsafe.Sizeof(previous)),&previous,&size);e!=nil{return e}
 defer windows.AdjustTokenPrivileges(token,false,&previous,0,nil,nil)
 message,_:=windows.UTF16PtrFromString("HomeNodeから電源操作を要求しました。未保存の作業を保存してください。")
 reboot:=uintptr(0);if operation=="restart"{reboot=1}
 // No force-app-close flag. The 15 second grace period allows acknowledgment
 // delivery and gives interactive applications time to save their work.
 r,_,e:=windows.NewLazySystemDLL("advapi32.dll").NewProc("InitiateSystemShutdownExW").Call(0,uintptr(unsafe.Pointer(message)),15,0,reboot,0x80040000)
 if r==0{return e};return nil
}
