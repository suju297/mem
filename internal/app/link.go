package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"mempack/internal/store"
)

type LinkResponse struct {
	FromID    string  `json:"from_id"`
	Rel       string  `json:"rel"`
	ToID      string  `json:"to_id"`
	Weight    float64 `json:"weight"`
	CreatedAt string  `json:"created_at"`
	Status    string  `json:"status"`
}

func runLink(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fromID := fs.String("from", "", "Source memory id")
	relRaw := fs.String("rel", "", "Relation type (for example: depends_on, evidence_for)")
	toID := fs.String("to", "", "Target memory id")
	workspace := fs.String("workspace", "", "Workspace name")
	repoOverride := fs.String("repo", "", "Override repo id")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"from":      {RequiresValue: true},
		"rel":       {RequiresValue: true},
		"to":        {RequiresValue: true},
		"workspace": {RequiresValue: true},
		"repo":      {RequiresValue: true},
	})
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	fromWasSet := flagWasSet(args, "from")
	relWasSet := flagWasSet(args, "rel")
	toWasSet := flagWasSet(args, "to")
	if !fromWasSet && len(positional) > 0 {
		*fromID = positional[0]
		positional = positional[1:]
	}
	if !relWasSet && len(positional) > 0 {
		*relRaw = positional[0]
		positional = positional[1:]
	}
	if !toWasSet && len(positional) > 0 {
		*toID = positional[0]
		positional = positional[1:]
	}
	if len(positional) > 0 {
		fmt.Fprintf(errOut, "unexpected args: %s\n", strings.Join(positional, " "))
		return 2
	}

	from := strings.TrimSpace(*fromID)
	if from == "" && isInteractiveTerminal(os.Stdin) {
		promptedFrom, promptErr := promptText(os.Stdin, errOut, "From memory id", false)
		if promptErr != nil {
			fmt.Fprintf(errOut, "from prompt error: %v\n", promptErr)
			return 1
		}
		from = strings.TrimSpace(promptedFrom)
	}
	to := strings.TrimSpace(*toID)
	if to == "" && isInteractiveTerminal(os.Stdin) {
		promptedTo, promptErr := promptText(os.Stdin, errOut, "To memory id", false)
		if promptErr != nil {
			fmt.Fprintf(errOut, "to prompt error: %v\n", promptErr)
			return 1
		}
		to = strings.TrimSpace(promptedTo)
	}
	relInput := strings.TrimSpace(*relRaw)
	if relInput == "" && isInteractiveTerminal(os.Stdin) {
		promptedRel, promptErr := promptText(os.Stdin, errOut, "Relation (for example: depends_on)", false)
		if promptErr != nil {
			fmt.Fprintf(errOut, "rel prompt error: %v\n", promptErr)
			return 1
		}
		relInput = strings.TrimSpace(promptedRel)
	}
	if from == "" {
		fmt.Fprintln(errOut, "missing from id (use --from or first positional argument)")
		return 2
	}
	if to == "" {
		fmt.Fprintln(errOut, "missing to id (use --to or third positional argument)")
		return 2
	}
	if from == to {
		fmt.Fprintln(errOut, "from and to must differ")
		return 2
	}
	if relInput == "" {
		fmt.Fprintln(errOut, "missing relation (use --rel or second positional argument)")
		return 2
	}
	rel, err := normalizeLinkRelation(relInput)
	if err != nil {
		fmt.Fprintf(errOut, "invalid --rel: %v\n", err)
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))

	repoInfo, err := resolveRepo(&cfg, strings.TrimSpace(*repoOverride))
	if err != nil {
		fmt.Fprintf(errOut, "repo detection error: %v\n", err)
		return 1
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		fmt.Fprintf(errOut, "store open error: %v\n", err)
		return 1
	}
	defer st.Close()

	if err := st.EnsureRepo(repoInfo); err != nil {
		fmt.Fprintf(errOut, "store repo error: %v\n", err)
		return 1
	}
	if _, err := ensureMemoryExistsForLink(st, repoInfo.ID, workspaceName, from, "from"); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 1
	}
	if _, err := ensureMemoryExistsForLink(st, repoInfo.ID, workspaceName, to, "to"); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 1
	}

	createdAt := time.Now().UTC()
	if err := st.AddLink(store.Link{
		FromID:    from,
		Rel:       rel,
		ToID:      to,
		Weight:    1,
		CreatedAt: createdAt,
	}); err != nil {
		fmt.Fprintf(errOut, "link error: %v\n", err)
		return 1
	}

	return writeJSON(out, errOut, LinkResponse{
		FromID:    from,
		Rel:       rel,
		ToID:      to,
		Weight:    1,
		CreatedAt: createdAt.Format(time.RFC3339Nano),
		Status:    "linked",
	})
}
