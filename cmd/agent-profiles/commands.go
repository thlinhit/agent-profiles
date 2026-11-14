package main

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

type command struct {
	name, group, synopsis, summary, usage string
	flags, examples                       []string
	arguments                             []string
	note                                  string
	hidden                                bool
	run                                   func(profile.Store, []string, io.Writer, io.Writer) error
}

var commandRegistry = buildCommandRegistry()

func buildCommandRegistry() []command {
	return []command{
		{name: "create", group: "Profiles", synopsis: "create <agent> <name>", summary: "Create a Profile (--init, --from, --base-url)", usage: "create <agent> <name> [--init | --from <profile>] [--base-url <url>] [--auth-token-stdin]", arguments: []string{agentArgument(), "<name>\tProfile name"}, flags: []string{"--init\tWrite starter settings", "--from <profile>\tCopy settings from another Profile of the same Agent", "--base-url <url>\tSet Claude API base URL", "--auth-token-stdin\tRead Claude token from stdin"}, examples: []string{"ap create claude work --init"}, run: func(s profile.Store, a []string, o, d io.Writer) error { return runCreate(s, a, o, d) }},
		{name: "list", group: "Profiles", synopsis: "list [agent]", summary: "List Profiles", usage: "list [agent] [--json]", arguments: []string{"[agent]\tFilter by Agent"}, flags: []string{"--json\tPrint JSON"}, examples: []string{"ap list claude", "ap list --json"}, run: func(s profile.Store, a []string, o, d io.Writer) error { return runList(s, a, o) }},
		{name: "update", group: "Profiles", synopsis: "update <agent> <name|--all>", summary: "Refresh ap-owned settings in Profiles", usage: "update <agent> <name> | update <agent> --all", arguments: []string{agentArgument(), "<name>\tProfile name"}, flags: []string{"--all\tRefresh every Profile for the Agent"}, examples: []string{"ap update claude work", "ap update claude --all"}, note: "Claude only. Refreshes the status line and Linked-skill access; permissions and env are kept.", run: func(s profile.Store, a []string, o, d io.Writer) error { return runUpdate(s, a, o) }},
		{name: "delete", group: "Profiles", synopsis: "delete <agent> <name>", summary: "Delete a Profile", usage: "delete <agent> <name> [--yes]", arguments: []string{agentArgument(), "<name>\tProfile name"}, flags: []string{"--yes\tDelete without prompting"}, examples: []string{"ap delete claude work --yes"}, note: "Prompts unless --yes is supplied.", run: func(s profile.Store, a []string, o, d io.Writer) error { return runDelete(s, a, d) }},
		{name: "use", group: "Activation", synopsis: "use <agent> <name>", summary: "Activate a Profile in this shell", usage: "use <agent> <name>", arguments: []string{agentArgument(), "<name>\tProfile name"}, examples: []string{"ap use claude work"}, run: func(s profile.Store, a []string, o, d io.Writer) error { return runUse(s, a, o) }},
		{name: "off", group: "Activation", synopsis: "off <agent>", summary: "Return this shell to the Default config dir", usage: "off <agent>", arguments: []string{agentArgument()}, examples: []string{"ap off claude"}, run: func(s profile.Store, a []string, o, d io.Writer) error { return runUse(s, a, o) }},
		{name: "current", group: "Activation", synopsis: "current", summary: "Show Active profile and Source per Agent", usage: "current [--json]", flags: []string{"--json\tPrint JSON"}, examples: []string{"ap current", "ap current --json"}, run: func(s profile.Store, a []string, o, d io.Writer) error { return runCurrent(a, o, d) }},
		{name: "exec", group: "Activation", synopsis: "exec <agent> <name> [-- <cmd>]", summary: "Run a command with a Profile", usage: "exec <agent> <name> [-- <command> [args...]]", arguments: []string{agentArgument(), "<name>\tProfile name", "<cmd>\tCommand and arguments after --"}, examples: []string{"ap exec claude work -- claude --help"}, run: func(s profile.Store, a []string, o, d io.Writer) error { return runExec(s, a) }},
		{name: "local", group: "Per-directory", synopsis: "local <agent> <name>", summary: "Write a Selection to .agent-profile", usage: "local <agent> <name>", arguments: []string{agentArgument(), "<name>\tProfile name"}, examples: []string{"ap local claude work"}, note: "Writes .agent-profile here. Run 'ap trust' next; it has no effect until trusted.", run: func(s profile.Store, a []string, o, d io.Writer) error { return runLocal(s, a) }},
		{name: "trust", group: "Per-directory", synopsis: "trust", summary: "Trust the current Dotfile", usage: "trust", examples: []string{"ap trust"}, note: "Trust is bound to the file's exact bytes; re-run after any edit. The shell hook applies it on the next prompt in bash or the next directory change in zsh; cd . applies it now in either shell.", run: func(s profile.Store, a []string, o, d io.Writer) error { return runTrust(a) }},
		{name: "untrust", group: "Per-directory", synopsis: "untrust", summary: "Revoke trust for the current Dotfile", usage: "untrust", examples: []string{"ap untrust"}, note: "Revokes Trust; it does not deactivate profiles already exported in this shell. Use 'ap off <agent>'.", run: func(s profile.Store, a []string, o, d io.Writer) error { return runTrust(a) }},
		{name: "skills", group: "Skills", synopsis: "skills ls|add|rm|select", summary: "Manage Profile Skills; manage pools with 'skills pool'", usage: "skills <ls|add|rm|select|pool>", examples: []string{"ap skills ls claude work", "ap skills pool ls"}, run: func(s profile.Store, a []string, o, d io.Writer) error { return runSkills(s, a, o) }},
		{name: "init", group: "Setup", synopsis: "init <zsh|bash>", summary: "Print shell setup", usage: "init zsh|bash", arguments: []string{"<zsh|bash>\tShell to configure"}, examples: []string{"eval \"$(agent-profiles init zsh)\""}, run: func(s profile.Store, a []string, o, d io.Writer) error { return runInit(a, o) }},
		{name: "auto", group: "Hidden", synopsis: "auto", summary: "Print exports for trusted Dotfile selections in this directory", usage: "auto", note: "Called by the shell hook, not by users.", hidden: true, run: func(s profile.Store, a []string, o, d io.Writer) error { return runAuto(a, o, d) }},
	}
}

