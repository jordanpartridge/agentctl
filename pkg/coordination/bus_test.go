package coordination

import (
	"os"
	"testing"
	"time"
)

func TestPublishAndReadMessages(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	msg := Message{
		Type:  MsgCommitted,
		Agent: "agent-1",
		Data:  map[string]string{"sha": "abc123"},
	}

	if err := Publish(repoURL, msg); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	msgs, err := ReadMessages(repoURL)
	if err != nil {
		t.Fatalf("ReadMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	if msgs[0].Type != MsgCommitted {
		t.Errorf("expected type committed, got %s", msgs[0].Type)
	}
	if msgs[0].Agent != "agent-1" {
		t.Errorf("expected agent agent-1, got %s", msgs[0].Agent)
	}
	if msgs[0].Data["sha"] != "abc123" {
		t.Errorf("expected sha abc123, got %s", msgs[0].Data["sha"])
	}
	if msgs[0].Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
}

func TestPublishMultipleMessages(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	types := []MessageType{MsgCommitted, MsgPushed, MsgPRCreated}
	for _, mt := range types {
		Publish(repoURL, Message{Type: mt, Agent: "agent-1"})
	}

	msgs, _ := ReadMessages(repoURL)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	for i, mt := range types {
		if msgs[i].Type != mt {
			t.Errorf("message %d: expected type %s, got %s", i, mt, msgs[i].Type)
		}
	}
}

func TestReadMessagesSince(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	Publish(repoURL, Message{Type: MsgCommitted, Agent: "agent-1"})

	cutoff := time.Now()
	time.Sleep(10 * time.Millisecond)

	Publish(repoURL, Message{Type: MsgPushed, Agent: "agent-1"})

	msgs, err := ReadMessagesSince(repoURL, cutoff)
	if err != nil {
		t.Fatalf("ReadMessagesSince failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message since cutoff, got %d", len(msgs))
	}
	if msgs[0].Type != MsgPushed {
		t.Errorf("expected pushed, got %s", msgs[0].Type)
	}
}

func TestReadMessagesForAgent(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	Publish(repoURL, Message{Type: MsgCommitted, Agent: "agent-1"})
	Publish(repoURL, Message{Type: MsgCommitted, Agent: "agent-2"})
	Publish(repoURL, Message{Type: MsgPushed, Agent: "agent-2"}) // broadcast, relevant to all

	msgs, err := ReadMessagesForAgent(repoURL, "agent-1")
	if err != nil {
		t.Fatalf("ReadMessagesForAgent failed: %v", err)
	}
	// agent-1's own committed + agent-2's pushed (broadcast)
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages for agent-1, got %d", len(msgs))
	}
}

func TestHasRebaseNeeded(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	since := time.Now()
	time.Sleep(10 * time.Millisecond)

	// No rebase needed initially
	needed, err := HasRebaseNeeded(repoURL, "agent-1", since)
	if err != nil {
		t.Fatalf("HasRebaseNeeded failed: %v", err)
	}
	if needed {
		t.Error("should not need rebase initially")
	}

	// Broadcast rebase_needed
	Publish(repoURL, Message{Type: MsgRebaseNeeded, Agent: "agent-2"})

	needed, _ = HasRebaseNeeded(repoURL, "agent-1", since)
	if !needed {
		t.Error("should need rebase after broadcast")
	}
}

func TestHasRebaseNeededTargeted(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	since := time.Now()
	time.Sleep(10 * time.Millisecond)

	// Targeted rebase for agent-1 only
	Publish(repoURL, Message{
		Type:  MsgRebaseNeeded,
		Agent: "agent-2",
		Data:  map[string]string{"target": "agent-1"},
	})

	needed, _ := HasRebaseNeeded(repoURL, "agent-1", since)
	if !needed {
		t.Error("agent-1 should need rebase")
	}

	needed, _ = HasRebaseNeeded(repoURL, "agent-3", since)
	if needed {
		t.Error("agent-3 should not need rebase (not targeted)")
	}
}

func TestReadMessagesEmpty(t *testing.T) {
	repoURL := "https://github.com/test/" + t.Name()
	dir, err := Init(repoURL)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer os.RemoveAll(dir)

	msgs, err := ReadMessages(repoURL)
	if err != nil {
		t.Fatalf("ReadMessages on empty bus failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages on empty bus, got %d", len(msgs))
	}
}

func TestMessageTypes(t *testing.T) {
	// Verify all expected message types exist
	types := []MessageType{
		MsgClaim, MsgRelease, MsgCommitted,
		MsgPushed, MsgPRCreated, MsgMerged, MsgRebaseNeeded,
	}
	expected := []string{
		"claim", "release", "committed",
		"pushed", "pr_created", "merged", "rebase_needed",
	}
	for i, mt := range types {
		if string(mt) != expected[i] {
			t.Errorf("expected %s, got %s", expected[i], mt)
		}
	}
}
