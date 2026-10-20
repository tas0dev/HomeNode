//go:build !windows
package service
import("context";"errors";"log/slog")
func Protect([]byte)([]byte,error){return nil,errors.New("DPAPI enrollment requires Windows")}
func Unprotect([]byte)([]byte,error){return nil,errors.New("HomeNode Agent requires Windows")}
func Log(level,message string){slog.Info(message,"level",level)}
func Run(run func(context.Context)error)error{return errors.New("HomeNode Agent requires Windows")}
