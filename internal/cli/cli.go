package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/ilyalavrenov/pantograph/internal/collect"
	"github.com/ilyalavrenov/pantograph/internal/config"
	"github.com/ilyalavrenov/pantograph/internal/render"
	"github.com/urfave/cli/v3"
)

const defaultPattern = "./..."

func Run(osArgs []string) error {
	cmd := &cli.Command{
		Name:                  "pantograph",
		Usage:                 "flow diagrams for a Go codebase, generated from //pantograph: annotations",
		DefaultCommand:        "generate",
		HideVersion:           true,
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			generateCommand(),
			validateCommand(),
			listCommand(),
			coverageCommand(),
		},
	}

	return cmd.Run(context.Background(), osArgs) //nolint:wrapcheck // error is already contextual
}

func generateCommand() *cli.Command {
	return &cli.Command{
		Name:  "generate",
		Usage: "render the flow diagrams to disk",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "out", Value: "docs/flows", Usage: "output directory for .d2 files"},
			&cli.BoolFlag{Name: "check", Usage: "verify on-disk files are up to date; exit non-zero if they drifted (writes nothing)"},
			strictFlag(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return generate(cmd.String("out"), cmd.Bool("check"), cmd.Bool("strict"))
		},
	}
}

func validateCommand() *cli.Command {
	return &cli.Command{
		Name:  "validate",
		Usage: "fast pre-merge check: parse and fuse the annotations without rendering or the freshness assertion; writes nothing",
		Flags: []cli.Flag{strictFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return validate(cmd.Bool("strict"))
		},
	}
}

func listCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "print every flow-id, its node count, and its nodes (sorted); writes nothing",
		Action: func(_ context.Context, _ *cli.Command) error {
			flows, _, _, err := collect.Collect([]string{defaultPattern})
			if err != nil {
				return err
			}

			printLines(collect.ListReport(flows))

			return nil
		},
	}
}

func coverageCommand() *cli.Command {
	return &cli.Command{
		Name:      "coverage",
		Usage:     "list EXPORTED funcs/methods with no //pantograph: annotation; writes nothing",
		ArgsUsage: "[package-path]",
		Description: "With no argument, reports all scanned packages. Pass a module-relative package " +
			"path (e.g. pkg/api) to filter to one package.",
		Action: func(_ context.Context, cmd *cli.Command) error {
			return coverage(newLogger(), cmd.Args().First())
		},
	}
}

func strictFlag() *cli.BoolFlag {
	return &cli.BoolFlag{
		Name:  "strict",
		Usage: "promote orphan-node warnings (a node in a ≥2-node flow with no edge touching it) to a build failure",
	}
}

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func generate(out string, check, strict bool) error {
	log := newLogger()

	collectStart := time.Now()

	domains, cfg, err := loadDomains(log, strict)
	if err != nil {
		return err
	}

	collectDur := time.Since(collectStart)

	relOut, err := collect.OutputRelPath(out)
	if err != nil {
		return err
	}

	renderStart := time.Now()

	files, err := render.Render(domains, cfg.Kinds, relOut)
	if err != nil {
		return err
	}

	log.Info("timing", slog.Duration("collect", collectDur), slog.Duration("render", time.Since(renderStart)))

	if check {
		return render.CheckUpToDate(files, out)
	}

	if err := os.MkdirAll(out, 0o755); err != nil { //nolint:mnd // standard directory permission
		return fmt.Errorf("mkdir %s: %w", out, err)
	}

	return render.WriteFiles(log, files, out)
}

func validate(strict bool) error {
	_, _, err := loadDomains(newLogger(), strict)

	return err
}

func coverage(log *slog.Logger, pkg string) error {
	_, inv, _, err := collect.Collect([]string{defaultPattern})
	if err != nil {
		return err
	}

	lines, matched := collect.CoverageReport(inv, pkg)
	if !matched {
		log.Warn(fmt.Sprintf("coverage %q matched no scanned package", pkg))
	}

	printLines(lines)

	return nil
}

func loadDomains(log *slog.Logger, strict bool) (map[string]*collect.Flow, *config.Config, error) {
	flows, _, cfg, err := collect.Collect([]string{defaultPattern})
	if err != nil {
		return nil, nil, err
	}

	if len(flows) == 0 {
		return nil, nil, fmt.Errorf("no //pantograph: annotations found under %s", defaultPattern)
	}

	if err := lintOrphans(log, flows, strict); err != nil {
		return nil, nil, err
	}

	domains, err := collect.FuseFlows(flows, cfg.DomainDecls())
	if err != nil {
		return nil, nil, err
	}

	return domains, cfg, nil
}

func lintOrphans(log *slog.Logger, flows map[string]*collect.Flow, strict bool) error {
	orphans := collect.FindOrphanNodes(flows)
	if len(orphans) == 0 {
		return nil
	}

	if strict {
		return fmt.Errorf("%d orphan node(s) (-strict):\n%s", len(orphans), strings.Join(orphans, "\n"))
	}

	for _, w := range orphans {
		log.Warn(w)
	}

	return nil
}

func printLines(lines []string) {
	for _, l := range lines {
		fmt.Println(l)
	}
}
