package collector
import (
 "context"
 "net/http"
 "net/http/httptest"
 "testing"
 "github.com/tas0dev/HomeNode/internal/config"
 "github.com/tas0dev/HomeNode/internal/protocol"
)
func TestSensorTreeUsesExplicitIDs(t *testing.T){
 h:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Type","application/json");w.Write([]byte(`{"id":0,"Children":[{"id":1,"SensorId":"/cpu/temp","Value":"52.5 °C"},{"id":2,"SensorId":"/gpu/power","Value":"-1 W"},{"id":3,"SensorId":"/other/temp","Value":"99 °C"}]}`))}));defer h.Close()
 c:=New(config.Config{SensorURL:h.URL,Sensors:map[string]string{"cpu.temperature_c":"/cpu/temp","gpu.power_w":"/gpu/power"}});defer c.Close();s:=protocol.Sample{Availability:map[string]string{}};c.sensors(context.Background(),&s);s.Sanitize();if s.CPU.Temperature==nil||*s.CPU.Temperature!=52.5{t.Fatal("sensor ID selection failed")};if s.GPU.Power!=nil{t.Fatal("negative sensor value accepted")}
}
