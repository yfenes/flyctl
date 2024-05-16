package deploy_test

import (
	"bytes"
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/task"
	"github.com/superfly/flyctl/iostreams"
)

//go:embed testdata
var testdata embed.FS

func TestCommand_Execute(t *testing.T) {
	dir := t.TempDir()
	fsys, _ := fs.Sub(testdata, "testdata/basic")
	if err := copyFS(fsys, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil { // TODO: Revert working directory
		t.Fatal(err)
	}

	var buf bytes.Buffer
	cmd := deploy.New()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--image", "registry.fly.io/my-image:deployment-00000000000000000000000000"})

	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, &iostreams.IOStreams{Out: &buf, ErrOut: &buf})
	ctx = task.NewWithContext(ctx)
	ctx = logger.NewContext(ctx, logger.New(&buf, logger.Info, true))

	var client mock.Client
	ctx = flyutil.NewContextWithClient(ctx, &client)

	var flapsClient mock.FlapsClient
	ctx = flapsutil.NewContextWithClient(ctx, &flapsClient)

	client.AuthenticatedFunc = func() bool { return true }
	client.GetCurrentUserFunc = func(ctx context.Context) (*fly.User, error) {
		return &fly.User{ID: "USER1"}, nil
	}
	client.GetAppCompactFunc = func(ctx context.Context, appName string) (*fly.AppCompact, error) {
		if got, want := appName, "test-basic"; got != want {
			t.Fatalf("appName=%s, want %s", got, want)
		}
		return &fly.AppCompact{}, nil // TODO
	}
	/*
		client.EnsureRemoteBuilderFunc = func(ctx context.Context, orgID, appName, region string) (*fly.GqlMachine, *fly.App, error) {
			if got, want := appName, "test-basic"; got != want {
				t.Fatalf("appName=%s, want %s", got, want)
			}
			if got, want := region, ""; got != want {
				t.Fatalf("region=%s, want %s", got, want)
			}
			machine := &fly.GqlMachine{ID: "00000000000001"}
			machine.IPs.Nodes = []*fly.MachineIP{{IP: "ff06::c3", Kind: "privatenet"}}
			app := &fly.App{Name: "my-builder", Organization: fly.Organization{Slug: "my-org"}}
			return machine, app, nil
		}
	*/

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
}

// copyFS writes the contents of a file system to a destination path on disk.
func copyFS(fsys fs.FS, dst string) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		target := filepath.Join(dst, filepath.FromSlash(path))
		if err != nil {
			return err
		}

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o666)
	})
}
