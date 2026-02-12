package coordination

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MessageType represents the type of coordination message.
type MessageType string

const (
	MsgClaim        MessageType = "claim"
	MsgRelease      MessageType = "release"
	MsgCommitted    MessageType = "committed"
	MsgPushed       MessageType = "pushed"
	MsgPRCreated    MessageType = "pr_created"
	MsgMerged       MessageType = "merged"
	MsgRebaseNeeded MessageType = "rebase_needed"
)

// Message represents a single coordination message on the bus.
type Message struct {
	Type      MessageType       `json:"type"`
	Agent     string            `json:"agent"`
	Timestamp time.Time         `json:"timestamp"`
	Data      map[string]string `json:"data,omitempty"`
}

// Publish appends a message to the bus (messages.jsonl).
func Publish(repoURL string, msg Message) error {
	dir, err := CoordDir(repoURL)
	if err != nil {
		return err
	}

	msg.Timestamp = time.Now()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("cannot marshal message: %w", err)
	}
	data = append(data, '\n')

	messagesPath := filepath.Join(dir, "messages.jsonl")
	f, err := os.OpenFile(messagesPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("cannot open messages.jsonl: %w", err)
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// ReadMessages reads all messages from the bus.
func ReadMessages(repoURL string) ([]Message, error) {
	dir, err := CoordDir(repoURL)
	if err != nil {
		return nil, err
	}

	return readMessagesFromDir(dir)
}

// ReadMessagesSince reads messages from the bus that occurred after the given time.
func ReadMessagesSince(repoURL string, since time.Time) ([]Message, error) {
	all, err := ReadMessages(repoURL)
	if err != nil {
		return nil, err
	}

	var filtered []Message
	for _, msg := range all {
		if msg.Timestamp.After(since) {
			filtered = append(filtered, msg)
		}
	}
	return filtered, nil
}

// ReadMessagesForAgent reads messages relevant to a specific agent.
func ReadMessagesForAgent(repoURL, agentName string) ([]Message, error) {
	all, err := ReadMessages(repoURL)
	if err != nil {
		return nil, err
	}

	var filtered []Message
	for _, msg := range all {
		// Include messages FROM this agent and messages that affect this agent
		if msg.Agent == agentName || isRelevantToAgent(msg, agentName) {
			filtered = append(filtered, msg)
		}
	}
	return filtered, nil
}

// HasRebaseNeeded checks if any rebase_needed message exists for the given agent
// since the specified time.
func HasRebaseNeeded(repoURL, agentName string, since time.Time) (bool, error) {
	msgs, err := ReadMessagesSince(repoURL, since)
	if err != nil {
		return false, err
	}

	for _, msg := range msgs {
		if msg.Type == MsgRebaseNeeded {
			// Check if this rebase message targets this agent
			if target, ok := msg.Data["target"]; ok && target == agentName {
				return true, nil
			}
			// Or if it's a broadcast rebase_needed (no specific target)
			if _, ok := msg.Data["target"]; !ok {
				return true, nil
			}
		}
	}
	return false, nil
}

func readMessagesFromDir(dir string) ([]Message, error) {
	messagesPath := filepath.Join(dir, "messages.jsonl")
	f, err := os.Open(messagesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot open messages.jsonl: %w", err)
	}
	defer f.Close()

	var messages []Message
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue // skip malformed lines
		}
		messages = append(messages, msg)
	}

	return messages, scanner.Err()
}

// isRelevantToAgent checks if a message is relevant to a specific agent.
// Broadcast messages (like rebase_needed without a target) are relevant to all.
func isRelevantToAgent(msg Message, agentName string) bool {
	if msg.Type == MsgRebaseNeeded {
		target, ok := msg.Data["target"]
		return !ok || target == agentName
	}
	// pushed/committed/merged events are relevant to all agents on the same repo
	switch msg.Type {
	case MsgPushed, MsgMerged:
		return true
	}
	return false
}
