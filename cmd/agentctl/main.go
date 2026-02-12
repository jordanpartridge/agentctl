package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jordanpartridge/agentctl/pkg/container"
	"github.com/jordanpartridge/agentctl/pkg/coordination"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "spawn":
		if len(os.Args) < 4 {
			fmt.Println("Usage: agentctl spawn <name> <repo> [branch]")
			os.Exit(1)
		}
		branch := "main"
		if len(os.Args) > 4 {
			branch = os.Args[4]
		}
		agent, err := container.Spawn(os.Args[2], os.Args[3], branch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("🤖 Agent: %s\n📦 Container: %s\n🌐 Port: %d\n", agent.Name, agent.ContainerID[:12], agent.Port)

	case "run":
		// Run until done: agentctl run <name> <task> [max-attempts]
		if len(os.Args) < 4 {
			fmt.Println("Usage: agentctl run <name> <task> [max-attempts]")
			fmt.Println("  Runs Claude repeatedly until task is complete (tests pass, changes committed)")
			os.Exit(1)
		}
		name := os.Args[2]
		task := os.Args[3]
		maxAttempts := 10
		if len(os.Args) > 4 {
			if n, err := strconv.Atoi(os.Args[4]); err == nil {
				maxAttempts = n
			}
		}

		fmt.Printf("🚀 Running agent %s until done (max %d attempts)\n", name, maxAttempts)
		fmt.Printf("📋 Task: %s\n", task)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		result, err := container.RunUntilDone(name, task, maxAttempts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}

		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("✅ Completed in %d attempts\n", result.Attempts)

	case "check":
		// Check completion status
		if len(os.Args) < 3 {
			fmt.Println("Usage: agentctl check <name>")
			os.Exit(1)
		}
		status := container.CheckCompletion(os.Args[2])
		fmt.Printf("Tests: %s\n", status.TestStatus)
		fmt.Printf("Uncommitted changes: %v\n", status.HasUncommitted)
		fmt.Printf("Claude running: %v\n", status.ClaudeRunning)

		if status.TestStatus == "pass" && !status.HasUncommitted {
			fmt.Println("✅ Agent appears complete")
		} else {
			fmt.Println("⏳ Agent has pending work")
		}

	case "kill":
		if len(os.Args) < 3 {
			fmt.Println("Usage: agentctl kill <name>")
			os.Exit(1)
		}
		container.Kill(os.Args[2])

	case "list":
		agents, _ := container.List()
		if len(agents) == 0 {
			fmt.Println("No agents")
			return
		}
		for _, a := range agents {
			status := container.CheckCompletion(a.Name)
			indicator := "⏳"
			if status.TestStatus == "pass" && !status.HasUncommitted {
				indicator = "✅"
			} else if status.ClaudeRunning {
				indicator = "🔄"
			}
			fmt.Printf("%s %-15s %-12s port:%-5d %s\n", indicator, a.Name, a.ContainerID[:12], a.Port, a.Status)
		}

	case "status":
		if len(os.Args) < 3 {
			fmt.Println("Usage: agentctl status <name>")
			os.Exit(1)
		}
		container.Status(os.Args[2])

	case "logs":
		if len(os.Args) < 3 {
			fmt.Println("Usage: agentctl logs [-f] <name>")
			os.Exit(1)
		}
		// Check for -f flag
		if os.Args[2] == "-f" {
			if len(os.Args) < 4 {
				fmt.Println("Usage: agentctl logs -f <name>")
				os.Exit(1)
			}
			container.LogsFollow(os.Args[3])
		} else {
			container.Logs(os.Args[2])
		}

	case "spy":
		if len(os.Args) < 3 {
			fmt.Println("Usage: agentctl spy <name> [--raw] [--tools] [--thinking] [--verbose] [--json]")
			os.Exit(1)
		}
		name := ""
		opts := container.SpyOptions{}
		for _, arg := range os.Args[2:] {
			switch arg {
			case "--raw":
				opts.Raw = true
			case "--tools":
				opts.ToolsOnly = true
			case "--thinking":
				opts.Thinking = true
			case "--verbose":
				opts.Verbose = true
			case "--json":
				opts.JSON = true
			default:
				if !strings.HasPrefix(arg, "--") {
					name = arg
				}
			}
		}
		if name == "" {
			fmt.Println("Usage: agentctl spy <name> [--raw] [--tools] [--thinking] [--verbose] [--json]")
			os.Exit(1)
		}
		if err := container.Spy(name, opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "shell":
		if len(os.Args) < 3 {
			fmt.Println("Usage: agentctl shell <name>")
			os.Exit(1)
		}
		container.Shell(os.Args[2])

	case "diagnose":
		if len(os.Args) < 3 {
			fmt.Println("Usage: agentctl diagnose <name>")
			os.Exit(1)
		}
		info, err := container.Diagnose(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("🔍 Agent Diagnostics")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Claude status
		if info.ClaudeRunning {
			fmt.Println("🤖 Claude: Running")
		} else {
			fmt.Println("🤖 Claude: Not running")
		}
		fmt.Println()

		// Auth files
		fmt.Println("🔐 Auth Files:")
		for file, exists := range info.AuthFiles {
			if exists {
				fmt.Printf("   ✅ %s exists\n", file)
			} else {
				fmt.Printf("   ❌ %s missing\n", file)
			}
		}
		fmt.Println()

		// Available tools
		fmt.Println("🛠️  Available Tools:")
		fmt.Printf("   %s\n", strings.Join(info.AvailableTools, ", "))
		fmt.Println()

		// Disk space
		fmt.Println("💾 Disk Space:")
		for _, line := range strings.Split(info.DiskSpace, "\n") {
			fmt.Printf("   %s\n", line)
		}
		fmt.Println()

		// Running processes
		fmt.Println("📋 Running Processes:")
		for _, line := range strings.Split(info.Processes, "\n") {
			fmt.Printf("   %s\n", line)
		}
		fmt.Println()

		// Error logs
		fmt.Println("📜 Last 20 Lines of Error Logs:")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println(info.ErrorLogs)

	case "claim":
		// Claim a file: agentctl claim <agent> <repo-url> <file>
		if len(os.Args) < 5 {
			fmt.Println("Usage: agentctl claim <agent> <repo-url> <file>")
			os.Exit(1)
		}
		agentName := os.Args[2]
		repoURL := os.Args[3]
		filePath := os.Args[4]

		// Initialize coordination dir
		if _, err := coordination.Init(repoURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing coordination: %v\n", err)
			os.Exit(1)
		}

		if err := coordination.ClaimFile(repoURL, agentName, filePath); err != nil {
			fmt.Fprintf(os.Stderr, "Claim failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Claimed %s for agent %s\n", filePath, agentName)

	case "release":
		// Release a file: agentctl release <agent> <repo-url> <file>
		if len(os.Args) < 5 {
			fmt.Println("Usage: agentctl release <agent> <repo-url> <file>")
			os.Exit(1)
		}
		agentName := os.Args[2]
		repoURL := os.Args[3]
		filePath := os.Args[4]

		if err := coordination.ReleaseFile(repoURL, agentName, filePath); err != nil {
			fmt.Fprintf(os.Stderr, "Release failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Released %s from agent %s\n", filePath, agentName)

	case "notify":
		// Send a notification: agentctl notify <agent> <repo-url> <type> [key=value...]
		if len(os.Args) < 5 {
			fmt.Println("Usage: agentctl notify <agent> <repo-url> <type> [key=value...]")
			fmt.Println("  Types: committed, pushed, pr_created, merged, rebase_needed")
			os.Exit(1)
		}
		agentName := os.Args[2]
		repoURL := os.Args[3]
		msgType := coordination.MessageType(os.Args[4])

		// Parse optional key=value data
		data := make(map[string]string)
		for _, arg := range os.Args[5:] {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				data[parts[0]] = parts[1]
			}
		}

		// Initialize coordination dir
		if _, err := coordination.Init(repoURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing coordination: %v\n", err)
			os.Exit(1)
		}

		msg := coordination.Message{
			Type:  msgType,
			Agent: agentName,
			Data:  data,
		}
		if err := coordination.Publish(repoURL, msg); err != nil {
			fmt.Fprintf(os.Stderr, "Notify failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Published %s from agent %s\n", msgType, agentName)

	case "bus":
		// Show bus state: agentctl bus <repo-url> [--claims] [--messages] [--state]
		if len(os.Args) < 3 {
			fmt.Println("Usage: agentctl bus <repo-url> [--claims] [--messages] [--state]")
			os.Exit(1)
		}
		repoURL := os.Args[2]

		// Parse flags
		showClaims := false
		showMessages := false
		showState := false
		for _, arg := range os.Args[3:] {
			switch arg {
			case "--claims":
				showClaims = true
			case "--messages":
				showMessages = true
			case "--state":
				showState = true
			}
		}
		// If no specific flags, show everything
		if !showClaims && !showMessages && !showState {
			showClaims = true
			showMessages = true
			showState = true
		}

		// Initialize coordination dir
		if _, err := coordination.Init(repoURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing coordination: %v\n", err)
			os.Exit(1)
		}

		if showClaims {
			fmt.Println("File Claims:")
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			claims, err := coordination.ListClaims(repoURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			} else if len(claims) == 0 {
				fmt.Println("  (no active claims)")
			} else {
				for file, claim := range claims {
					fmt.Printf("  %-40s  %s (since %s)\n", file, claim.Agent, claim.ClaimedAt.Format(time.RFC3339))
				}
			}
			fmt.Println()
		}

		if showMessages {
			fmt.Println("Recent Messages:")
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			msgs, err := coordination.ReadMessages(repoURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			} else if len(msgs) == 0 {
				fmt.Println("  (no messages)")
			} else {
				// Show last 20 messages
				start := 0
				if len(msgs) > 20 {
					start = len(msgs) - 20
				}
				for _, msg := range msgs[start:] {
					dataStr := ""
					if len(msg.Data) > 0 {
						pairs := make([]string, 0, len(msg.Data))
						for k, v := range msg.Data {
							pairs = append(pairs, k+"="+v)
						}
						dataStr = " " + strings.Join(pairs, " ")
					}
					fmt.Printf("  [%s] %-15s %-15s%s\n",
						msg.Timestamp.Format("15:04:05"), msg.Type, msg.Agent, dataStr)
				}
			}
			fmt.Println()
		}

		if showState {
			fmt.Println("Agent State:")
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			state, err := coordination.GetState(repoURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			} else if len(state.Agents) == 0 {
				fmt.Println("  (no agents registered)")
			} else {
				for _, agent := range state.Agents {
					fmt.Printf("  %-15s status=%-10s branch=%-20s updated=%s\n",
						agent.Name, agent.Status, agent.Branch, agent.LastUpdate.Format(time.RFC3339))
				}
			}
		}

	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("agentctl - Claude Code Agent Container Orchestrator")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  spawn <name> <repo> [branch]    Create new agent container")
	fmt.Println("  run <name> <task> [attempts]    Run until task complete (Ralph Wiggum mode)")
	fmt.Println("  check <name>                    Check if agent's task is complete")
	fmt.Println("  list                            List all agents with status")
	fmt.Println("  status <name>                   Show agent details")
	fmt.Println("  logs [-f] <name>                Show Claude logs (-f to follow in real-time)")
	fmt.Println("  spy <name> [flags]              Stream Claude's real-time session activity")
	fmt.Println("  shell <name>                    Open shell in agent container")
	fmt.Println("  diagnose <name>                 Debug stuck agents (processes, logs, auth)")
	fmt.Println("  kill <name>                     Stop and remove agent")
	fmt.Println()
	fmt.Println("Coordination:")
	fmt.Println("  claim <agent> <repo-url> <file>             Claim a file for editing")
	fmt.Println("  release <agent> <repo-url> <file>           Release a file claim")
	fmt.Println("  notify <agent> <repo-url> <type> [k=v...]   Publish a coordination message")
	fmt.Println("  bus <repo-url> [--claims|--messages|--state] Show coordination bus state")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  agentctl spawn fix-bug https://github.com/user/repo feature-branch")
	fmt.Println("  agentctl run fix-bug 'Fix the failing tests in src/auth.go'")
	fmt.Println("  agentctl spy fix-bug")
	fmt.Println("  agentctl check fix-bug")
	fmt.Println("  agentctl kill fix-bug")
	fmt.Println()
	fmt.Println("Coordination Example:")
	fmt.Println("  agentctl claim agent-1 https://github.com/user/repo src/main.go")
	fmt.Println("  agentctl notify agent-1 https://github.com/user/repo committed sha=abc123")
	fmt.Println("  agentctl bus https://github.com/user/repo")
}
