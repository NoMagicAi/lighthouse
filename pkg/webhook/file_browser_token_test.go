package webhook

import (
	"net/http"
	"net/http/cgi" // #nosec G504 -- runs git-http-backend for this test only
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jenkins-x/lighthouse/pkg/config"
	"github.com/jenkins-x/lighthouse/pkg/filebrowser"
	"github.com/stretchr/testify/require"
)

const (
	testOwner      = "owner"
	testRepo       = "repo"
	testConfigFile = "config.yaml"
)

// A GitHub App token is replaced before it expires after one hour.
// The file browser must then read the main branch with the new token.
func TestFileBrowserReadsMainBranchAfterTokenRotation(t *testing.T) {
	gitServer := newGitHTTPServer(t)
	tokenPath := filepath.Join(t.TempDir(), "oauth")
	t.Setenv("GIT_TOKEN", "")
	t.Setenv("GIT_TOKEN_PATH", tokenPath)
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	rotateToken := func(token string) {
		require.NoError(t, os.WriteFile(tokenPath, []byte(token), 0o600))
		gitServer.acceptOnly(token)
	}

	rotateToken("first-token")
	gitServer.commit(testConfigFile, "version: 1")
	server := &Server{ConfigAgent: &config.Agent{}, ServerURL: gitServer.url}
	require.NoError(t, server.initializeFileBrowser("bot", gitServer.url.String()))
	fileBrowser := server.FileBrowsers.LighthouseGitFileBrowser()

	content, err := fileBrowser.GetFile(testOwner, testRepo, testConfigFile, "", filebrowser.NewFetchCache())
	require.NoError(t, err)
	require.Equal(t, "version: 1", string(content))

	rotateToken("second-token")
	gitServer.commit(testConfigFile, "version: 2")

	content, err = fileBrowser.GetFile(testOwner, testRepo, testConfigFile, "", filebrowser.NewFetchCache())
	require.NoError(t, err)
	require.Equal(t, "version: 2", string(content))
}

// gitHTTPServer serves one repository over HTTP and accepts only the current token as the password.
type gitHTTPServer struct {
	t       *testing.T
	url     *url.URL
	repoDir string

	lock  sync.Mutex
	token string
}

func newGitHTTPServer(t *testing.T) *gitHTTPServer {
	execPath, err := exec.Command("git", "--exec-path").Output()
	require.NoError(t, err)

	projectRoot := t.TempDir()
	gitServer := &gitHTTPServer{t: t, repoDir: filepath.Join(projectRoot, testOwner, testRepo)}
	require.NoError(t, os.MkdirAll(gitServer.repoDir, 0o700))
	gitServer.git("init", "--initial-branch=main")

	backend := &cgi.Handler{
		Path: filepath.Join(strings.TrimSpace(string(execPath)), "git-http-backend"),
		Env:  []string{"GIT_PROJECT_ROOT=" + projectRoot, "GIT_HTTP_EXPORT_ALL=1"},
	}
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, password, _ := r.BasicAuth()
		gitServer.lock.Lock()
		accepted := password == gitServer.token
		gitServer.lock.Unlock()
		if !accepted {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		backend.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	gitServer.url, err = url.Parse(httpServer.URL)
	require.NoError(t, err)
	return gitServer
}

func (s *gitHTTPServer) acceptOnly(token string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.token = token
}

func (s *gitHTTPServer) commit(path, content string) {
	require.NoError(s.t, os.WriteFile(filepath.Join(s.repoDir, path), []byte(content), 0o600))
	s.git("add", path)
	s.git("-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "--message", content)
}

func (s *gitHTTPServer) git(args ...string) {
	command := exec.Command("git", args...)
	command.Dir = s.repoDir
	output, err := command.CombinedOutput()
	require.NoError(s.t, err, string(output))
}
