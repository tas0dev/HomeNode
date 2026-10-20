//go:build !windows
package collector
import "github.com/tas0dev/HomeNode/internal/protocol"
func platformNetwork(*protocol.Sample){}