func commandUsage(name string) string {
	usage := map[string]string{"create": "create <agent> <name> [--init | --from <profile>] [--base-url <url>] [--auth-token-stdin]", "list": "list [agent] [--json]", "update": "update <agent> <name> | update <agent> --all", "delete": "delete <agent> <name> [--yes]", "use": "use <agent> <name>", "off": "off <agent>", "current": "current [--json]", "exec": "exec <agent> <name> [-- <command> [args...]]", "local": "local <agent> <name>", "trust": "trust", "untrust": "untrust", "init": "init zsh|bash", "auto": "auto"}[name]
	if usage == "" {
		usage = name
	}
	return "usage: ap " + usage + "\nRun 'ap help " + name + "' for details."
}

var (
	skillSubcommands = []string{"ls", "add", "rm", "select", "pool"}
	poolSubcommands  = []string{"add", "rm", "ls"}
)

func skillsUsage() string {
	return "usage: ap skills <ls|add|rm|select|pool>\nRun 'ap help skills' for details."
}

func skillUsage(sub string) string {
	usage := skillHelpUsage(sub)
	return "usage: ap " + usage + "\nRun 'ap help skills " + sub + "' for details."
}

func skillHelpUsage(sub string) string {
	m := map[string]string{"ls": "skills ls <agent> <profile> [--json]", "ls --pool": "skills ls --pool [--json]", "add": "skills add <agent> <profile> <path-or-name> [--name N]", "rm": "skills rm <agent> <profile> <skill>", "select": "skills select [<agent> [<profile>]]", "pool": "skills pool <add|rm|ls>", "pool add": "skills pool add <path>", "pool rm": "skills pool rm <path>", "pool ls": "skills pool ls [--json]"}
	if usage := m[sub]; usage != "" {
		return usage
	}
	return "skills"
}

