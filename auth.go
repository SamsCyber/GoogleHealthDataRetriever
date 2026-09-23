package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	clientSecretFile = "client_secret.json"
	tokenFile        = "token.json"
)

// Each scope here must also be added under Data Access in the Cloud console.
// After changing this list, delete token.json so the consent screen asks again.
var scopes = []string{
	"https://www.googleapis.com/auth/googlehealth.activity_and_fitness.readonly",
	"https://www.googleapis.com/auth/googlehealth.sleep.readonly",
}

// getTokenSource returns an auto-refreshing token source. It only runs the
// browser consent flow when there's no saved token, or the saved one can no
// longer be refreshed (revoked, or expired under "Testing" publishing status).
func getTokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	secret, err := os.ReadFile(clientSecretFile)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", clientSecretFile, err)
	}

	config, err := google.ConfigFromJSON(secret, scopes...)
	if err != nil {
		return nil, fmt.Errorf("parsing client secret: %w", err)
	}

	tok, err := loadToken()
	if err != nil || tok.RefreshToken == "" {
		if tok, err = consentAndSave(ctx, config); err != nil {
			return nil, err
		}
	}

	ts := config.TokenSource(ctx, tok)

	// Prove the token works now, rather than failing mid-RPC later.
	if _, err := ts.Token(); err != nil {
		fmt.Println("Saved token is no longer valid, re-authorizing...")
		if tok, err = consentAndSave(ctx, config); err != nil {
			return nil, err
		}
		ts = config.TokenSource(ctx, tok)
	}

	return &savingTokenSource{base: ts}, nil
}

func consentAndSave(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	tok, err := runConsentFlow(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := saveToken(tok); err != nil {
		return nil, fmt.Errorf("saving token: %w", err)
	}
	return tok, nil
}

func runConsentFlow(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting local listener: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	config.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/", port)

	state := randomString()
	verifier := oauth2.GenerateVerifier()
	authURL := config.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
		oauth2.S256ChallengeOption(verifier),
	)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		switch {
		case q.Get("state") != state:
			http.Error(w, "State mismatch.", http.StatusBadRequest)
			trySend(errCh, errors.New("oauth state mismatch"))
		case q.Get("error") != "":
			fmt.Fprintln(w, "Authorization failed. Check the terminal.")
			trySend(errCh, fmt.Errorf("authorization error: %s", q.Get("error")))
		default:
			fmt.Fprintln(w, "Authorized. You can close this tab.")
			trySend(codeCh, q.Get("code"))
		}
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	fmt.Println("Opening browser for Google sign-in...")
	fmt.Println("If it doesn't open, visit this URL:")
	fmt.Println(authURL)
	openBrowser(authURL)

	select {
	case code := <-codeCh:
		return config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("timed out waiting for authorization: %w", ctx.Err())
	}
}

type savingTokenSource struct {
	base       oauth2.TokenSource
	mu         sync.Mutex
	lastAccess string
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tok.AccessToken != s.lastAccess {
		s.lastAccess = tok.AccessToken
		if err := saveToken(tok); err != nil {
			fmt.Fprintln(os.Stderr, "warning: couldn't save refreshed token:", err)
		}
	}
	return tok, nil
}

func loadToken() (*oauth2.Token, error) {
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func saveToken(tok *oauth2.Token) error {
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tokenFile, data, 0o600)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func randomString() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func trySend[T any](ch chan T, v T) {
	select {
	case ch <- v:
	default:
	}
}
