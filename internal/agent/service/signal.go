package service
import("os";"os/signal")
func watchInterrupt(c chan os.Signal){signal.Notify(c,os.Interrupt)}
