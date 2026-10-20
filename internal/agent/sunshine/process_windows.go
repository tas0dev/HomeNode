//go:build windows

package sunshine

import (
 "context"
 "strings"
 "unsafe"
 "golang.org/x/sys/windows"
)

func processRunning(ctx context.Context)(bool,bool){
 snapshot,e:=windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS,0)
 if e!=nil{return false,false}
 defer windows.CloseHandle(snapshot)
 entry:=windows.ProcessEntry32{Size:uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
 e=windows.Process32First(snapshot,&entry)
 for e==nil {
  if ctx.Err()!=nil{return false,false}
  if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]),"sunshine.exe"){return true,true}
  e=windows.Process32Next(snapshot,&entry)
 }
 return false,e==windows.ERROR_NO_MORE_FILES
}
