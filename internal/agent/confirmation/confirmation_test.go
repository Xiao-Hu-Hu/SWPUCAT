package confirmation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func testSigner(t *testing.T) *Signer {
	t.Helper()
	signer, err := NewSigner(strings.Repeat("s", 32), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	signer.now = func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) }
	signer.random = strings.NewReader(strings.Repeat("n", 16))
	return signer
}

func TestProposalTokenBindsActorSessionToolAndParameters(t *testing.T) {
	signer := testSigner(t)
	proposal, err := signer.Propose(7, "session-7", ToolDraftAnnouncement, "创建公告草稿", "标题：通知", map[string]any{"title": "通知", "pinned": false})
	if err != nil {
		t.Fatal(err)
	}
	claims, canonical, err := signer.Verify(proposal.Token, 7, "session-7", ToolDraftAnnouncement, proposal.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if claims.ParametersDigest != ParametersDigest(canonical) || proposal.ExpiresAt != "2026-09-18T12:05:00Z" {
		t.Fatalf("claims/proposal = %+v %+v", claims, proposal)
	}

	var parameters map[string]any
	_ = json.Unmarshal(proposal.Parameters, &parameters)
	parameters["pinned"] = true
	tamperedParameters, _ := json.Marshal(parameters)
	for name, verify := range map[string]func() error{
		"user": func() error {
			_, _, err := signer.Verify(proposal.Token, 8, "session-7", ToolDraftAnnouncement, proposal.Parameters)
			return err
		},
		"session": func() error {
			_, _, err := signer.Verify(proposal.Token, 7, "other", ToolDraftAnnouncement, proposal.Parameters)
			return err
		},
		"tool": func() error {
			_, _, err := signer.Verify(proposal.Token, 7, "session-7", ToolClockIn, proposal.Parameters)
			return err
		},
		"parameters": func() error {
			_, _, err := signer.Verify(proposal.Token, 7, "session-7", ToolDraftAnnouncement, tamperedParameters)
			return err
		},
		"signature": func() error {
			_, _, err := signer.Verify(proposal.Token+"x", 7, "session-7", ToolDraftAnnouncement, proposal.Parameters)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := verify(); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("error = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func TestProposalTokenExpires(t *testing.T) {
	signer := testSigner(t)
	proposal, err := signer.Propose(7, "session-7", ToolClockIn, "上班打卡", "确认打卡", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	signer.now = func() time.Time { return time.Date(2026, 9, 18, 12, 5, 1, 0, time.UTC) }
	_, _, err = signer.Verify(proposal.Token, 7, "session-7", ToolClockIn, proposal.Parameters)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("error = %v, want ErrExpiredToken", err)
	}
}