func printTopHelp(out io.Writer) error {
	if _, err := fmt.Fprintln(out, "ap: switch AI agent config profiles per shell and per directory\n\nUsage:  ap <command> [args]"); err != nil {
		return err
	}
	groups := []string{"Profiles", "Activation", "Per-directory", "Skills", "Setup"}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	for _, group := range groups {
		if _, err := fmt.Fprintln(w, "\n"+group); err != nil {
			return err
		}
		for _, c := range commandRegistry {
			if c.group == group && !c.hidden {
				if c.name == "skills" {
					for _, row := range []struct{ synopsis, summary string }{{"skills ls|add|rm|select", "Manage Profile Skills"}, {"skills pool add|rm|ls", "Manage Skill pools"}} {
						if _, err := fmt.Fprintf(w, "  %s\t%s\n", row.synopsis, row.summary); err != nil {
							return err
						}
					}
					continue
				}
				if _, err := fmt.Fprintf(w, "  %s\t%s\n", c.synopsis, c.summary); err != nil {
					return err
				}
			}
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "\nAgents: %s\nRun 'ap help <command>' for flags and examples.\n", strings.Join(agentNames(), ", "))
	return err
}

func agentNames() []string {
	names := make([]string, 0)
	for _, a := range agent.All() {
		names = append(names, a.Name())
	}
	sort.Strings(names)
	return names
}

func agentArgument() string {
	return "<agent>\tOne of: " + strings.Join(agentNames(), ", ")
}

func printCommandHelp(out io.Writer, args []string) error {
	if len(args) == 0 {
		return printTopHelp(out)
	}
	if args[0] == "help" {
		args = args[1:]
	}
	if len(args) > 1 && args[0] == "skills" {
		if !slices.Contains(skillSubcommands, args[1]) {
			return unknownSkillSubcommand("", args[1])
		}
		if args[1] == "pool" && len(args) > 2 && !slices.Contains(poolSubcommands, args[2]) {
			return unknownSkillSubcommand("pool", args[2])
		}
	}
	c := findCommand(args[0])
	if c == nil {
		return unknownCommand(args[0])
	}
	if c.name == "skills" && len(args) == 1 {
		c2 := *c
		c2.examples = []string{"ap skills ls claude work", "ap skills pool ls"}
		c = &c2
	}
	if c.name == "skills" && len(args) > 1 {
		sub := args[1]
		if sub == "pool" && len(args) > 2 {
			sub = "pool " + args[2]
		}
		if sub == "ls" && len(args) > 2 && args[2] == "--pool" {
			sub = "ls --pool"
		}
		usage := map[string]string{"ls": "skills ls <agent> <profile> [--json]", "ls --pool": "skills ls --pool [--json]", "add": "skills add <agent> <profile> <path-or-name> [--name N]", "rm": "skills rm <agent> <profile> <skill>", "select": "skills select [<agent> [<profile>]]", "pool": "skills pool <add|rm|ls>", "pool add": "skills pool add <path>", "pool rm": "skills pool rm <path>", "pool ls": "skills pool ls [--json]"}
		if u, ok := usage[sub]; ok {
			c2 := *c
			c2.usage = u
			c2.summary = "Manage Skills"
			if sub == "ls --pool" {
				c2.usage = u
			}
			c2.flags, c2.arguments, c2.examples, c2.note = nil, nil, nil, ""
			switch sub {
			case "ls":
				c2.flags = []string{"--json\tPrint JSON"}
				c2.arguments = []string{agentArgument(), "<profile>\tProfile name"}
				c2.examples = []string{"ap skills ls claude work", "ap skills ls claude work --json"}
			case "ls --pool":
				c2.flags = []string{"--json\tPrint JSON"}
				c2.examples = []string{"ap skills ls --pool --json"}
			case "add":
				c2.arguments = []string{agentArgument(), "<profile>\tProfile name", "<path-or-name>\tSkill source path, or Skill name found in configured pools"}
				c2.flags = []string{"--name N\tUse an Alias for the Skill link"}
				c2.examples = []string{"ap skills add claude work ./my-skill"}
			case "rm":
				c2.arguments = []string{agentArgument(), "<profile>\tProfile name", "<skill>\tSkill link name"}
				c2.examples = []string{"ap skills rm claude work my-skill"}
			case "select":
				c2.arguments = []string{"[<agent>]\tAgent to select for", "[<profile>]\tProfile to update"}
				c2.note = "Prompts; use ap skills add/rm non-interactively."
				c2.examples = []string{"ap skills select claude work"}
			case "pool add", "pool rm":
				c2.arguments = []string{"<path>\tSkill pool directory"}
				c2.examples = []string{"ap skills " + sub + " ~/skills"}
			case "pool ls":
				c2.flags = []string{"--json\tPrint JSON"}
				c2.examples = []string{"ap skills pool ls --json"}
			}
			c = &c2
		}
	}
	if c.name == "skills" && len(args) == 1 {
		return printSkillsHelp(out)
	}
	if _, err := fmt.Fprintf(out, "Usage: ap %s\n", c.usage); err != nil {
		return err
	}
	if c.summary != "" {
		if _, err := fmt.Fprintln(out, c.summary); err != nil {
			return err
		}
	}
	if len(c.arguments) > 0 {
		if _, err := fmt.Fprintln(out, "\nArguments:"); err != nil {
			return err
		}
		if err := writeAligned(out, c.arguments); err != nil {
			return err
		}
	}
	if len(c.flags) > 0 {
		if _, err := fmt.Fprintln(out, "\nFlags:"); err != nil {
			return err
		}
		if err := writeAligned(out, c.flags); err != nil {
			return err
		}
	}
	if len(c.examples) > 0 {
		if _, err := fmt.Fprintln(out, "\nExamples:"); err != nil {
			return err
		}
		for _, e := range c.examples {
			if _, err := fmt.Fprintf(out, "  %s\n", e); err != nil {
				return err
			}
		}
	}
	if c.note != "" {
		_, err := fmt.Fprintf(out, "\nNotes: %s\n", c.note)
		return err
	}
	return nil
}

func printSkillsHelp(out io.Writer) error {
	if _, err := fmt.Fprintln(out, "Usage: ap skills <subcommand>\n\nSubcommands:"); err != nil {
		return err
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	for _, row := range []struct{ synopsis, summary string }{{"ls <agent> <profile>", "List Skill links"}, {"ls --pool", "List Skills in configured pools"}, {"add <agent> <profile> <path-or-name>", "Add a Skill link"}, {"rm <agent> <profile> <skill>", "Remove a Skill link"}, {"select [<agent> [<profile>]]", "Interactively choose pool Skills"}, {"pool add|rm <path>", "Add or remove a Skill pool"}, {"pool ls", "List Skill pools"}} {
		if _, err := fmt.Fprintf(w, "  %s\t%s\n", row.synopsis, row.summary); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, "\nExamples:\n  ap skills ls claude work\n  ap skills pool ls")
	return err
}

func writeAligned(out io.Writer, rows []string) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "  %s\n", row); err != nil {
			return err
		}
	}
	return w.Flush()
}
func findCommand(name string) *command {
	for i := range commandRegistry {
		if commandRegistry[i].name == name {
			return &commandRegistry[i]
		}
	}
	return nil
}
func unknownCommand(name string) error {
	return unknownSuggestion("command", name, "")
}

