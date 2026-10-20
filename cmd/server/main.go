package main

import (
 "bufio"
 "context"
 "flag"
 "fmt"
 "log/slog"
 "net/http"
 "os"
 "os/signal"
 "strings"
 "syscall"
 "time"
 "github.com/tas0dev/HomeNode/internal/config"
 "github.com/tas0dev/HomeNode/internal/server"
 "github.com/tas0dev/HomeNode/internal/server/auth"
 "github.com/tas0dev/HomeNode/web"
)
func main(){if e:=run();e!=nil{slog.Error("HomeNode stopped", "error",e);os.Exit(1)}}
func run()error{
 path:=flag.String("config","config.yaml","configuration file");initDir:=flag.String("init-secrets","","initialize credentials in this directory (password on stdin)");flag.Parse()
 if *initDir!=""{fmt.Fprintln(os.Stderr,"管理者パスワードを標準入力から読み込みます（12文字以上）。");r:=bufio.NewReaderSize(os.Stdin,2048);password,e:=r.ReadString('\n');if e!=nil&&len(password)==0{return e};return auth.Init(*initDir,[]byte(strings.TrimRight(password,"\r\n")))}
 c,e:=config.Load(*path,false);if e!=nil{return e};s,e:=server.New(c,web.Assets());if e!=nil{return e}
 ctx,stop:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer stop();serverDone:=make(chan struct{});go func(){defer close(serverDone);s.Run(ctx)}()
 h:=&http.Server{Addr:c.Listen,Handler:s.Handler(),ReadHeaderTimeout:5*time.Second,ReadTimeout:15*time.Second,IdleTimeout:60*time.Second,MaxHeaderBytes:16<<10}
 serveError:=make(chan error,1);go func(){serveError<-h.ListenAndServe()}()
 select {
 case e=<-serveError:stop();<-serverDone;if e==http.ErrServerClosed{return nil};return e
 case <-ctx.Done():
  shutdown,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
  e=h.Shutdown(shutdown);<-serveError;<-serverDone;return e
 }
}
