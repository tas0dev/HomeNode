package main
import(
 "bufio"
 "flag"
 "fmt"
 "log/slog"
 "os"
 "path/filepath"
 "strings"
 "github.com/tas0dev/HomeNode/internal/agent"
 "github.com/tas0dev/HomeNode/internal/agent/service"
 "github.com/tas0dev/HomeNode/internal/config"
)
func main(){if e:=run();e!=nil{slog.Error("HomeNode Agent stopped","error",e);os.Exit(1)}}
func run()error{path:=flag.String("config","agent.yaml","configuration file");enroll:=flag.Bool("enroll-token",false,"protect token read from stdin with machine DPAPI");flag.Parse();c,e:=config.Load(*path,true);if e!=nil{return e};if *enroll{b,e:=bufio.NewReaderSize(os.Stdin,2048).ReadString('\n');if e!=nil&&b==""{return e};token:=strings.TrimSpace(b);if len(token)<32||len(token)>1024{return fmt.Errorf("invalid token length")};protected,e:=service.Protect([]byte(token));if e!=nil{return e};if e=os.MkdirAll(c.SecretsDir,0700);e!=nil{return e};f,e:=os.OpenFile(filepath.Join(c.SecretsDir,"agent.token.dpapi"),os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if e!=nil{return e};defer f.Close();_,e=f.Write(protected);return e};a,e:=agent.New(c);if e!=nil{return e};return service.Run(a.Run)}
