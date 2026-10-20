//go:build !windows

package collector
import "github.com/tas0dev/HomeNode/internal/protocol"
type gpuCounters struct{}
func newGPU()*gpuCounters{return &gpuCounters{}}
func(*gpuCounters) collect(*protocol.Sample){}
func(*gpuCounters) close(){}
