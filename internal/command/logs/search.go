package logs

import (
	"context"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func NewSearch() (cmd *cobra.Command) {

	const (
		short = "Search and analyze application logs"
		long  = short + "\n"
	)

	cmd = command.New("search", short, long, runSearch, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func runSearch(ctx context.Context) (err error) {
	client := fly.ClientFromContext(ctx).GenqClient
	io := iostreams.FromContext(ctx)
	resp, err := gql.GetApp(ctx, client, appconfig.NameFromContext(ctx))

	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://fly-metrics.net/d/fly-logs/fly-logs?orgId=%s&var-app=%s", resp.App.Organization.Id, resp.App.Name)
	fmt.Fprintln(io.Out, "Opening", url)
	return open.Run(url)

}
