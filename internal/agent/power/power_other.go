//go:build !windows
package power
import "errors"
func Execute(string)error{return errors.New("power operations require Windows")}
