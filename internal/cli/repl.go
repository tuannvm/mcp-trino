package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// REPL provides an interactive read-eval-print loop for SQL queries
type REPL struct {
	commands *Commands
	scanner  *bufio.Scanner
	prompt   string
}

// NewREPL creates a new interactive REPL session
func NewREPL(commands *Commands, catalog, schema string) *REPL {
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
		scanner:  bufio.NewScanner(os.Stdin),
		prompt:   prompt,
	}
}

// Run starts the interactive REPL loop
func (r *REPL) Run(ctx context.Context) error {
	fmt.Println("mcp-trino CLI - Interactive Mode")
	fmt.Println("Type '\\help' for help, '\\quit' or Ctrl-D to exit")
	fmt.Println()

	history := []string{}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Display prompt
		fmt.Print(r.prompt)

		// Read input
		lineRaw, ok, err := r.scanLine(ctx)
		if err != nil {
			return fmt.Errorf("input error: %w", err)
		}
		if !ok {
			// Check for real I/O errors (not just EOF)
			if err := r.scanner.Err(); err != nil {
				return fmt.Errorf("input error: %w", err)
			}
			// EOF (Ctrl-D)
			fmt.Println()
			return nil
		}

		line := strings.TrimSpace(lineRaw)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Handle special commands
		if strings.HasPrefix(line, "\\") {
			if err := r.handleMetaCommand(ctx, line, &history); err != nil {
				if errors.Is(err, ErrExitREPL) {
					return nil
				}
				fmt.Printf("Error: %v\n", err)
			}
			continue
		}

		// Handle multi-line queries
		query := line
		for !strings.HasSuffix(query, ";") && r.hasMoreInput(query) {
			fmt.Print("... ")
			nextLine, ok, err := r.scanLine(ctx)
			if err != nil {
				return fmt.Errorf("multiline input error: %w", err)
			}
			if !ok {
				// Check for real I/O errors (not just EOF)
				if err := r.scanner.Err(); err != nil {
					return fmt.Errorf("multiline input error: %w", err)
				}
				// EOF (Ctrl-D) during multiline input - execute what we have
				break
			}
			query += "\n" + nextLine
		}

		// Remove trailing semicolon
		query = strings.TrimSuffix(query, ";")
		query = strings.TrimSpace(query)

		// Add to history
		history = append(history, query)

		// Execute query
		startTime := time.Now()
		if err := r.commands.Query(ctx, query); err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			duration := time.Since(startTime)
			if duration > time.Second {
				fmt.Printf("(%v)\n", duration.Round(time.Millisecond))
			}
		}
		fmt.Println()
	}
}

func (r *REPL) scanLine(ctx context.Context) (string, bool, error) {
	type scanResult struct {
		line string
		ok   bool
		err  error
	}

	resultCh := make(chan scanResult, 1)
	go func() {
		ok := r.scanner.Scan()
		if !ok {
			resultCh <- scanResult{ok: false, err: r.scanner.Err()}
			return
		}
		resultCh <- scanResult{line: r.scanner.Text(), ok: true}
	}()

	select {
	case <-ctx.Done():
		return "", false, ctx.Err()
	case result := <-resultCh:
		return result.line, result.ok, result.err
	}
}

// ErrExitREPL is returned when the user wants to exit the REPL
var ErrExitREPL = errors.New("exit REPL")

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
			fmt.Printf("Current format: %s\n", r.commands.format)
			return nil
		}
		format := strings.ToLower(parts[1])
		if format != "table" && format != "json" && format != "csv" {
			return fmt.Errorf("invalid format. Supported: table, json, csv")
		}
		r.commands.format = format
		fmt.Printf("Output format set to: %s\n", format)
	case "\\timing":
		// Toggle timing display (for future implementation)
		fmt.Println("Timing display is always enabled for queries > 1s")
	default:
		return fmt.Errorf("unknown command: %s (type \\help for available commands)", command)
	}

	return nil
}

// hasMoreInput checks if there's more input to read (for multi-line queries)
func (r *REPL) hasMoreInput(query string) bool {
	// If query ends with semicolon, it's complete
	query = strings.TrimSpace(query)
	if query == "" || strings.HasSuffix(query, ";") {
		return false
	}

	// Check for incomplete SQL patterns that require continuation
	queryLower := strings.ToLower(query)
	lastLine := queryLower
	if idx := strings.LastIndex(lastLine, "\n"); idx >= 0 {
		lastLine = strings.TrimSpace(lastLine[idx+1:])
	}

	// Common continuation markers
	if strings.HasSuffix(lastLine, ",") {
		return true
	}
	if strings.HasSuffix(lastLine, "(") {
		return true
	}

	operators := []string{
		"+", "-", "*", "/", "%", "=", "<", ">", "<=", ">=", "<>", "!=",
		"||", "and", "or", "like", "in", "is", "between",
	}
	for _, op := range operators {
		if strings.HasSuffix(lastLine, " "+op) || lastLine == op {
			return true
		}
	}

	// Check if query ends with incomplete keywords (without trailing space after trim)
	incompleteEnds := []string{
		"select", "from", "where", "join", "left join",
		"right join", "inner join", "outer join", "on",
		"group by", "order by", "having", "limit",
		"and", "or", "not",
		"insert into", "values", "update", "set",
		"create", "alter", "drop",
		"with", "with recursive", "as", "union", "intersect", "except",
	}

	for _, end := range incompleteEnds {
		// Check if query ends with this keyword (as a whole word)
		if strings.HasSuffix(queryLower, end) {
			return true
		}
	}

	// Unbalanced parentheses indicate unfinished expression or CTE body.
	parenBalance := 0
	for _, ch := range query {
		switch ch {
		case '(':
			parenBalance++
		case ')':
			if parenBalance > 0 {
				parenBalance--
			}
		}
	}
	return parenBalance > 0
}

// printHelp displays help information for REPL commands
func (r *REPL) printHelp() {
	fmt.Println("Meta-commands:")
	fmt.Println("  \\help              Display this help")
	fmt.Println("  \\quit, \\exit, \\q  Exit the REPL")
	fmt.Println("  \\history           Display command history")
	fmt.Println("  \\catalogs          List all catalogs")
	fmt.Println("  \\schemas [cat]     List schemas (optional catalog)")
	fmt.Println("  \\tables [cat sch]  List tables (optional catalog.schema)")
	fmt.Println("  \\describe <table>  Describe table (format: catalog.schema.table)")
	fmt.Println("  \\format <fmt>      Set output format (table, json, csv)")
	fmt.Println()
	fmt.Println("SQL Queries:")
	fmt.Println("  SELECT ...         Execute a SQL query")
	fmt.Println("  EXPLAIN ...        Analyze query execution plan")
	fmt.Println()
	fmt.Println("Tips:")
	fmt.Println("  - Use ; to terminate multi-line queries")
	fmt.Println("  - Ctrl-D exits the REPL")
}

// printHistory displays command history
func (r *REPL) printHistory(history *[]string) {
	if len(*history) == 0 {
		fmt.Println("No history")
		return
	}

	for i, cmd := range *history {
		fmt.Printf("%4d  %s\n", i+1, cmd)
	}
}
