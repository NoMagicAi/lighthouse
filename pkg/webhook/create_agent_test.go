package webhook

import (
	"net/http"
	"net/http/httptest"
	"testing"

	lru "github.com/hashicorp/golang-lru"
	"github.com/jenkins-x/go-scm/scm/factory"
	"github.com/jenkins-x/lighthouse/pkg/config"
	"github.com/jenkins-x/lighthouse/pkg/config/lighthouse"
	"github.com/jenkins-x/lighthouse/pkg/filebrowser"
	fbfake "github.com/jenkins-x/lighthouse/pkg/filebrowser/fake"
	"github.com/jenkins-x/lighthouse/pkg/plugins"
	"github.com/jenkins-x/lighthouse/pkg/util"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// Each webhook builds a GitHub client with the token of that moment, and a GitHub App token
// expires after one hour. An event for a cached SHA must use the client of its own webhook.
func TestCreateAgentUsesCurrentClientForCachedSHA(t *testing.T) {
	const headSHA = "0123456789abcdef0123456789abcdef01234567"
	// GitHub accepts only the second token: the first one has expired.
	gitHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer second-token" {
			http.Error(w, "Bad credentials", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(gitHub.Close)

	inRepoConfigEnabled := true
	server := &Server{ConfigAgent: &config.Agent{}, Plugins: &plugins.ConfigAgent{}}
	server.ConfigAgent.Set(&config.Config{ProwConfig: config.ProwConfig{
		InRepoConfig: lighthouse.InRepoConfig{Enabled: map[string]*bool{"*": &inRepoConfigEnabled}},
	}})
	server.Plugins.Set(&plugins.Configuration{})
	var err error
	server.FileBrowsers, err = filebrowser.NewFileBrowsers(gitHub.URL, fbfake.NewFakeFileBrowser(t.TempDir(), true))
	require.NoError(t, err)
	server.InRepoCache, err = lru.New(1)
	require.NoError(t, err)

	receiveWebhook := func(token string) plugins.Agent {
		scmClient, err := factory.NewClient("github", gitHub.URL, "")
		require.NoError(t, err)
		util.AddAuthToSCMClient(scmClient, token)
		server.ClientAgent = &plugins.ClientAgent{SCMProviderClient: scmClient}
		agent, err := server.CreateAgent(logrus.WithField("test", t.Name()), testOwner, testRepo, headSHA)
		require.NoError(t, err)
		return agent
	}

	receiveWebhook("first-token")
	agent := receiveWebhook("second-token")

	_, err = agent.SCMProviderClient.GetPullRequest(testOwner, testRepo, 1)
	require.NoError(t, err)
}
