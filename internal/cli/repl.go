package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// REPL provides an interactive read-eval-print loop for SQL queries
type REPL struct {
	commands *Commands
	scanner  *bufio.Scanner
	prompt   string
	out      io.Writer
}

// NewREPL creates a new interactive REPL session reading from stdin
func NewREPL(commands *Commands, catalog, schema string) *REPL {
	return NewREPLWithReader(commands, catalog, schema, os.Stdin)
}

// NewREPLWithReader creates a new REPL reading from the given reader
func NewREPLWithReader(commands *Commands, catalog, schema string, in io.Reader) *REPL {
	prompt := "trino>"
	if catalog != "" {
		if schema != "" {
			prompt = fmt.Sprintf("%s.%s>", catalog, schema)
		} else {
			prompt = fmt.Sprintf("%s>", catalog)
		}
	}

	return &REPL{
		commands: commands,
		scanner:  bufio.NewScanner(in),
		prompt:   prompt,
		out:      commands.out,
	}
}

// Run starts the interactive REPL loop
func (r *REPL) Run(ctx context.Context) error {
	_, _ = fmt.Fprintln(r.out, "mcp-trino CLI - Interactive Mode")
	_, _ = fmt.Fprintln(r.out, "Type '\\help' for help, '\\quit' or Ctrl-D to exit")
	_, _ = fmt.Fprintln(r.out)

	history := []string{}

	for {
		_, _ = fmt.Fprint(r.out, r.prompt)

		if !r.scanner.Scan() {
			_, _ = fmt.Fprintln(r.out)
			return nil
		}

		line := strings.TrimSpace(r.scanner.Text())

		if line == "" {
			continue
		}

		// Handle meta-commands
		if strings.HasPrefix(line, "\\") {
			if err := r.handleMetaCommand(ctx, line, &history); err != nil {
				if err == ErrExitREPL {
					return nil
				}
				_, _ = fmt.Fprintf(r.commands.errOut, "Error: %v\n", err)
			}
			continue
		}

		// Handle multi-line queries
		query := line
		for !strings.HasSuffix(query, ";") && r.hasMoreInput(query) {
			_, _ = fmt.Fprint(r.out, "... ")
			if !r.scanner.Scan() {
				if err := r.scanner.Err(); err != nil {
					return fmt.Errorf("multiline input error: %w", err)
				}
				break
			}
			nextLine := r.scanner.Text()
			query += "\n" + nextLine
		}

		query = strings.TrimSuffix(query, ";")
		query = strings.TrimSpace(query)

		history = append(history, query)

		startTime := time.Now()
		if err := r.commands.Query(ctx, query); err != nil {
			_, _ = fmt.Fprintf(r.commands.errOut, "Error: %v\n", err)
		} else {
			duration := time.Since(startTime)
			if duration > time.Second {
				_, _ = fmt.Fprintf(r.out, "(%v)\n", duration.Round(time.Millisecond))
			}
		}
		_, _ = fmt.Fprintln(r.out)
	}
}

// ErrExitREPL is returned when the user wants to exit the REPL
var ErrExitREPL = fmt.Errorf("exit REPL")

// handleMetaCommand handles REPL meta-commands (prefixed with \)
func (r *REPL) handleMetaCommand(ctx context.Context, cmd string, history *[]string) error {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "\\help", "\\?":
		r.printHelp()
	case "\\quit", "\\exit", "\\q":
		return ErrExitREPL
	case "\\history":
		r.printHistory(history)
	case "\\catalogs":
		return r.commands.Catalogs(ctx)
	case "\\schemas":
		catalog := ""
		if len(parts) > 1 {
			catalog = parts[1]
		}
		return r.commands.Schemas(ctx, catalog)
	case "\\tables":
		catalog, schema := "", ""
		if len(parts) > 1 {
			catalog = parts[1]
		}
		if len(parts) > 2 {
			schema = parts[2]
		}
		return r.commands.Tables(ctx, catalog, schema)
	case "\\describe", "\\d":
		if len(parts) < 2 {
			return fmt.Errorf("usage: \\describe <catalog.schema.table>")
		}
		return r.commands.Describe(ctx, parts[1])
	case "\\format":
		if len(parts) < 2 {
			_, _ = fmt.Fprintf(r.out, "Current format: %s\n", r.commands.format)
			return nil
		}
		format := strings.ToLower(parts[1])
		if format != "table" && format != "json" && format != "csv" {
			return fmt.Errorf("invalid format. Supported: table, json, csv")
		}
		r.commands.format = format
		_, _ = fmt.Fprintf(r.out, "Output format set to: %s\n", format)
	case "\\timing":
		_, _ = fmt.Fprintln(r.out, "Timing display is always enabled for queries > 1s")
	default:
		return fmt.Errorf("unknown command: %s (type \\help for available commands)", command)
	}

	return nil
}

// hasMoreInput checks if there's more input to read (for multi-line queries)
func (r *REPL) hasMoreInput(query string) bool {
	query = strings.TrimSpace(query)
	if strings.HasSuffix(query, ";") {
		return false
	}

	queryLower := strings.ToLower(query)

	incompleteEnds := []string{
		"select", "from", "where", "join", "left join",
		"right join", "inner join", "outer join", "on",
		"group by", "order by", "having", "limit",
		"and", "or", "not",
		"insert into", "values", "update", "set",
		"create", "alter", "drop",
	}

	for _, end := range incompleteEnds {
		if strings.HasSuffix(queryLower, end) {
			return true
		}
	}

	return false
}

// printHelp displays help information for REPL commands
func (r *REPL) printHelp() {
	_, _ = fmt.Fprintln(r.out, "Meta-commands:")
	_, _ = fmt.Fprintln(r.out, "  \\help              Display this help")
	_, _ = fmt.Fprintln(r.out, "  \\quit, \\exit, \\q  Exit the REPL")
	_, _ = fmt.Fprintln(r.out, "  \\history           Display command history")
	_, _ = fmt.Fprintln(r.out, "  \\catalogs          List all catalogs")
	_, _ = fmt.Fprintln(r.out, "  \\schemas [cat]     List schemas (optional catalog)")
	_, _ = fmt.Fprintln(r.out, "  \\tables [cat sch]  List tables (optional catalog.schema)")
	_, _ = fmt.Fprintln(r.out, "  \\describe <table>  Describe table (format: catalog.schema.table)")
	_, _ = fmt.Fprintln(r.out, "  \\format <fmt>      Set output format (table, json, csv)")
	_, _ = fmt.Fprintln(r.out)
	_, _ = fmt.Fprintln(r.out, "SQL Queries:")
	_, _ = fmt.Fprintln(r.out, "  SELECT ...         Execute a SQL query")
	_, _ = fmt.Fprintln(r.out, "  EXPLAIN ...        Analyze query execution plan")
	_, _ = fmt.Fprintln(r.out)
	_, _ = fmt.Fprintln(r.out, "Tips:")
	_, _ = fmt.Fprintln(r.out, "  - Use ; to terminate multi-line queries")
	_, _ = fmt.Fprintln(r.out, "  - Ctrl-D exits the REPL")
}

// printHistory displays command history
func (r *REPL) printHistory(history *[]string) {
	if len(*history) == 0 {
		_, _ = fmt.Fprintln(r.out, "No history")
		return
	}

	for i, cmd := range *history {
		_, _ = fmt.Fprintf(r.out, "%4d  %s\n", i+1, cmd)
	}
}
