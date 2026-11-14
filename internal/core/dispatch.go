package core

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/nwverse/personal-agent-club/internal/protocol"
)

// Call answers one protocol call in a space: method is the full name (agentsrc.status, club.info, …), params its
// JSON (empty or null for none). Errors are *protocol.Error for the four protocol codes; anything else carries only a
// message. Both shells answer through here.
func (c *Club) Call(ctx context.Context, space, method string, params json.RawMessage) (any, error) {
	if method == protocol.MethodClubInfo {
		return c.Info(), nil
	}
	ns, name, _ := strings.Cut(method, ".")
	if ns != protocol.AgentSourceNamespace && ns != protocol.ClubNamespace {
		return nil, protocol.Unsupported("unknown method " + method)
	}
	if ns == protocol.ClubNamespace && !slices.Contains(clubMethods, method) {
		return nil, protocol.Unsupported("unknown method " + method)
	}
	if ns == protocol.AgentSourceNamespace && !slices.Contains(protocol.AgentSourceMethods, name) {
		return nil, protocol.Unsupported("unknown method " + method)
	}
	s, err := c.Space(ctx, space)
	if err != nil {
		return nil, err
	}
	res, err := s.call(ctx, method, name, params)
	if err != nil && !errorIsProtocol(err) {
		c.opts.Log.Warn("call failed", "space", space, "method", method, "err", err)
	}
	return res, err
}

// clubMethods are the club.* methods this version knows (answered or stubbed).
var clubMethods = append(slices.Clone(protocol.ClubMethods), protocol.ClubStubMethods...)

func (s *Space) call(ctx context.Context, method, name string, params json.RawMessage) (any, error) {
	if strings.HasPrefix(method, protocol.ClubNamespace+".") {
		return s.callClub(ctx, method, params)
	}
	switch name {
	case protocol.MethodStatus:
		return s.Status(ctx)
	case protocol.MethodSessions:
		return s.Sessions(ctx)
	case protocol.MethodMessages:
		var p protocol.MessagesParams
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		return s.Messages(ctx, p)
	case protocol.MethodSend:
		var p protocol.SendParams
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		return s.Send(ctx, p)
	case protocol.MethodDeleteMessage:
		var p protocol.DeleteMessageParams
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		return protocol.Empty{}, s.DeleteMessage(ctx, p)
	case protocol.MethodMarkRead:
		var p protocol.MarkReadParams
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		return protocol.Empty{}, s.MarkRead(ctx, p)
	case protocol.MethodSearch:
		var p protocol.SearchParams
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		return s.Search(ctx, p)
	}
	return nil, protocol.Unsupported("unknown method " + method)
}

func (s *Space) callClub(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case protocol.MethodClubAccountsAdd:
		var p protocol.AddAccountParams
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		return s.AddAccount(ctx, p)
	case protocol.MethodClubAccountsRemove:
		var p protocol.RemoveAccountParams
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		return protocol.Empty{}, s.RemoveAccount(ctx, p)
	case protocol.MethodClubAccountsSync:
		var p protocol.AccountParams
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		if err := s.SyncAccount(ctx, p.AccountID); err != nil {
			return nil, err
		}
		return s.Account(ctx, p.AccountID)
	}
	return nil, protocol.Unsupported(method + " is not implemented yet")
}

func decode(params json.RawMessage, v any) error {
	params = bytes.TrimSpace(params)
	if len(params) == 0 || string(params) == "null" {
		return nil
	}
	if err := json.Unmarshal(params, v); err != nil {
		return protocol.Invalid("invalid params: " + err.Error())
	}
	return nil
}
