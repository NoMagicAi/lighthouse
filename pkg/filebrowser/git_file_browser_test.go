package filebrowser_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jenkins-x/lighthouse/pkg/filebrowser"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/lighthouse/pkg/git/v2"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitFileBrowser(t *testing.T) {
	const verbose = false

	cf, err := git.NewClientFactory()
	require.NoError(t, err, "failed to create git client factory")
	fb := filebrowser.NewFileBrowserFromGitClient(cf)

	fc := filebrowser.NewFetchCache()

	owner := "jenkins-x-quickstarts"
	repo := "node-http"
	path := "/"
	ref := "5067522b7ed292bef46570ffe3ab75d3a5428769"
	fileName := "LICENSE"

	message := func() string {
		return fmt.Sprintf("for %s/%s path %s ref %s", owner, repo, path, ref)
	}

	fileMessage := func() string {
		return message() + " file " + fileName
	}

	files, err := fb.ListFiles(owner, repo, path, ref, fc)
	require.NoError(t, err, "failed to list files "+message())
	assert.NotEmpty(t, files, "should not be empty")
	for _, f := range files {
		t.Logf("file %s type %s\n", f.Name, f.Type)
	}

	data, err := fb.GetFile(owner, repo, fileName, ref, fc)
	require.NoError(t, err, "failed to get file "+fileMessage())
	text := string(data)

	if verbose {
		t.Logf("loaded file %s content: %s\n", path, text)
	}
	require.Contains(t, text, "Apache", message())

	// switch the main branch
	ref = ""
	fileName = "package.json"

	data, err = fb.GetFile(owner, repo, fileName, ref, fc)
	require.NoError(t, err, "failed to get file "+fileMessage())
	text = string(data)

	t.Logf("loaded file %s content: %s\n", path, text)
	require.Contains(t, text, "server.js", message())

	// switch back to a old sha
	ref = "5067522b7ed292bef46570ffe3ab75d3a5428769"
	files, err = fb.ListFiles(owner, repo, path, ref, fc)
	require.NoError(t, err, "failed to list files "+message())
	assert.NotEmpty(t, files, "should not be empty")
	for _, f := range files {
		t.Logf("file %s type %s\n", f.Name, f.Type)
	}

	assertNoScmFileExists(t, files, fileName, message())

	ref = ""
	files, err = fb.ListFiles(owner, repo, path, ref, fc)
	require.NoError(t, err, "failed to list files "+message())
	assertScmFileExists(t, files, fileName, message())
}

func assertScmFileExists(t *testing.T, files []*scm.FileEntry, name, message string) {
	for _, f := range files {
		if f.Name == name {
			t.Logf("includes expected file name %s for %s", name, message)
			return
		}
	}
	require.Fail(t, "should have found file %s %s", name, message)
}

func assertNoScmFileExists(t *testing.T, files []*scm.FileEntry, name, message string) {
	for _, f := range files {
		if f.Name == name {
			require.Fail(t, "should not have found file %s %s", name, message)
		}
	}
	t.Logf("correctly does not include file name %s for %s", name, message)
}

func TestGitFileBrowser_Clone_CreateTag_FetchRef(t *testing.T) {

	logger := logrus.WithField("client", "git")

	baseDir := t.TempDir()
	fmt.Println(baseDir)

	fc := filebrowser.NewFetchCache()

	repoDir := filepath.Join(baseDir, "org", "repo")

	err := os.MkdirAll(repoDir, 0700)
	require.NoError(t, err, "failed to make repo dir")

	userGetter := func() (name, email string, err error) {
		return "bot", "bot@example.com", nil
	}
	censor := func(content []byte) []byte { return content }

	executor, err := git.NewCensoringExecutor(repoDir, censor, logger)
	require.NoError(t, err, "failed to find git binary")

	// lets fetch the default branch
	defaultBranch := "master"
	out, err := executor.Run("config", "--global", "--get", "init.defaultBranch")
	if err == nil {
		text := strings.TrimSpace(string(out))
		if text != "" {
			defaultBranch = text
		}
	}
	t.Logf("using default branch: %s\n", defaultBranch)

	err = os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("README"), 0600)
	require.NoError(t, err, "failed to write README.md file")

	out, err = executor.Run("init", ".")
	require.NoError(t, err, "failed to init git repo: %s", out)

	out, err = executor.Run("add", "README.md")
	require.NoError(t, err, "failed to add README.md: %s", out)

	name, email, _ := userGetter()
	out, err = executor.Run("config", "--add", "user.name", name)
	require.NoError(t, err, "failed to set user.name: %s", out)

	out, err = executor.Run("config", "--add", "user.email", email)
	require.NoError(t, err, "failed to set user.email: %s", out)

	out, err = executor.Run("commit", "-m", "add README.md")
	require.NoError(t, err, "failed to commit README.md: %s", out)

	cf, err := git.NewLocalClientFactory(baseDir, userGetter, censor)
	require.NoError(t, err, "failed to create git client factory")
	fb := filebrowser.NewFileBrowserFromGitClient(cf)

	files, err := fb.ListFiles("org", "repo", "", defaultBranch, fc)
	require.NoError(t, err, "failed to list files")

	require.True(t, len(files) == 1, "exepecting 1 file")
	require.Equal(t, files[0].Name, "README.md")

	out, err = executor.Run("checkout", "-b", "update-1")
	require.NoError(t, err, "failed to create new branch: %s", out)

	err = os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("README-update-1"), 0600)
	require.NoError(t, err, "failed write updated README.md")

	out, err = executor.Run("commit", "-a", "-m", "update README.md")
	require.NoError(t, err, "failed to commit README.md: %s", out)

	out, err = executor.Run("tag", "v0.0.1")
	require.NoError(t, err, "failed to create v0.0.1 tag: %s", out)

	files, err = fb.ListFiles("org", "repo", "", "v0.0.1", fc)
	require.NoError(t, err, "failed to lst files in v0.0.1 tag")
	require.True(t, len(files) == 1, "exepecting 1 file")
	require.Equal(t, files[0].Name, "README.md")

	data, err := fb.GetFile("org", "repo", "README.md", "v0.0.1", fc)
	require.NoError(t, err, "failed to lst files in v0.0.1 tag")
	require.Equal(t, string(data), "README-update-1")
}

