package logging
import("testing";"time")
func TestPersistenceFiltersAndPagination(t *testing.T){dir:=t.TempDir();s,e:=New(dir,1<<20,7,"DEBUG");if e!=nil{t.Fatal(e)};for i:=0;i<55;i++{s.Add("INFO","SERVER","test.event","event",nil)};s.Add("ERROR","AGENT","agent.failed","failed",nil)
 first,more:=s.List("","",1);if len(first)!=50||!more||first[0].ID!="agent.failed"{t.Fatal("page ordering failed")};filtered,more:=s.List("ERROR","AGENT",1);if len(filtered)!=1||more{t.Fatal("filter failed")}
 restored,e:=New(dir,1<<20,7,"INFO");if e!=nil{t.Fatal(e)};next,_:=restored.List("INFO","SERVER",2);if len(next)!=5{t.Fatal("restart persistence failed")}
 restored.items[0].Timestamp=time.Now().Add(-8*24*time.Hour);restored.trim();if len(restored.items)!=55{t.Fatal("retention cutoff failed")}
}
