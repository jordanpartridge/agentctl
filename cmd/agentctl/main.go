package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jordanpartridge/agentctl/pkg/container"
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

	case "watch":
		if len(os.Args) < 3 {
			fmt.Println("Usage: agentctl watch <name>")
			os.Exit(1)
		}
		container.Watch(os.Args[2])

	case "shell":
		if len(os.Args) < 3 {
			fmt.Println("Usage: agentctl shell <name>")
			os.Exit(1)
		}
		container.Shell(os.Args[2])

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
	fmt.Println("  watch <name>                    Stream Claude's activity in real-time")
	fmt.Println("  shell <name>                    Open shell in agent container")
	fmt.Println("  kill <name>                     Stop and remove agent")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  agentctl spawn fix-bug https://github.com/user/repo feature-branch")
	fmt.Println("  agentctl run fix-bug 'Fix the failing tests in src/auth.go'")
	fmt.Println("  agentctl watch fix-bug")
	fmt.Println("  agentctl check fix-bug")
	fmt.Println("  agentctl kill fix-bug")
}
