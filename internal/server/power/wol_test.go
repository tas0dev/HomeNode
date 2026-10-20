package power
import("bytes";"testing")
func TestMagicPacket(t *testing.T){p,e:=Packet("00:11:22:33:44:55");if e!=nil{t.Fatal(e)};if len(p)!=102||!bytes.Equal(p[:6],bytes.Repeat([]byte{255},6)){t.Fatal("bad header")};for i:=0;i<16;i++{if !bytes.Equal(p[6+i*6:12+i*6],[]byte{0,17,34,51,68,85}){t.Fatal("bad payload")}};if _,e=Packet("invalid");e==nil{t.Fatal("invalid MAC accepted")}}
