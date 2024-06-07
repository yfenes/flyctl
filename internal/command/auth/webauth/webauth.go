package webauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/azazeal/pause"
	"github.com/briandowns/spinner"
	"github.com/skratchdot/open-golang/open"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func SaveToken(ctx context.Context, token string) error {

	if ac, err := agent.DefaultClient(ctx); err == nil {
		_ = ac.Kill(ctx)
	}
	config.Clear(state.ConfigFile(ctx))

	if err := persistAccessToken(ctx, token); err != nil {
		return err
	}

	user, err := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{
		AccessToken: token,
	}).GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving current user: %w", err)
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	fmt.Fprintf(io.Out, "successfully logged in as %s\n", colorize.Bold(user.Email))

	return nil
}

func RunWebLogin(ctx context.Context, signup bool) (string, error) {
	fmt.Printf("%+v\n", state.Hostname(ctx))
	auth, err := StartCLISessionWebAuth(state.Hostname(ctx), signup)
	fmt.Printf("%+v\n", auth.URL)
	fmt.Printf("%+v\n", auth)
	if err != nil {
		return "", err
	}

	io := iostreams.FromContext(ctx)
	if err := open.Run(auth.URL); err != nil {
		fmt.Fprintf(io.ErrOut,
			"failed opening browser. Copy the url (%s) into a browser and continue\n",
			auth.URL,
		)
	}

	logger := logger.FromContext(ctx)

	colorize := io.ColorScheme()
	fmt.Fprintf(io.Out, "Opening %s ...\n\n", colorize.Bold(auth.URL))

	token, err := waitForCLISession(ctx, logger, io.ErrOut, auth.ID)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "", errors.New("Login expired, please try again")
	case err != nil:
		return "", err
	case token == "":
		return "", errors.New("failed to log in, please try again")
	}

	return token, nil
}

func StartCLISessionWebAuth(machineName string, signup bool) (fly.CLISession, error) {

	return StartCLISession(machineName, map[string]interface{}{
		"signup": signup,
		"target": "auth",
	})
}

// StartCLISession starts a session with the platform via web
func StartCLISession(sessionName string, args map[string]interface{}) (fly.CLISession, error) {
	var result fly.CLISession

	if args == nil {
		args = make(map[string]interface{})
	}
	args["name"] = sessionName

	postData, _ := json.Marshal(args)

	url := fmt.Sprintf("%s/api/v1/cli_sessions", "http://localhost:4000")
	// url := fmt.Sprintf("%s/api/v1/cli_sessions", "https://api.fly.io")

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(postData))
	fmt.Printf("%+v\n", bytes.NewBuffer(postData))
	fmt.Printf("%+v\n", err)
	if err != nil {
		return result, err
	}
	req.Header.Add("content-type", "application/json")
	req.Header.Add("x-staging", "1")
	resp, err := http.DefaultClient.Do(req)
	fmt.Printf("%+v\n", err)
	fmt.Printf("%+v\n", resp)
	// resp, err := http.Post(url, "application/json", bytes.NewBuffer(postData))
	if err != nil {
		return result, err
	}

	if resp.StatusCode != 201 {
		fmt.Printf("%+v\n", req.Body)
		errorRes := make(map[string]interface{})
		json.NewDecoder(resp.Body).Decode(&errorRes)
		fmt.Printf("%+v\n", errorRes)
		return result, fly.ErrUnknown
	}

	defer resp.Body.Close() //skipcq: GO-S2307

	json.NewDecoder(resp.Body).Decode(&result)

	return result, nil
}

// GetAccessTokenForCLISession Obtains the access token for the session
func GetAccessTokenForCLISession(ctx context.Context, id string) (string, error) {
	val, err := GetCLISessionState(ctx, id)
	if err != nil {
		return "", err
	}
	return val.AccessToken, nil
}

func GetCLISessionState(ctx context.Context, id string) (fly.CLISession, error) {

	var value fly.CLISession

	url := fmt.Sprintf("%s/api/v1/cli_sessions/%s", "http://localhost:4000", id)
	// url := fmt.Sprintf("%s/api/v1/cli_sessions/%s", "https://api.fly.io", id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	fmt.Printf("%+v\n", err)
	if err != nil {
		return value, err
	}
	req.Header.Add("x-staging", "1")

	res, err := http.DefaultClient.Do(req)
	fmt.Printf("%+v\n", err)
	if err != nil {
		return value, err
	}
	defer res.Body.Close() //skipcq: GO-S2307

	switch res.StatusCode {
	case http.StatusOK:
		var auth fly.CLISession
		if err = json.NewDecoder(res.Body).Decode(&auth); err != nil {
			return value, fmt.Errorf("failed to decode session, please try again: %w", err)
		}
		return auth, nil
	case http.StatusNotFound:
		return value, fly.ErrNotFound
	default:
		return value, fly.ErrUnknown
	}
}

// TODO: this does NOT break on interrupts
func waitForCLISession(parent context.Context, logger *logger.Logger, w io.Writer, id string) (token string, err error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = w
	s.Prefix = "Waiting for session..."
	s.Start()

	for ctx.Err() == nil {
		if token, err = GetAccessTokenForCLISession(ctx, id); err != nil {
			logger.Debugf("failed retrieving token: %v", err)

			pause.For(ctx, time.Second)

			continue
		}

		logger.Debug("retrieved access token.")

		s.FinalMSG = "Waiting for session... Done\n"
		s.Stop()

		break
	}

	return
}

func persistAccessToken(ctx context.Context, token string) (err error) {
	path := state.ConfigFile(ctx)

	if err = config.SetAccessToken(path, token); err != nil {
		err = fmt.Errorf("failed persisting %s in %s: %w\n",
			config.AccessTokenFileKey, path, err)
	}

	return
}