func unknownSuggestion(kind, name, parent string) error {
	best := ""
	dist := 3
	for _, c := range commandRegistry {
		if c.hidden || parent != "" && c.name != parent {
			continue
		}
		d := distance(name, c.name)
		if d < dist {
			best = c.name
			dist = d
		}
	}
	if best != "" {
		return fmt.Errorf("unknown %s %q. Did you mean %q?\nRun 'ap help' for a list of commands.", kind, name, best)
	}
	return fmt.Errorf("unknown %s %q.\nRun 'ap help' for a list of commands.", kind, name)
}

// unknownSkillSubcommand reports an unrecognized skills subcommand; parent is "pool" for pool subcommands.
func unknownSkillSubcommand(parent, name string) error {
	candidates, help := skillSubcommands, "ap help skills"
	if parent == "pool" {
		candidates, help = poolSubcommands, "ap help skills pool"
	}
	msg := fmt.Sprintf("unknown skills subcommand %q.", name)
	if best := nearestName(name, candidates); best != "" {
		msg = fmt.Sprintf("unknown skills subcommand %q. Did you mean %q?", name, best)
	}
	return fmt.Errorf("%s\nRun '%s' for a list of subcommands.", msg, help)
}

// nearestName returns the candidate within edit distance 2 of name, closest first, or "".
func nearestName(name string, candidates []string) string {
	best, dist := "", 3
	for _, c := range candidates {
		if d := distance(name, c); d < dist {
			best, dist = c, d
		}
	}
	return best
}
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			cur[j] = min(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
