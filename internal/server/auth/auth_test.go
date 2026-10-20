package auth
import("strings";"testing";"time")
func TestPasswordHash(t *testing.T){h:=Hash("correct-password");if !Verify(h,"correct-password")||Verify(h,"wrong-password"){t.Fatal("verification failed")};if Verify(strings.Replace(h,"m=65536","m=999999999",1),"correct-password"){t.Fatal("unbounded Argon parameters accepted")}}
func TestSessionsAndLimiter(t *testing.T){s:=New();id,v:=s.Create();if len(v.CSRF)<32{t.Fatal("weak CSRF token")};if _,ok:=s.Get(id);!ok{t.Fatal("session missing")};s.Delete(id);if _,ok:=s.Get(id);ok{t.Fatal("logout did not invalidate session")};l:=NewLimiter();if !l.Allow("test",1,time.Minute)||l.Allow("test",1,time.Minute){t.Fatal("rate limit failed")}}
