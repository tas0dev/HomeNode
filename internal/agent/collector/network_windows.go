//go:build windows
package collector
import (
 "net"
 "golang.org/x/sys/windows"
 "github.com/tas0dev/HomeNode/internal/protocol"
)
func platformNetwork(s *protocol.Sample){
 iface,e:=net.InterfaceByName(s.Network.Interface);if e!=nil{return}
 row:=windows.MibIfRow2{InterfaceIndex:uint32(iface.Index)}
 if windows.GetIfEntry2Ex(windows.MibIfEntryNormalWithoutStatistics,&row)==nil&&row.ReceiveLinkSpeed>0{s.Network.LinkSpeed=protocol.Number(float64(row.ReceiveLinkSpeed))}
}
