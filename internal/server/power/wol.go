package power
import ("errors";"net";"time")
func Packet(mac string)([]byte,error){m,e:=net.ParseMAC(mac);if e!=nil||len(m)!=6{return nil,errors.New("invalid Ethernet MAC")};p:=make([]byte,6,102);for i:=range p{p[i]=255};for i:=0;i<16;i++{p=append(p,m...)};return p,nil}
func Wake(mac,destination string)error{p,e:=Packet(mac);if e!=nil{return e};a,e:=net.ResolveUDPAddr("udp4",destination);if e!=nil{return e};c,e:=net.DialUDP("udp4",nil,a);if e!=nil{return e};defer c.Close();c.SetWriteDeadline(time.Now().Add(3*time.Second));_,e=c.Write(p);return e}