// recordingClientFactory keeps the repo client that the file browser works on.
type recordingClientFactory struct {
	git.ClientFactory
	repoClient git.RepoClient
}

func (f *recordingClientFactory) ClientFor(org, repo string, sparseCheckoutPatterns []string) (git.RepoClient, error) {
	repoClient, err := f.ClientFactory.ClientFor(org, repo, sparseCheckoutPatterns)
	f.repoClient = repoClient
	return repoClient, err
}

func TestGitFileBrowser_ReadsWithoutCheckout(t *testing.T) {
	baseDir := t.TempDir()
	repoDir := filepath.Join(baseDir, "org", "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".lighthouse"), 0700))

	userGetter := func() (name, email string, err error) {
		return "bot", "bot@example.com", nil
	}
	censor := func(content []byte) []byte { return content }
	executor, err := git.NewCensoringExecutor(repoDir, censor, logrus.WithField("client", "git"))
	require.NoError(t, err, "failed to find git binary")

	runGit := func(args ...string) string {
		out, err := executor.Run(args...)
		require.NoError(t, err, "failed to run git %v: %s", args, out)
		return strings.TrimSpace(string(out))
	}
	writeFile := func(name, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, name), []byte(content), 0600))
	}

	runGit("init", ".")
	runGit("config", "--add", "user.name", "bot")
	runGit("config", "--add", "user.email", "bot@example.com")
	writeFile("README.md", "first")
	writeFile(".lighthouse/triggers.yaml", "first triggers")
	runGit("add", ".")
	runGit("commit", "-m", "first")
	firstSHA := runGit("rev-parse", "HEAD")
	writeFile("README.md", "second")
	writeFile(".lighthouse/extra.yaml", "extra")
	runGit("add", ".")
	runGit("commit", "-m", "second")

	localFactory, err := git.NewLocalClientFactory(baseDir, userGetter, censor)
	require.NoError(t, err, "failed to create git client factory")
	factory := &recordingClientFactory{ClientFactory: localFactory}
	fb := filebrowser.NewFileBrowserFromGitClient(factory)
	fc := filebrowser.NewFetchCache()

	data, err := fb.GetFile("org", "repo", "README.md", firstSHA, fc)
	require.NoError(t, err, "failed to get README.md at the first commit")
	assert.Equal(t, "first", string(data))

	data, err = fb.GetFile("org", "repo", "README.md", "", fc)
	require.NoError(t, err, "failed to get README.md on the main branch")
	assert.Equal(t, "second", string(data))

	data, err = fb.GetFile("org", "repo", "missing.md", firstSHA, fc)
	require.NoError(t, err, "a missing file is not an error")
	assert.Nil(t, data)

	err = fb.WithDir("org", "repo", firstSHA, fc, []string{"/.lighthouse/**"}, func(dir string) error {
		triggers, err := os.ReadFile(filepath.Join(dir, ".lighthouse", "triggers.yaml"))
		require.NoError(t, err, "failed to read the exported triggers.yaml")
		assert.Equal(t, "first triggers", string(triggers))
		assert.NoFileExists(t, filepath.Join(dir, ".lighthouse", "extra.yaml"), "extra.yaml is not in the first commit")
		assert.NoFileExists(t, filepath.Join(dir, "README.md"), "only .lighthouse is exported")
		return nil
	})
	require.NoError(t, err, "failed to export .lighthouse at the first commit")

	// a new request sees a new commit on the main branch
	writeFile("README.md", "third")
	runGit("commit", "-a", "-m", "third")
	data, err = fb.GetFile("org", "repo", "README.md", "", filebrowser.NewFetchCache())
	require.NoError(t, err, "failed to get README.md after a new commit")
	assert.Equal(t, "third", string(data))

	assert.NoFileExists(t, filepath.Join(factory.repoClient.Directory(), "README.md"), "the clone must stay without a checkout")
}

func TestIsSHA(t *testing.T) {
	testCases := map[string]bool{
		"de6cc99": true,
		"de6cc99a6de8ca34b8884fcc05945bd30033f330": true,
		"main":    false,
		"123_567": false,
	}

	for ref, expected := range testCases {
		got := filebrowser.IsSHA(ref)
		assert.Equal(t, expected, got, "for ref: %s", ref)
	}
}
