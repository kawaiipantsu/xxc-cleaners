package protect

import "testing"

func TestBlocked(t *testing.T) {
	s := New([]string{
		"/etc", "/var/lib/mysql", "/var/lib/postgresql", "/root/.ssh",
	})

	blocked := []string{
		"/",                           // bare root
		"/etc",                        // exact protected
		"/etc/passwd",                 // inside protected
		"/var/lib/mysql/ibdata1",      // inside protected
		"/var/lib",                    // ancestor of a protected path
		"/var",                        // ancestor of a protected path
		"/root/.ssh/id_rsa",           // inside protected
		"relative/path",               // relative paths are refused
		"",                            // empty
	}
	for _, p := range blocked {
		if ok, _ := s.Blocked(p); !ok {
			t.Errorf("expected %q to be BLOCKED, but it was allowed", p)
		}
	}

	allowed := []string{
		"/tmp/foo",
		"/tmp",
		"/home/user/.cache/pip",
		"/var/cache/apt/archives/x.deb",
		"/var/log/syslog.1",
		"/root/.cache/go-build",
	}
	for _, p := range allowed {
		if ok, which := s.Blocked(p); ok {
			t.Errorf("expected %q to be ALLOWED, but it was blocked by %q", p, which)
		}
	}
}

func TestRootAlwaysProtected(t *testing.T) {
	s := New(nil)
	if ok, _ := s.Blocked("/"); !ok {
		t.Fatal("root / must always be protected")
	}
}
