package core

import "fmt"

// Control is an account's assistant control level (assistant_control): how far the Persona assistant may drive the
// agents of that account through club's own tools.
type Control string

const (
	// ControlOff: the assistant does not see the account. The default for new and imported accounts.
	ControlOff Control = "off"
	// ControlRead: list and read.
	ControlRead Control = "read"
	// ControlConfirm: list and read; every send, start and interrupt is confirmed by the user first.
	ControlConfirm Control = "confirm"
	// ControlAuto: list and read; sends, starts and interrupts go without asking each time.
	ControlAuto Control = "auto"
)

// DefaultControl is the level of an account nobody has set.
const DefaultControl = ControlOff

// ParseControl reads a level; "" is the default.
func ParseControl(s string) (Control, error) {
	switch c := Control(s); c {
	case "":
		return DefaultControl, nil
	case ControlOff, ControlRead, ControlConfirm, ControlAuto:
		return c, nil
	}
	return "", fmt.Errorf("unknown assistant_control %q", s)
}

// Operation is something the assistant asks to do with an agent.
type Operation string

const (
	OpList      Operation = "list"
	OpRead      Operation = "read"
	OpSend      Operation = "send"
	OpStart     Operation = "start"
	OpInterrupt Operation = "interrupt"
	// OpManage is pairing, unpairing, replacing a credential, signing in or changing assistant_control itself:
	// never the assistant's, at any level.
	OpManage Operation = "manage"
)

// Writes says whether op changes anything at the agent.
func (op Operation) Writes() bool {
	return op == OpSend || op == OpStart || op == OpInterrupt || op == OpManage
}

// Verdict is what Decide allows.
type Verdict string

const (
	Allow   Verdict = "allow"
	Confirm Verdict = "confirm" // only after the user confirms this one call
	Deny    Verdict = "deny"
)

// Decide is whether the assistant may do op on an account at level. unattended is a call with nobody present to
// confirm (a scheduled task, a background follow-up): it may only read, at any level. Unknown levels and operations
// are denied.
func Decide(level Control, op Operation, unattended bool) Verdict {
	switch op {
	case OpList, OpRead, OpSend, OpStart, OpInterrupt:
	default:
		return Deny
	}
	if op.Writes() && unattended {
		return Deny
	}
	switch level {
	case ControlRead:
		if op.Writes() {
			return Deny
		}
		return Allow
	case ControlConfirm:
		if op.Writes() {
			return Confirm
		}
		return Allow
	case ControlAuto:
		return Allow
	}
	return Deny
}
