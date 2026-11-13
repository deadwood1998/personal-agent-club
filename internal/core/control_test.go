package core

import "testing"

func TestDecide(t *testing.T) {
	type row struct {
		level      Control
		op         Operation
		unattended bool
		want       Verdict
	}
	var rows []row
	reads := []Operation{OpList, OpRead}
	writes := []Operation{OpSend, OpStart, OpInterrupt}
	for _, unattended := range []bool{false, true} {
		for _, op := range reads {
			rows = append(rows,
				row{ControlOff, op, unattended, Deny},
				row{ControlRead, op, unattended, Allow},
				row{ControlConfirm, op, unattended, Allow},
				row{ControlAuto, op, unattended, Allow},
			)
		}
		for _, op := range writes {
			want := func(attended Verdict) Verdict {
				if unattended {
					return Deny
				}
				return attended
			}
			rows = append(rows,
				row{ControlOff, op, unattended, Deny},
				row{ControlRead, op, unattended, Deny},
				row{ControlConfirm, op, unattended, want(Confirm)},
				row{ControlAuto, op, unattended, want(Allow)},
			)
		}
		for _, level := range []Control{ControlOff, ControlRead, ControlConfirm, ControlAuto, "", "bogus"} {
			rows = append(rows, row{level, OpManage, unattended, Deny}, row{level, "bogus", unattended, Deny})
		}
		rows = append(rows, row{"", OpRead, unattended, Deny}, row{"bogus", OpRead, unattended, Deny})
	}
	for _, r := range rows {
		if got := Decide(r.level, r.op, r.unattended); got != r.want {
			t.Errorf("Decide(%q, %q, unattended=%v) = %s, want %s", r.level, r.op, r.unattended, got, r.want)
		}
	}
}

func TestParseControl(t *testing.T) {
	if c, err := ParseControl(""); err != nil || c != ControlOff {
		t.Fatalf("default: %q %v", c, err)
	}
	for _, s := range []string{"off", "read", "confirm", "auto"} {
		if c, err := ParseControl(s); err != nil || string(c) != s {
			t.Fatalf("%s: %q %v", s, c, err)
		}
	}
	if _, err := ParseControl("always"); err == nil {
		t.Fatal("unknown level accepted")
	}
}

func TestIDs(t *testing.T) {
	id := NewAccountID()
	if !ValidAccountID(id) {
		t.Fatalf("bad account id %q", id)
	}
	key := SessionKey("echo", id, "thread:1")
	p, a, e, ok := ParseSessionKey(key)
	if !ok || p != "echo" || a != id || e != "thread:1" {
		t.Fatalf("parse %q: %q %q %q %v", key, p, a, e, ok)
	}
	for _, bad := range []string{"", "agent:echo:a1", "session:echo:a1:x", "agent::a1:x"} {
		if _, _, _, ok := ParseSessionKey(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
	for name, want := range map[string]bool{"default": true, "u-1_x": true, "": false, "../x": false, "a/b": false, "-x": false} {
		if ValidSpace(name) != want {
			t.Errorf("ValidSpace(%q) != %v", name, want)
		}
	}
}
