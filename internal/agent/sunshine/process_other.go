//go:build !windows

package sunshine

import (
 "context"
 "strings"
 "github.com/shirou/gopsutil/v4/process"
)
func processRunning(ctx context.Context)(bool,bool){
 items,e:=process.ProcessesWithContext(ctx);if e!=nil{return false,false}
 unknown:=false
 for _,p:=range items{name,e:=p.NameWithContext(ctx);if e!=nil{unknown=true;continue};if strings.EqualFold(name,"sunshine"){return true,true}}
 return false,!unknown
}
